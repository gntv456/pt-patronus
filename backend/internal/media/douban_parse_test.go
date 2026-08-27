package media

import "testing"

// parseCardSubtitle 把豆瓣榜单 card_subtitle（年/产地/类型/导演/主演）结构化，
// 供 Trending 填 Genres/Countries/Directors——卡片字段与其他源统一。
func TestParseCardSubtitle(t *testing.T) {
	g, c, n := parseCardSubtitle("2026 / 中国大陆 / 剧情 古装 / 邓科 / 丁禹兮 邓恩熙")
	if len(c) != 1 || c[0] != "中国大陆" {
		t.Errorf("countries = %v", c)
	}
	if len(g) != 2 || g[0] != "剧情" || g[1] != "古装" {
		t.Errorf("genres = %v", g)
	}
	if len(n) != 1 || n[0] != "邓科" {
		t.Errorf("names = %v", n)
	}
	// 仅 年/产地/类型 三段：无导演
	g, c, n = parseCardSubtitle("1994 / 美国 / 剧情")
	if len(g) != 1 || len(c) != 1 || n != nil {
		t.Errorf("short subtitle: %v %v %v", g, c, n)
	}
	// 两段（缺类型）不结构化
	if g, c, n = parseCardSubtitle("2026 / 美国"); g != nil || c != nil || n != nil {
		t.Errorf("two-segment should be nil: %v %v %v", g, c, n)
	}
}
