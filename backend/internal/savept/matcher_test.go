package savept

import (
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// TestMatchCards URL/域名/名称匹配优先级 + 已绑定回填。
func TestMatchCards(t *testing.T) {
	cards := []SaveptSiteCard{
		{Name: "示例站", URL: "https://example.pt", Domain: "example.pt"},
		{Name: "未收录站", URL: "https://nowhere.pt", Domain: "nowhere.pt"},
		{Name: "仅名称匹配", URL: "https://foo.pt", Domain: "foo.pt"},
	}
	sites := []models.Site{
		{ID: 1, Name: "示例站", BaseURL: "https://example.pt"},     // URL 精确
		{ID: 2, Name: "其它站", BaseURL: "https://bar.example.pt"}, // 干扰
		{ID: 3, Name: "仅名称匹配的别名", BaseURL: "https://other.pt"},  // 名称模糊
	}
	matchCards(cards, sites)

	if cards[0].MatchType != MatchURLExact {
		t.Errorf("card0 match: got %q want url_exact", cards[0].MatchType)
	}
	if cards[0].LocalSiteID != 1 || cards[0].LocalSiteName != "示例站" {
		t.Errorf("card0 bound site: got id=%d name=%q", cards[0].LocalSiteID, cards[0].LocalSiteName)
	}
	if cards[1].MatchType != MatchNone {
		t.Errorf("card1 match: got %q want none", cards[1].MatchType)
	}
	if cards[2].MatchType != MatchNameFuzzy {
		t.Errorf("card2 match: got %q want name_fuzzy", cards[2].MatchType)
	}
}

// TestSummarize 汇总统计。
func TestSummarize(t *testing.T) {
	cards := []SaveptSiteCard{
		{Status: HealthHealthy, MatchType: MatchURLExact, SiteType: TypeInternal, Year: "2020", AnniversaryText: "今天站庆"},
		{Status: HealthCritical, MatchType: MatchNameFuzzy, SiteType: TypeInternal, Year: "2018"},
		{Status: HealthClosed, MatchType: MatchNone, SiteType: TypeExternal, Year: "2019"},
	}
	s := summarize(cards)
	if s.Total != 3 || s.Healthy != 1 || s.Critical != 1 || s.Closed != 1 {
		t.Errorf("counts: %+v", s)
	}
	if s.Bound != 1 || s.Available != 1 {
		t.Errorf("match counts: bound=%d available=%d", s.Bound, s.Available)
	}
	// 细分统计
	if s.Internal != 2 || s.External != 1 {
		t.Errorf("type counts: internal=%d external=%d", s.Internal, s.External)
	}
	if s.TodayAnniv != 1 {
		t.Errorf("today_anniv: got %d want 1", s.TodayAnniv)
	}
	if s.Years != 3 {
		t.Errorf("years: got %d want 3", s.Years)
	}
	if s.MpOwned != 1 || s.MpAvailable != 1 || s.MpUnsupported != 1 {
		t.Errorf("mp counts: owned=%d available=%d unsupported=%d", s.MpOwned, s.MpAvailable, s.MpUnsupported)
	}
}

// TestSummarizeCountsUnknown 探测不明的站必须进汇总：
// 回归前只数 健康/病危/已关 三项，unknown（连通性探测失败、上游没给状态）
// 被静默漏掉，头部四个数字里 healthy+critical+closed < total，
// 用户对不上数却不知道少的是哪一类。
func TestSummarizeCountsUnknown(t *testing.T) {
	cards := []SaveptSiteCard{
		{Status: HealthHealthy},
		{Status: HealthCritical},
		{Status: HealthClosed},
		{Status: HealthUnknown},
		{Status: ""}, // 上游缺状态同样归入不明，不能凭空消失
	}
	s := summarize(cards)
	if s.Total != 5 {
		t.Fatalf("total=%d want 5", s.Total)
	}
	if s.Unknown != 2 {
		t.Fatalf("unknown=%d want 2（含缺状态）", s.Unknown)
	}
	if s.Healthy+s.Critical+s.Closed+s.Unknown != s.Total {
		t.Errorf("四项必须对得上总数：+%v", s)
	}

	// 正控：全是已知状态时不明为 0（证明不是把所有卡都塞进不明凑数）。
	s2 := summarize([]SaveptSiteCard{{Status: HealthHealthy}, {Status: HealthClosed}})
	if s2.Unknown != 0 || s2.Total != 2 || s2.Healthy+s2.Closed != 2 {
		t.Errorf("无不明卡时不应计数：%+v", s2)
	}
}
