package groupchat

import "testing"

func TestParseSitesMessages(t *testing.T) {
	raw := "好学|求魔力|求上传\n# 这是一行注释\n藏宝阁|求上传|60s\n\n织梦|求电力|求VIP|90s"
	out := ParseSitesMessages(raw)
	if len(out) != 3 {
		t.Fatalf("expected 3 entries (注释+空行跳过), got %d: %#v", len(out), out)
	}
	if out[0].SiteName != "好学" || len(out[0].Messages) != 2 || out[0].Messages[0] != "求魔力" {
		t.Fatalf("entry 0 wrong: %#v", out[0])
	}
	if out[1].Interval != 60 {
		t.Fatalf("entry 1 interval should be 60, got %d", out[1].Interval)
	}
	if out[2].Interval != 90 || len(out[2].Messages) != 2 {
		t.Fatalf("entry 2 wrong: %#v", out[2])
	}
}

func TestParseSitesMessagesSkipsMalformed(t *testing.T) {
	raw := "仅站名无消息\n\n|\n#纯注释"
	out := ParseSitesMessages(raw)
	if len(out) != 0 {
		t.Fatalf("expected 0 valid entries, got %d: %#v", len(out), out)
	}
}

func TestMessagesForSite(t *testing.T) {
	all := []SiteMessages{
		{SiteName: "好学", Messages: []string{"求魔力", "求上传"}, Interval: 30},
		{SiteName: "好学", Messages: []string{"求VIP"}}, // 同名合并
	}
	msgs, interval := MessagesForSite(all, SiteRef{Name: "好学PT"})
	if len(msgs) != 3 {
		t.Fatalf("expected 3 merged messages, got %d: %#v", len(msgs), msgs)
	}
	if interval != 30 {
		t.Fatalf("expected interval 30, got %d", interval)
	}
}

func TestMatchSiteByName(t *testing.T) {
	sites := []SiteRef{{ID: 1, Name: "好学PT"}, {ID: 2, Name: "织梦"}}
	if s, ok := MatchSiteByName(sites, "好学"); !ok || s.ID != 1 {
		t.Fatalf("expected match 好学PT, got %+v ok=%v", s, ok)
	}
	if _, ok := MatchSiteByName(sites, "不存在站"); ok {
		t.Fatal("should not match unknown name")
	}
}
