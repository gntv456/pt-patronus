package site

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func isEmptyElementQuery(q ElementQuery) bool {
	return q.Selector == "" && len(q.Selectors) == 0 && q.Text == "" && q.Data == "" &&
		q.Ancestor == "" && q.Find == "" && q.Attr == "" && q.Remove == "" && len(q.Filters) == 0
}

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

func TestTCGDefaults(t *testing.T) {
	def := EnsureDefaults(&SiteMetadata{
		ID:     "tcg-demo",
		Schema: "tcg",
		Selectors: SiteSelectors{Fields: map[string]ElementQuery{
			"category": {Selector: `a[href^="browse.php?cat="]`},
		}},
	})
	if def.SearchEntry != "browse.php" || def.SearchParam != "search" {
		t.Fatalf("search defaults = entry %q param %q", def.SearchEntry, def.SearchParam)
	}
	for _, key := range []string{"incldead", "titleonly", "nonboolean"} {
		if def.ExtraParam[key] == "" {
			t.Fatalf("ExtraParam[%s] missing", key)
		}
	}
	if def.Selectors.Rows.Selector == "" {
		t.Fatalf("Rows selector should be populated")
	}
	for _, field := range []string{"title", "detail", "download", "size", "seeders", "leechers", "category"} {
		if isEmptyElementQuery(def.Selectors.Fields[field]) {
			t.Fatalf("field %s should be present", field)
		}
	}
	if !userInfoPagesCoverField(def.UserInfo.Pages, "id") || !userInfoPagesCoverField(def.UserInfo.Pages, "seeding_size") {
		t.Fatalf("TCG user info pages should cover id and seeding_size")
	}
	if def.Freeleech.Enabled || def.Harem.Enabled || def.Messages.Enabled || def.Detail.Enabled {
		t.Errorf("TCG defaults should not enable NexusPHP capabilities")
	}
}

func TestSmallHorseDefaults(t *testing.T) {
	def := EnsureDefaults(&SiteMetadata{ID: "sh", Schema: "smallhorse"})
	if def.Selectors.Rows.Selector != "table#torrent_table tr" {
		t.Errorf("Rows 应为 table#torrent_table tr, got %q", def.Selectors.Rows.Selector)
	}
	for _, field := range []string{"title", "detail", "download", "size", "seeders", "leechers"} {
		if isEmptyElementQuery(def.Selectors.Fields[field]) {
			t.Fatalf("field %s should be present", field)
		}
	}
	if len(def.UserInfo.Pages) != 3 {
		t.Fatalf("UserInfo.Pages = %d, want 3", len(def.UserInfo.Pages))
	}
	if !userInfoPagesCoverField(def.UserInfo.Pages, "id") {
		t.Fatalf("UserInfo.Pages should discover id")
	}
	if !userInfoPagesCoverField(def.UserInfo.Pages, "seeding_size") {
		t.Fatalf("UserInfo.Pages should cover seeding_size")
	}
	if def.UserInfo.Pages[2].Path != "torrents.php?type=seeding&userid={id}" {
		t.Fatalf("seeding page path = %q", def.UserInfo.Pages[2].Path)
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
		if _, ok := def.UserInfo.Fields["bonus_per_hour"]; !ok {
			t.Errorf("schema=%q: UserInfo.Fields 应含 NexusPHP bonus_per_hour 默认，供自定义 mybonus/bonus 页面复用", schema)
		}
		if _, ok := def.UserInfo.Fields["seeding_size"]; !ok {
			t.Errorf("schema=%q: UserInfo.Fields 应含 NexusPHP seeding_size 默认，供 userdetails/pages 复用", schema)
		}
		for _, field := range []string{"level_id", "avatar", "is_donor"} {
			if _, ok := def.UserInfo.DetailFields[field]; !ok {
				t.Errorf("schema=%q: UserInfo.DetailFields 应含 NexusPHP %s 默认", schema, field)
			}
		}
		if schema == "nexusphp" && !userInfoPagesCoverField(def.UserInfo.Pages, "personal_avatar") {
			t.Errorf("schema=%q: UserInfo.Pages 应含 personal_avatar 默认页", schema)
		}
		if schema == "" && userInfoPagesCoverField(def.UserInfo.Pages, "personal_avatar") {
			t.Errorf("schema=%q: UserInfo.Pages 不应为 generic 空 schema 追加 personal_avatar 默认页", schema)
		}
		if !def.Freeleech.Enabled || !def.Harem.Enabled || !def.Messages.Enabled || !def.Detail.Enabled {
			t.Errorf("schema=%q: capability 默认应开启 (Freeleech/Harem/Messages/Detail)", schema)
		}
	}
}

func userInfoPagesCoverField(pages []UserInfoPageDef, field string) bool {
	for _, page := range pages {
		if _, ok := page.Fields[field]; ok {
			return true
		}
		for _, key := range page.UseFields {
			if key == field {
				return true
			}
		}
	}
	return false
}

func TestNexusPHPSearchFieldsMergeDefaults(t *testing.T) {
	def := EnsureDefaults(&SiteMetadata{
		ID:     "partial",
		Schema: "nexusphp",
		Selectors: SiteSelectors{
			Rows: ElementQuery{Selector: "table.custom > tbody > tr"},
			Fields: map[string]ElementQuery{
				"category": {Selector: "td.cat"},
				"detail":   {Selector: "a.custom-detail", Attr: "href"},
			},
		},
	})
	if def.Selectors.Rows.Selector != "table.custom > tbody > tr" {
		t.Fatalf("custom rows should be preserved, got %q", def.Selectors.Rows.Selector)
	}
	if def.Selectors.Fields["category"].Selector != "td.cat" {
		t.Fatalf("custom category should be preserved, got %+v", def.Selectors.Fields["category"])
	}
	if def.Selectors.Fields["detail"].Selector != "a.custom-detail" {
		t.Fatalf("custom detail should be preserved, got %+v", def.Selectors.Fields["detail"])
	}
	for _, field := range []string{"title", "download", "size", "seeders", "leechers", "completed"} {
		if isEmptyElementQuery(def.Selectors.Fields[field]) {
			t.Fatalf("field %s should inherit NexusPHP default", field)
		}
	}
}

// TestMTeamDefaults：mteam schema 经 EnsureDefaults 后应默认放行 Freeleech/Announcement
// （适配器已实现 FetchFreeleech/FetchAnnouncement），但不注入 NexusPHP 选择器，也不开
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

func TestRartrackerDefaults(t *testing.T) {
	def := EnsureDefaults(&SiteMetadata{
		ID:     "superbits",
		Schema: "rartracker",
		Selectors: SiteSelectors{Fields: map[string]ElementQuery{
			"completed": {Selector: "timesCompleted"},
		}},
	})
	if def.SearchEntry != "/api/v1/torrents" || def.SearchParam != "searchText" || def.Format != "json" {
		t.Fatalf("search defaults = entry %q param %q format %q", def.SearchEntry, def.SearchParam, def.Format)
	}
	if def.ExtraParam["limit"] != "100" || def.ExtraParam["section"] != "all" {
		t.Fatalf("extra params = %#v", def.ExtraParam)
	}
	if def.Selectors.Rows.Selector != ":self" {
		t.Fatalf("rows = %q, want :self", def.Selectors.Rows.Selector)
	}
	for _, field := range []string{"title", "detail", "size", "seeders", "leechers", "completed", "category"} {
		if isEmptyElementQuery(def.Selectors.Fields[field]) {
			t.Fatalf("field %s should be present", field)
		}
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
		{ID: "h", Schema: "rartracker"},
		{ID: "i", Schema: "smallhorse"},
		{ID: "j", Schema: "tcg"},
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

func TestDefinitionUserInfoPageUseFieldsResolvable(t *testing.T) {
	dir := "../../definitions"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("definitions 不在测试相对路径下，跳过：%v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(dir + "/" + entry.Name())
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		var def SiteMetadata
		if err := yaml.Unmarshal(data, &def); err != nil {
			t.Fatalf("parse %s: %v", entry.Name(), err)
		}
		if def.ID == "" {
			continue
		}
		d := EnsureDefaults(&def)
		defined := map[string]bool{}
		for key := range d.UserInfo.Fields {
			defined[key] = true
		}
		for key := range d.UserInfo.DetailFields {
			defined[key] = true
		}
		for i, page := range d.UserInfo.Pages {
			for _, key := range page.UseFields {
				if _, ok := page.Fields[key]; ok {
					continue
				}
				if !defined[key] {
					t.Errorf("%s user_info.pages[%d].use_fields references missing %q", def.ID, i, key)
				}
			}
		}
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

func TestTNodeMedalDefaults(t *testing.T) {
	d := EnsureDefaults(&SiteMetadata{Schema: "tnode"})
	if !d.Medal.Enabled {
		t.Fatal("TNode medal capability must be enabled when the registered adapter is available")
	}
	if d.Medal.ShopPath != "medals.php" || d.Medal.OwnPath != "medals.php?action=owned" {
		t.Fatalf("unexpected TNode medal defaults: %+v", d.Medal)
	}
}

func TestKnownSitesWithoutMedalEndpointsStayDisabled(t *testing.T) {
	ids := []string{
		"52pt", "afun", "btschool", "chdbits", "hdarea", "hdhome", "hdsky", "hdupt",
		"opencd", "ourbits", "keepfrds", "pthome", "itzmx", "springsunday", "nanyangpt",
		"tmpt", "pttime", "zhuque",
	}
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			data, err := os.ReadFile("../../definitions/" + id + ".yaml")
			if err != nil {
				t.Fatalf("read definition: %v", err)
			}
			var def SiteMetadata
			if err := yaml.Unmarshal(data, &def); err != nil {
				t.Fatalf("parse definition: %v", err)
			}
			if got := EnsureDefaults(&def); got.Medal.Enabled {
				t.Fatalf("Medal.Enabled = true, want false after live endpoint audit")
			}
		})
	}
}
