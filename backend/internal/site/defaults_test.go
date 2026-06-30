package site

import (
	"os"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestNoNexusPHPPollution：非 nexusphp schema 的空 def 经 EnsureDefaults 后，不应被注入
// NexusPHP 选择器默认（table.torrents / font.color_*）与 capability 默认（Freeleech/Harem/
// Messages/Detail 不应默认开启）。通用项（SearchEntry/SearchParam/HR.Path）对所有 schema 保留。
func TestNoNexusPHPPollution(t *testing.T) {
	for _, schema := range []string{"tnode", "rousi", "gazelle_jsonapi"} {
		def := EnsureDefaults(&SiteMetadata{ID: "x", Schema: schema})
		if def.Selectors.Rows.Selector != "" {
			t.Errorf("schema=%s: Selectors.Rows 不应填 NexusPHP 默认, got %q", schema, def.Selectors.Rows.Selector)
		}
		if len(def.UserInfo.Fields) != 0 {
			t.Errorf("schema=%s: UserInfo.Fields 不应填默认（含 font.color_* 选择器）", schema)
		}
		if len(def.UserInfo.DetailFields) != 0 {
			t.Errorf("schema=%s: UserInfo.DetailFields 不应填默认", schema)
		}
		if def.Freeleech.Enabled {
			t.Errorf("schema=%s: Freeleech 不应默认开启", schema)
		}
		if def.Harem.Enabled {
			t.Errorf("schema=%s: Harem 不应默认开启", schema)
		}
		if def.Messages.Enabled {
			t.Errorf("schema=%s: Messages 不应默认开启", schema)
		}
		if def.Detail.Enabled {
			t.Errorf("schema=%s: Detail 不应默认开启", schema)
		}
		// 通用项仍应填充（所有 schema 共享）
		if def.SearchEntry != "torrents.php" {
			t.Errorf("schema=%s: SearchEntry 通用默认应为 torrents.php, got %q", schema, def.SearchEntry)
		}
		if def.SearchParam != "search" {
			t.Errorf("schema=%s: SearchParam 通用默认应为 search, got %q", schema, def.SearchParam)
		}
		if def.HR.Path != "getusertorrentlist.php?type=hr" {
			t.Errorf("schema=%s: HR.Path 通用默认应填充, got %q", schema, def.HR.Path)
		}
	}
}

// TestUnit3DDefaults：unit3d schema 经 EnsureDefaults 后应填入 UNIT3D 专属默认（非 NexusPHP），
// 且不启用 NexusPHP capability。与 TestNoNexusPHPPollution 互补（unit3d 已移出该列表）。
func TestUnit3DDefaults(t *testing.T) {
	def := EnsureDefaults(&SiteMetadata{ID: "u3d", Schema: "unit3d"})
	// 搜索入口/参数：UNIT3D 用 /torrents + name（非 torrents.php/search）
	if def.SearchEntry != "torrents" {
		t.Errorf("SearchEntry 应为 torrents, got %q", def.SearchEntry)
	}
	if def.SearchParam != "name" {
		t.Errorf("SearchParam 应为 name, got %q", def.SearchParam)
	}
	// rows 应是 UNIT3D 回退列表（含 torrent-search--list__results），非 NexusPHP table.torrents
	if len(def.Selectors.Rows.Selectors) == 0 {
		t.Fatalf("Selectors.Rows.Selectors 应填 UNIT3D 回退列表")
	}
	if def.Selectors.Rows.Selector == "table.torrents tr" {
		t.Errorf("不应注入 NexusPHP table.torrents 选择器")
	}
	// uploaded 选择器应是 UNIT3D ratio-bar，非 NexusPHP font.color_uploaded
	uq, ok := def.UserInfo.Fields["uploaded"]
	if !ok {
		t.Fatalf("UserInfo.Fields 应含 uploaded")
	}
	if uq.Selector == "font.color_uploaded" || len(uq.Selectors) == 0 {
		t.Errorf("uploaded 应是 UNIT3D ratio-bar 选择器, got %+v", uq)
	}
	// bonus_per_hour 在 DetailFields（earnings 页第 3 个 dd）
	if _, ok := def.UserInfo.DetailFields["bonus_per_hour"]; !ok {
		t.Errorf("DetailFields 应含 bonus_per_hour")
	}
	// NoLogin：UNIT3D 重定向 /login
	if len(def.NoLogin.URLPattern) == 0 || def.NoLogin.URLPattern[0] != "/login" {
		t.Errorf("NoLogin.URLPattern 应含 /login, got %v", def.NoLogin.URLPattern)
	}
	// 不启用 NexusPHP capability（属后续里程碑）
	if def.Freeleech.Enabled || def.Harem.Enabled || def.Messages.Enabled || def.Detail.Enabled {
		t.Errorf("不应启用 NexusPHP capability (Freeleech/Harem/Messages/Detail)")
	}
}

// TestGazelleDefaults：gazelle schema 经 EnsureDefaults 后填入 Gazelle 专属默认（非 NexusPHP）。
// gazelle/luminance 已从 TestNoNexusPHPPollution 移出（它们现在注入选择器）。
func TestGazelleDefaults(t *testing.T) {
	def := EnsureDefaults(&SiteMetadata{ID: "gz", Schema: "gazelle"})
	if def.SearchParam != "searchstr" {
		t.Errorf("SearchParam 应为 searchstr, got %q", def.SearchParam)
	}
	if def.ExtraParam["searchsubmit"] != "1" {
		t.Errorf("ExtraParam[searchsubmit] 应为 1, got %q", def.ExtraParam["searchsubmit"])
	}
	if def.Selectors.Rows.Selector != "table.torrent_table tr" {
		t.Errorf("Rows 应为 table.torrent_table tr, got %q", def.Selectors.Rows.Selector)
	}
	if _, ok := def.UserInfo.Fields["uploaded"]; !ok {
		t.Errorf("UserInfo.Fields 应含 uploaded")
	}
	if len(def.NoLogin.URLPattern) == 0 || def.NoLogin.URLPattern[0] != "login.php" {
		t.Errorf("NoLogin.URLPattern 应含 login.php, got %v", def.NoLogin.URLPattern)
	}
	if def.Freeleech.Enabled || def.Harem.Enabled || def.Messages.Enabled || def.Detail.Enabled {
		t.Errorf("不应启用 NexusPHP capability")
	}
}

// TestLuminanceDefaults：luminance schema 经 EnsureDefaults 后填入 Luminance 专属默认。
func TestLuminanceDefaults(t *testing.T) {
	def := EnsureDefaults(&SiteMetadata{ID: "lm", Schema: "luminance"})
	if def.SearchParam != "title" {
		t.Errorf("SearchParam 应为 title, got %q", def.SearchParam)
	}
	if def.ExtraParam["action"] != "advanced" {
		t.Errorf("ExtraParam[action] 应为 advanced, got %q", def.ExtraParam["action"])
	}
	if def.Selectors.Rows.Selector != "table#torrent_table tr" {
		t.Errorf("Rows 应为 table#torrent_table tr, got %q", def.Selectors.Rows.Selector)
	}
	if _, ok := def.UserInfo.Fields["uploaded"]; !ok {
		t.Errorf("UserInfo.Fields 应含 uploaded")
	}
	if def.Freeleech.Enabled || def.Harem.Enabled || def.Messages.Enabled || def.Detail.Enabled {
		t.Errorf("不应启用 NexusPHP capability")
	}
}

// TestNexusPHPDefaultsPreserved：空 schema 与显式 nexusphp schema 仍走 NexusPHP 分支，
// 填入选择器与 capability 默认（向后兼容现有 20 站与既有测试）。
func TestNexusPHPDefaultsPreserved(t *testing.T) {
	for _, schema := range []string{"", "nexusphp"} {
		def := EnsureDefaults(&SiteMetadata{ID: "x", Schema: schema})
		if def.Selectors.Rows.Selector == "" {
			t.Errorf("schema=%q: Selectors.Rows 应填 NexusPHP 默认", schema)
		}
		if _, ok := def.UserInfo.Fields["uploaded"]; !ok {
			t.Errorf("schema=%q: UserInfo.Fields 应含 NexusPHP uploaded 默认", schema)
		}
		if !def.Freeleech.Enabled || !def.Harem.Enabled || !def.Messages.Enabled || !def.Detail.Enabled {
			t.Errorf("schema=%q: capability 默认应开启 (Freeleech/Harem/Messages/Detail)", schema)
		}
	}
}

// TestMTeamDefaults：mteam schema 经 EnsureDefaults 后应默认放行 Freeleech/Announcement
//（适配器已实现 FetchFreeleech/FetchAnnouncement），但不注入 NexusPHP 选择器，也不开
// 其余 NexusPHP capability（Harem/Messages/Detail 馒头适配器未实现）。
// 回归：修复前 mteam 落入 default 分支，Enabled 恒 false → 前端误显「未开启站免检测」。
func TestMTeamDefaults(t *testing.T) {
	def := EnsureDefaults(&SiteMetadata{ID: "mt", Schema: "mteam"})
	if !def.Freeleech.Enabled {
		t.Errorf("Freeleech 应默认开启（适配器实现了 FetchFreeleech）")
	}
	if !def.Announcement.Enabled {
		t.Errorf("Announcement 应默认开启（适配器实现了 FetchAnnouncement）")
	}
	// 不应注入 NexusPHP 选择器/capability
	if def.Selectors.Rows.Selector != "" {
		t.Errorf("不应注入 NexusPHP 选择器, got %q", def.Selectors.Rows.Selector)
	}
	if def.Harem.Enabled || def.Messages.Enabled || def.Detail.Enabled {
		t.Errorf("不应启用 NexusPHP capability (Harem/Messages/Detail)")
	}
	// 通用项仍填充
	if def.SearchEntry != "torrents.php" {
		t.Errorf("SearchEntry 通用默认应为 torrents.php, got %q", def.SearchEntry)
	}
}

// TestEnsureDefaultsIdempotent：同一 def 经 EnsureDefaults 两次，结果 reflect.DeepEqual。
// 覆盖空 schema、nexusphp、非 nexusphp 自定义选择器三种情形。
func TestEnsureDefaultsIdempotent(t *testing.T) {
	cases := []*SiteMetadata{
		{ID: "a", Schema: ""},
		{ID: "b", Schema: "nexusphp"},
		{ID: "c", Schema: "unit3d", Selectors: SiteSelectors{Rows: ElementQuery{Selector: "div.x tr"}}},
		{ID: "d", Schema: "nexusphp", Detail: DetailDef{Enabled: true, Fields: map[string]ElementQuery{
			"title": {Selector: "a.x"},
		}}},
		{ID: "e", Schema: "gazelle"},
		{ID: "f", Schema: "luminance"},
		{ID: "g", Schema: "mteam"},
	}
	for _, c := range cases {
		once := EnsureDefaults(c)
		twice := EnsureDefaults(once)
		if !reflect.DeepEqual(once, twice) {
			t.Errorf("schema=%q: EnsureDefaults 非幂等\n once=%+v\ntwice=%+v", c.Schema, once, twice)
		}
	}
}

// TestSiteMetadataDeadField：Dead 字段经 YAML 反序列化正确解析（死站保留）。
func TestSiteMetadataDeadField(t *testing.T) {
	const yamlStr = "id: ghost\nname: 鬼站\nschema: nexusphp\ndead: true\nurls:\n  - https://ghost.pt\n"
	var def SiteMetadata
	if err := yaml.Unmarshal([]byte(yamlStr), &def); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !def.Dead {
		t.Errorf("Dead 应为 true")
	}
	// 死站仍可正常 EnsureDefaults（加载不报错，仅信息保留）
	d := EnsureDefaults(&def)
	if d.Dead != true {
		t.Errorf("EnsureDefaults 不应清掉 Dead 标记")
	}
}

// TestAudiencesMedalDef 验证 audiences.yaml 勋章墙配置（实测校准 medal_center.php）。
// Audiences 是首个非默认路径的 NexusPHP 勋章站：shop_path=medal_center.php 须显式 enabled:true——
// EnsureDefaults 仅在 ShopPath 为空时才默认开启，自定义路径须手动开启，否则 chain 层跳过该站。
func TestAudiencesMedalDef(t *testing.T) {
	data, err := os.ReadFile("../../definitions/audiences.yaml")
	if err != nil {
		t.Skipf("definitions 不在测试相对路径下，跳过：%v", err)
	}
	var def SiteMetadata
	if err := yaml.Unmarshal(data, &def); err != nil {
		t.Fatalf("parse audiences.yaml: %v", err)
	}
	d := EnsureDefaults(&def)
	if d.Schema != "nexusphp" {
		t.Errorf("schema: got %q want nexusphp", d.Schema)
	}
	if !d.Medal.Enabled {
		t.Error("Medal.Enabled 必须为 true（自定义 shop_path 须显式开启，否则 chain 层跳过该站）")
	}
	if d.Medal.ShopPath != "medal_center.php" {
		t.Errorf("Medal.ShopPath: got %q want medal_center.php", d.Medal.ShopPath)
	}
	if d.Medal.OwnPath != "userdetails.php" {
		t.Errorf("Medal.OwnPath: got %q want userdetails.php (EnsureDefaults 默认)", d.Medal.OwnPath)
	}
}
