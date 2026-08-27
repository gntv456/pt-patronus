package filter

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
	"gorm.io/gorm"
)

// setupTestStore 用内存 SQLite 构造真实 SystemConfigOper，验证 Store+Engine+KV 协同。
func setupTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.SystemConfig{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return NewStore(oper.NewSystemConfigOper(db))
}

// TestIntegrationCRUDAndFilter 端到端：建规则/组 → 持久化 KV → 引擎求值 pri_order → 绑定 → 级联清理。
func TestIntegrationCRUDAndFilter(t *testing.T) {
	store := setupTestStore(t)
	eng := NewEngine(store)
	ctx := context.Background()

	// 1. 内置 22 条可查。
	if got := len(ListBuiltins()); got != 22 {
		t.Fatalf("内置规则数 %d，期望 22", got)
	}

	// 2. 建自定义规则（include 单正则）。
	if _, err := store.AddCustomRule(CustomRuleInput{ID: "MY4K", Name: "我的4K", Include: "2160p|4k"}); err != nil {
		t.Fatalf("AddCustomRule: %v", err)
	}
	customs, err := store.GetCustomRules()
	if err != nil || len(customs) != 1 || customs[0].ID != "MY4K" {
		t.Fatalf("自定义规则读取异常：err=%v customs=%+v", err, customs)
	}

	// 3. 建规则组（引用 MY4K + 内置 1080P）。
	if _, err := store.AddGroup(RuleGroupInput{Name: "测试组", RuleString: "MY4K > 1080P"}); err != nil {
		t.Fatalf("AddGroup: %v", err)
	}

	// 4. 引用未知 id 应被拒（ValidateRuleString）。
	if _, err := store.AddGroup(RuleGroupInput{Name: "坏组", RuleString: "MY4K > UNKNOWN"}); err == nil {
		t.Fatal("引用未知规则 id 应失败")
	}

	// 5. Engine.Filter：MY4K(2160p)→pri100，1080P→pri99，720p→剔除；返回按 pri 降序。
	torrents := []site.Torrent{
		{Title: "B.1080p.WEB-DL.x264"},
		{Title: "A.2160p.UHD.HEVC"},
		{Title: "C.720p.HDTV.x264"},
	}
	matched, err := eng.Filter(ctx, []string{"测试组"}, torrents, nil)
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	if len(matched) != 2 {
		t.Fatalf("应命中 2 条（剔除 720p），实际 %d", len(matched))
	}
	if matched[0].PriOrder != 100 || !strings.Contains(matched[0].Torrent.Title, "2160") {
		t.Fatalf("首位应为 2160p pri100，实际 %+v", matched[0])
	}
	if matched[1].PriOrder != 99 || !strings.Contains(matched[1].Torrent.Title, "1080") {
		t.Fatalf("次位应为 1080p pri99，实际 %+v", matched[1])
	}

	// 6. 自定义规则可覆盖内置：自定义 4K 把 include 改窄到只匹配 "xyz"。
	if _, err := store.AddCustomRule(CustomRuleInput{ID: "4K", Include: "xyz-not-real"}); err != nil {
		t.Fatalf("AddCustomRule override 4K: %v", err)
	}
	rs, _ := store.RuleSet()
	if rs["4K"].Include[0] != "xyz-not-real" {
		t.Fatalf("自定义应覆盖内置 4K，实际 %+v", rs["4K"])
	}

	// 7. 全局绑定 Get/Set + ResolveGroups。
	if err := store.SetBindings(KeySubscribeFilterRuleGroups, []string{"测试组"}); err != nil {
		t.Fatal(err)
	}
	got, _ := store.GetBindings(KeySubscribeFilterRuleGroups)
	if len(got) != 1 || got[0] != "测试组" {
		t.Fatalf("绑定异常：%v", got)
	}

	// 8. 删规则组 → 级联清理全局绑定的悬空引用。
	if err := store.DeleteGroup("测试组"); err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}
	got2, _ := store.GetBindings(KeySubscribeFilterRuleGroups)
	if len(got2) != 0 {
		t.Fatalf("删组后绑定应被清理，实际 %v", got2)
	}

	// 9. 删被引用的自定义规则应被拒（先建引用再试）。
	store.AddGroup(RuleGroupInput{Name: "引用组", RuleString: "MY4K"})
	if err := store.DeleteCustomRule("MY4K"); err == nil {
		t.Fatal("被规则组引用的自定义规则应禁止删除")
	}
}

// TestIntegrationResolveGroupsByMedia 验证 ResolveGroups 按 media_type/category 筛选适用组。
func TestIntegrationResolveGroupsByMedia(t *testing.T) {
	store := setupTestStore(t)
	store.AddGroup(RuleGroupInput{Name: "全局组", RuleString: "4K"})                  // media_type 空 → 全局
	store.AddGroup(RuleGroupInput{Name: "电影组", RuleString: "4K", MediaType: "电影"}) // 仅电影
	store.AddGroup(RuleGroupInput{Name: "电视组", RuleString: "4K", MediaType: "电视剧"})

	// movie → 全局组 + 电影组。
	got, _ := store.ResolveGroups(&MediaInfo{Type: "movie"}, nil)
	names := groupNames(got)
	if !contains(names, "全局组") || !contains(names, "电影组") || contains(names, "电视组") {
		t.Fatalf("movie 应命中 全局组+电影组，实际 %v", names)
	}
	// tv → 全局组 + 电视组。
	got2, _ := store.ResolveGroups(&MediaInfo{Type: "tv"}, nil)
	names2 := groupNames(got2)
	if !contains(names2, "电视组") || contains(names2, "电影组") {
		t.Fatalf("tv 应命中 全局组+电视组，实际 %v", names2)
	}
	// 指定 names 过滤。
	got3, _ := store.ResolveGroups(&MediaInfo{Type: "movie"}, []string{"电影组"})
	if len(got3) != 1 || got3[0].Name != "电影组" {
		t.Fatalf("names 过滤异常：%v", groupNames(got3))
	}
}

// TestMatchedTorrentJSON 验证 pri_order/torrent 小写 JSON key（前后端契约）。
func TestMatchedTorrentJSON(t *testing.T) {
	b, err := json.Marshal(MatchedTorrent{Torrent: site.Torrent{Title: "X"}, PriOrder: 99})
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `"torrent"`) || !strings.Contains(s, `"pri_order"`) {
		t.Fatalf("MatchedTorrent JSON 应含小写 torrent/pri_order key，实际 %s", s)
	}
}

func groupNames(gs []RuleGroup) []string {
	out := make([]string, 0, len(gs))
	for _, g := range gs {
		out = append(out, g.Name)
	}
	return out
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// TestSitePriorityTieBreak 站点优先级参与择优：同层（PriOrder 相同）候选按来源站
// Priority 降序稳定排序（对齐 MP 站点优先级择优）；不同层仍以 PriOrder 为准。
func TestSitePriorityTieBreak(t *testing.T) {
	store := setupTestStore(t)
	eng := NewEngine(store)
	ctx := context.Background()

	if _, err := store.AddGroup(RuleGroupInput{Name: "P组", RuleString: "1080P"}); err != nil {
		t.Fatalf("AddGroup: %v", err)
	}
	torrents := []site.Torrent{
		{Title: "B.1080p.WEB", SitePriority: 10},
		{Title: "A.1080p.WEB", SitePriority: 30},
		{Title: "C.1080p.WEB", SitePriority: 20},
	}
	matched, err := eng.Filter(ctx, []string{"P组"}, torrents, nil)
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	if len(matched) != 3 {
		t.Fatalf("应全部命中，实际 %d", len(matched))
	}
	for i, want := range []string{"A", "C", "B"} {
		if !strings.HasPrefix(matched[i].Torrent.Title, want) {
			t.Fatalf("第 %d 位应为站优先级 30/20/10 排序（%s…），实际 %s", i+1, want, matched[i].Torrent.Title)
		}
	}
}
