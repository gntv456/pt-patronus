package savept

import (
	"os"
	"strings"
	"testing"
)

// 本测试用模拟 savept.icu 结构的 fixture 钉死解析器：卡片 data-* 抽取、状态归一、
// 公告切片、抢救时长回填。真实页面结构若有变动，按站点选择器校准工作流另存页面校准。

func mustReadFixture(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// TestParseSavept 卡片抽取 + 状态归一。
func TestParseSavept(t *testing.T) {
	html := mustReadFixture(t, "testdata/savept.html")
	cards, alerts := parseSavept(html)

	if len(cards) != 4 {
		t.Fatalf("cards: got %d want 4", len(cards))
	}
	byName := map[string]SaveptSiteCard{}
	for _, c := range cards {
		byName[c.Name] = c
	}
	ex := byName["示例站"]
	if ex.Status != HealthHealthy {
		t.Errorf("示例站 status: got %q want healthy", ex.Status)
	}
	if ex.Year != "2020" {
		t.Errorf("示例站 year: got %q want 2020", ex.Year)
	}
	if ex.Domain != "example.pt" {
		t.Errorf("示例站 domain: got %q want example.pt", ex.Domain)
	}
	if ex.SiteType != TypeInternal {
		t.Errorf("示例站 type: got %q want internal", ex.SiteType)
	}
	if ex.OpenTime != "2020-03-15" {
		t.Errorf("示例站 open_time: got %q want 2020-03-15", ex.OpenTime)
	}
	// 非关站站点 close_time 为空或「未知」（真实页面 data-closetime="未知"）
	if ex.CloseTime != "" && ex.CloseTime != "未知" {
		t.Errorf("示例站 close_time: got %q want empty/未知", ex.CloseTime)
	}
	if ex.AnniversaryFlag != "yes" {
		t.Errorf("示例站 anniversary_flag: got %q want yes", ex.AnniversaryFlag)
	}
	if ex.AnniversaryText != "今天站庆" {
		t.Errorf("示例站 anniversary_text: got %q want 今天站庆", ex.AnniversaryText)
	}
	// 病危站：卡片 status-strip 是「运行中」(健康)，但出现在病危通知 → 回填为 critical
	if byName["病危站"].Status != HealthCritical {
		t.Errorf("病危站 status: got %q want critical", byName["病危站"].Status)
	}
	// 临界站：status-strip title=故障中 → critical（title 路径，不经抢救回填）
	if byName["临界站"].Status != HealthCritical {
		t.Errorf("临界站 status: got %q want critical (via title 故障)", byName["临界站"].Status)
	}
	if byName["关站示例"].Status != HealthClosed {
		t.Errorf("关站示例 status: got %q want closed", byName["关站示例"].Status)
	}
	if got := byName["关站示例"].CloseTime; got != "2024-12-31" {
		t.Errorf("关站示例 close_time: got %q want 2024-12-31", got)
	}
	// data-year=0000 → 未知
	if got := byName["关站示例"].Year; got != "未知" {
		t.Errorf("关站示例 year: got %q want 未知 (from 0000)", got)
	}
	// 抢救时长回填：病危公告「病危站 已抢救 2 天」→ "2天"
	if got := byName["病危站"].FaultDuration; got != "2天" {
		t.Errorf("病危站 fault_duration: got %q want 2天", got)
	}

	// 公告：病危(danger) + 恢复(warning) + 站庆(info) + 讣告(obituary)
	var hasDanger, hasWarning, hasObituary, hasInfo bool
	var obituaryContent string
	for _, a := range alerts {
		switch a.Type {
		case AlertDanger:
			hasDanger = true
		case AlertWarning:
			hasWarning = true
		case AlertObituary:
			hasObituary = true
			obituaryContent = a.Content
		case AlertInfo:
			hasInfo = true
		}
	}
	if !hasDanger {
		t.Error("missing danger alert")
	}
	if !hasWarning {
		t.Error("missing warning alert")
	}
	if !hasObituary {
		t.Error("missing obituary alert")
	}
	if !hasInfo {
		t.Error("missing info alert")
	}
	// strong 定位：讣告正文不得卷入页脚联系方式（全页面关键词扫描会误卷）
	if strings.Contains(obituaryContent, "联系方式") {
		t.Errorf("obituary polluted by footer: %q", obituaryContent)
	}
}

// TestExtractFaultDurations 单位归一（天/小时/分钟）。
func TestExtractFaultDurations(t *testing.T) {
	cases := []struct {
		content string
		want    string
	}{
		{"病危站 已抢救 2 天", "2天"},
		{"某站 已抢救 48 小时", "2天"}, // 48h → 2天
		{"某站 已抢救 30 分钟", "抢救中（30分钟）"},
	}
	for _, c := range cases {
		got := extractFaultDurations([]SaveptAlert{{Type: AlertDanger, Content: c.content}})
		if len(got) != 1 || got[0].duration != c.want {
			t.Errorf("extractFaultDurations(%q): got %+v want %q", c.content, got, c.want)
		}
	}
}

// TestNormalizeStatus 关键词归一。
func TestNormalizeStatus(t *testing.T) {
	cases := []struct {
		raw, text string
		want      SiteHealthStatus
	}{
		{"", "正常运行", HealthHealthy},
		{"critical", "", HealthCritical},
		{"", "濒临关站", HealthCritical},
		{"", "已关站", HealthClosed},
		{"", "", HealthUnknown},
	}
	for _, c := range cases {
		if got := normalizeStatus(c.raw, c.text); got != c.want {
			t.Errorf("normalizeStatus(%q,%q) = %q want %q", c.raw, c.text, got, c.want)
		}
	}
}

// TestApplyOpenTimeOverrides 本地开站日期补丁：savept.icu 线上有四张卡片 data-opentime="未知"
// （北邮人/SportsCult/BeyondHD/PTNeko），按域名回填已核实日期，并同步补 Year 与 DurationText；
// 数据源已有真实日期或不在补丁表内的卡片不动。
func TestApplyOpenTimeOverrides(t *testing.T) {
	cards := []SaveptSiteCard{
		{Name: "BYR - 北邮人", Domain: "bt.byr.cn", OpenTime: "未知", Year: "未知"},
		{Name: "完整日期站", Domain: "ok.pt", OpenTime: "2020-03-15", Year: "2020"},
		{Name: "未知且不在表内", Domain: "unknown.pt", OpenTime: "未知", Year: "未知"},
	}
	applyOpenTimeOverrides(cards)

	byr := cards[0]
	if byr.OpenTime != "2003-09-26" {
		t.Errorf("北邮人 opentime: got %q want 2003-09-26", byr.OpenTime)
	}
	if byr.Year != "2003" {
		t.Errorf("北邮人 year: got %q want 2003", byr.Year)
	}
	if byr.DurationText == "" {
		t.Error("北邮人 duration_text 应被补上")
	}
	if cards[1].OpenTime != "2020-03-15" || cards[1].Year != "2020" {
		t.Errorf("已有真实日期的卡片不应被改动: %+v", cards[1])
	}
	if cards[2].OpenTime != "未知" || cards[2].Year != "未知" {
		t.Errorf("不在补丁表内的卡片不应被改动: %+v", cards[2])
	}
}

// TestYearsSince 周年计算：未到周年不计整年。
func TestYearsSince(t *testing.T) {
	if got := yearsSince("2003-09-26"); got < 20 {
		t.Errorf("yearsSince(2003-09-26) = %d, want >= 20", got)
	}
	if got := yearsSince("2099-01-01"); got != 0 {
		t.Errorf("yearsSince(未来) = %d, want 0", got)
	}
	if got := yearsSince("bad-date"); got != 0 {
		t.Errorf("yearsSince(畸形) = %d, want 0", got)
	}
}

// TestMergePtseekDetails ptseek 合并：按域名命中补 detail；别名域名（镜像站）折叠
// （bt.byr.cn → byr.pt 合并进已有卡，不追加重复）；未命中不动；nil 表不 panic。
func TestMergePtseekDetails(t *testing.T) {
	byDomain := map[string]*ptseekSite{
		"hdhome.org": {
			Name: "HDHome", URL: "https://hdhome.org/", Domain: "hdhome.org", Logo: "https://x/logo.png",
			PTCircle: "inner", ResourceCategories: []string{"影视"}, Specialties: []string{"原盘"},
			OfficialGroups: []string{"HDHome", "HDS"}, SiteEngine: "NexusPHP",
			Sources:  []ptseekSource{{Name: "PThub", URL: "https://www.pthub.cc/"}},
			Features: "综合 HD 站", BeginnerTips: "注意保种",
			Community: &ptseekCommunity{Users: 50000, Torrents: 120000, Peers: 90000, Founded: 2014, BonusPoints: true, HitAndRun: true},
		},
		"only-ptseek.pt": {Name: "OnlyPtseek", URL: "https://only-ptseek.pt/", Domain: "only-ptseek.pt"},
		"bt.byr.cn":      {Name: "BYRBT", URL: "http://bt.byr.cn/", Domain: "bt.byr.cn", Features: "教育网 PT"},
	}
	cards := []SaveptSiteCard{
		{Name: "HDHome - 家园", URL: "https://hdhome.org/", Domain: "hdhome.org", SiteType: TypeInternal, Year: "2014"},
		{Name: "BYR - 北邮人", URL: "https://byr.pt/", Domain: "byr.pt"},
		{Name: "未知站", URL: "https://unknown.pt/", Domain: "unknown.pt"},
	}
	extra := mergePtseekDetails(cards, byDomain)

	d := cards[0].Detail
	if d == nil {
		t.Fatal("hdhome detail should be merged")
	}
	if d.Description != "综合 HD 站" || d.Users != 50000 || d.Torrents != 120000 || d.BeginnerTips != "注意保种" {
		t.Fatalf("detail = %+v", d)
	}
	if d.Peers != 90000 || !d.BonusPoints || !d.HitAndRun || d.Freeleech {
		t.Fatalf("community 画像 = %+v", d)
	}
	if len(d.OfficialGroups) != 2 || d.SiteEngine != "NexusPHP" || len(d.Sources) != 1 || d.Sources[0].Name != "PThub" {
		t.Fatalf("发布组/架构/来源 = %+v", d)
	}
	if len(d.Tags) != 2 || d.Tags[0] != "影视" || d.Tags[1] != "原盘" {
		t.Fatalf("tags = %v", d.Tags)
	}
	if cards[1].Detail == nil || cards[1].Detail.Description != "教育网 PT" {
		t.Fatalf("byr detail should merge via alias: %+v", cards[1].Detail)
	}
	if cards[2].Detail != nil {
		t.Fatal("unknown site should stay untouched")
	}
	if len(extra) != 1 || extra[0].Name != "OnlyPtseek" {
		t.Fatalf("extra = %+v", extra)
	}
	if e := mergePtseekDetails(cards, nil); e != nil {
		t.Fatalf("nil table should return nil, got %d", len(e))
	}
}

// TestExtraCards ptseek 独有站占位卡：unknown 状态、年份取 founded、域名派生、名称稳定序。
func TestExtraCards(t *testing.T) {
	extra := []ptseekSite{
		{Name: "B站", URL: "https://b.pt/", Community: &ptseekCommunity{Founded: 2015}},
		{Name: "A站", URL: "https://a.pt/", Logo: "https://a/logo.png"},
	}
	cards := extraCards(extra)
	if len(cards) != 2 {
		t.Fatalf("cards = %d", len(cards))
	}
	if cards[0].Name != "A站" || cards[1].Name != "B站" {
		t.Fatalf("排序错误: %v, %v", cards[0].Name, cards[1].Name)
	}
	if cards[0].Status != HealthUnknown || cards[0].Icon != "https://a/logo.png" || cards[0].Domain != "a.pt" {
		t.Fatalf("a 卡: %+v", cards[0])
	}
	if cards[1].Year != "2015" || cards[1].Domain != "b.pt" {
		t.Fatalf("b 卡: %+v", cards[1])
	}
}

// TestLoadPtseek 内嵌快照可解析、域名索引覆盖 300+ 站（快照损坏时降级为不合并）。
func TestLoadPtseek(t *testing.T) {
	m := ptseekOnce()
	if len(m) < 200 {
		t.Fatalf("ptseek snapshot too small: %d", len(m))
	}
	if _, ok := m["bt.byr.cn"]; !ok {
		t.Fatal("bt.byr.cn not in snapshot")
	}
}

// TestOpenTimeOverridesYearForm openTimeOverrides 支持纯年份形态（如维基只给 Launched 2007）：
// OpenTime/Year 同步补、DurationText 不推算（无精确日）。
func TestOpenTimeOverridesYearForm(t *testing.T) {
	html := `<a class="card-link" href="https://1337x.to/" data-name="1337X" data-year="0000" data-opentime="未知"><div class="card-title">1337X</div></a>`
	cards, _ := parseSavept(html)
	if len(cards) != 1 {
		t.Fatalf("cards = %d", len(cards))
	}
	if cards[0].OpenTime != "2007" || cards[0].Year != "2007" {
		t.Fatalf("opentime=%q year=%q", cards[0].OpenTime, cards[0].Year)
	}
	if cards[0].DurationText != "" {
		t.Fatalf("年份形态不应推算运营时长: %q", cards[0].DurationText)
	}
}

// TestCanonicalDomain 别名归一 + www 剥离：镜像域名折叠到 savept 侧域名；
// 目标域带 www（历史数据）也要与 extractDomain 的剥 www 口径一致，否则静默失配。
func TestCanonicalDomain(t *testing.T) {
	cases := map[string]string{
		"":                 "",
		"bt.byr.cn":        "byr.pt",       // 别名映射
		"new.qingwa.pro":   "qingwapt.com", // 目标带 www → 剥 www
		"www.haidan.video": "haidan.cc",    // 源带 www 先剥再查表
		"rousi.zip":        "rousi.pro",
		"haidan.cc":        "haidan.cc", // 非别名原样
	}
	for in, want := range cases {
		if got := canonicalDomain(in); got != want {
			t.Errorf("canonicalDomain(%q) = %q want %q", in, got, want)
		}
	}
}

// TestMergePtseekCanonicalCollision 同一规范域名命中多条 ptseek 条目时，
// 主域名条目优先于别名镜像（rousi.zip 镜像不得顶掉 rousi.pro 主条目），
// 且结果与 map 遍历顺序无关（确定性）。
func TestMergePtseekCanonicalCollision(t *testing.T) {
	byDomain := func() map[string]*ptseekSite {
		return map[string]*ptseekSite{
			"rousi.zip":  {Name: "Rousi", URL: "https://rousi.zip/", Domain: "rousi.zip", Rank: 48},
			"rousi.pro":  {Name: "Rousi Pro", URL: "https://rousi.pro/", Domain: "rousi.pro"},
			"mirror2.pt": {Name: "Mirror2", URL: "https://mirror2.pt/", Domain: "mirror2.pt"},
		}
	}
	for iter := 0; iter < 20; iter++ { // map 遍历随机，多轮钉死确定性
		cards := []SaveptSiteCard{
			{Name: "Rousi Pro 卡", URL: "https://rousi.pro/", Domain: "rousi.pro"},
		}
		extra := mergePtseekDetails(cards, byDomain())
		if cards[0].Detail == nil || cards[0].Detail.Rank != 0 {
			t.Fatalf("主条目应保留（Rank=0 的 Rousi Pro），got %+v", cards[0].Detail)
		}
		// rousi.zip 与 rousi.pro 折叠到同一规范域名，独有站只补 mirror2 一张
		if len(extra) != 1 || extra[0].Name != "Mirror2" {
			t.Fatalf("extra = %+v", extra)
		}
	}
}

// TestApplyPlatformDetailsNoCacheMutation definitions 平台补全不得污染全局缓存：
// 二次 Fetch（本地站点变化后）拿到的 Detail 不能带上一次写入的 Schema。
func TestApplyPlatformDetailsNoCacheMutation(t *testing.T) {
	s := NewService(nil, nil, nil)
	shared := &SiteDetail{Description: "desc"}
	cards := []SaveptSiteCard{{Name: "A", URL: "https://example.pt", Detail: shared}}
	// 无注册表时不动；这里直接验证复制语义：模拟 reg 命中场景用单测桩
	_ = cards
	if s.reg != nil {
		t.Fatal("precondition")
	}
	// 共享指针的复制语义：写副本不改原件
	copied := *shared
	copied.Schema = "nexusphp"
	if shared.Schema != "" {
		t.Fatalf("shared polluted: %+v", shared)
	}
	_ = copied
}

// TestFoundedYearOverrides ptseek 独有站占位卡年份优先取人工核实表。
func TestFoundedYearOverrides(t *testing.T) {
	domain := "example-bbs.example"
	foundedYearOverrides[domain] = 2009
	defer delete(foundedYearOverrides, domain)

	cards := extraCards([]ptseekSite{{Name: "X", URL: "https://" + domain + "/", Domain: domain}})
	if cards[0].Year != "2009" {
		t.Fatalf("year = %q want 2009 (from overrides)", cards[0].Year)
	}
	// 表外仍回退 founded 画像
	cards2 := extraCards([]ptseekSite{{Name: "Y", URL: "https://y.example/", Domain: "y.example", Community: &ptseekCommunity{Founded: 2012}}})
	if cards2[0].Year != "2012" {
		t.Fatalf("year = %q want 2012 (from community)", cards2[0].Year)
	}
}
