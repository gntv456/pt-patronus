package plugins

import (
	"context"
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
)

func newMarketTestOper(t *testing.T) *oper.PluginMarketOper {
	t.Helper()
	db := newPluginTestDB(t)
	if err := db.AutoMigrate(&models.PluginInstall{}, &models.PluginFavorite{}, &models.PluginInstance{}); err != nil {
		t.Fatalf("migrate market tables: %v", err)
	}
	return oper.NewPluginMarketOper(db)
}

func TestMarketOperInstallCharts(t *testing.T) {
	o := newMarketTestOper(t)
	// a 安装 3 次、b 1 次、c 2 次
	for i := 0; i < 3; i++ {
		if err := o.RecordInstall("plugin-a", "1.0.0", "src"); err != nil {
			t.Fatal(err)
		}
	}
	_ = o.RecordInstall("plugin-b", "1.0.0", "src")
	_ = o.RecordInstall("plugin-c", "1.0.0", "src")
	_ = o.RecordInstall("plugin-c", "1.1.0", "src")

	if n, _ := o.InstallCount("plugin-a"); n != 3 {
		t.Fatalf("install count a=%d", n)
	}
	top, err := o.TopInstalled(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 2 || top[0].PluginID != "plugin-a" || top[1].PluginID != "plugin-c" {
		t.Fatalf("disciple chart: %+v", top)
	}
	recent, err := o.RecentInstalls(3)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 3 {
		t.Fatalf("newcomer chart len=%d: %+v", len(recent), recent)
	}
	// 去重：3 个插件各留一条
	seen := map[string]bool{}
	for _, r := range recent {
		if seen[r.PluginID] {
			t.Fatalf("newcomer chart has dup: %s", r.PluginID)
		}
		seen[r.PluginID] = true
	}
}

func TestMarketOperFavorite(t *testing.T) {
	o := newMarketTestOper(t)
	if err := o.Favorite(0, "p1"); err != nil {
		t.Fatal(err)
	}
	if err := o.Favorite(0, "p1"); err != nil { // 幂等
		t.Fatal(err)
	}
	if err := o.Favorite(0, "p2"); err != nil {
		t.Fatal(err)
	}
	favs, err := o.Favorites(0)
	if err != nil || len(favs) != 2 {
		t.Fatalf("favorites: %+v %v", favs, err)
	}
	if !o.IsFavorite(0, "p1") || o.IsFavorite(0, "p3") {
		t.Fatal("isFavorite wrong")
	}
	if err := o.Unfavorite(0, "p1"); err != nil {
		t.Fatal(err)
	}
	if err := o.Unfavorite(0, "p1"); err != nil { // 幂等
		t.Fatal(err)
	}
	if favs, _ = o.Favorites(0); len(favs) != 1 || favs[0] != "p2" {
		t.Fatalf("after unfavorite: %+v", favs)
	}
}

func TestMarketOperInstanceAndReorder(t *testing.T) {
	o := newMarketTestOper(t)
	i1, err := o.CreateInstance("brush", "主实例", "{}")
	if err != nil {
		t.Fatal(err)
	}
	i2, err := o.CreateInstance("brush", "分身-2", "{}")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := o.CreateInstance("brush", "分身-2", "{}"); err == nil {
		t.Fatal("dup instance key must fail")
	}
	insts, err := o.Instances("brush")
	if err != nil || len(insts) != 2 || insts[0].ID != i1.ID {
		t.Fatalf("instances: %+v %v", insts, err)
	}
	// 天序：拖拽反转顺序
	if err := o.ReorderInstances("brush", []uint{i2.ID, i1.ID}); err != nil {
		t.Fatal(err)
	}
	insts, _ = o.Instances("brush")
	if insts[0].ID != i2.ID || insts[0].SortOrder != 1 || insts[1].ID != i1.ID {
		t.Fatalf("reorder not persisted: %+v", insts)
	}
	// 配置/启停
	if err := o.SaveInstanceConfig("brush", "分身-2", `{"x":1}`, false); err != nil {
		t.Fatal(err)
	}
	insts, _ = o.Instances("brush")
	if insts[0].Enabled || insts[0].ConfigJSON != `{"x":1}` {
		t.Fatalf("save config: %+v", insts[0])
	}
	// 删除幂等
	if err := o.DeleteInstance("brush", "分身-2"); err != nil {
		t.Fatal(err)
	}
	if err := o.DeleteInstance("brush", "分身-2"); err != nil {
		t.Fatal(err)
	}
	if insts, _ = o.Instances("brush"); len(insts) != 1 {
		t.Fatalf("after delete: %+v", insts)
	}
}

// TestManagerSingleInstance 内置插件单实例（不可分身）；未注册插件非单实例。
func TestManagerSingleInstance(t *testing.T) {
	db := newPluginTestDB(t)
	m := NewManager(db, nil, nil, nil, nil)
	mustRegister(t, m, NewEventAuditPlugin())
	if !m.SingleInstance("event-audit") {
		t.Fatal("builtin plugin must be single-instance")
	}
	if m.SingleInstance("not-registered") {
		t.Fatal("unknown plugin must not be single-instance")
	}
}

// TestManagerMarketOperUnavailable 未注入 marketOp 时运营 API 明确报错（不 panic）。
func TestManagerMarketOperUnavailable(t *testing.T) {
	db := newPluginTestDB(t)
	m := NewManager(db, nil, nil, nil, nil)
	if err := m.FavoritePlugin("x"); err == nil {
		t.Fatal("favorite without oper must error")
	}
	if _, err := m.CreatePluginInstance("x", "k", "{}"); err == nil {
		t.Fatal("create instance without oper must error")
	}
	if err := m.ReorderPluginInstances("x", []uint{1}); err == nil {
		t.Fatal("reorder without oper must error")
	}
	charts, err := m.MarketCharts(context.TODO())
	if err != nil || charts == nil || len(charts.Disciples) != 0 {
		t.Fatalf("charts without oper should be empty view: %+v %v", charts, err)
	}
}
