package schemas

import "testing"

func TestSunnyParseNewsMD(t *testing.T) {
	md := "## 2025.10.17 - 关于一周年的一点消息\n\nSunny迎来了开站一周年！\n\n## 2025.01.01 - 关于规则\n\n本站将于14天后开启规则。\n"
	items := sunnyParseNewsMD(md)
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
	if items[0].Title != "关于一周年的一点消息" {
		t.Errorf("title0=%q", items[0].Title)
	}
	if items[0].Date != "2025-10-17" {
		t.Errorf("date0=%q", items[0].Date)
	}
	if items[1].Title != "关于规则" {
		t.Errorf("title1=%q", items[1].Title)
	}
}

func TestSunnyExtractContentField(t *testing.T) {
	raw := "xx\"content\":\"width=device-width\"yy\"content\":\"\\n## 2025.10.17 - 标题\\n\\n正文\\n\"zz"
	c := sunnyExtractContentField(raw)
	if c == "" {
		t.Fatal("empty content")
	}
	items := sunnyParseNewsMD(c)
	if len(items) != 1 || items[0].Title != "标题" {
		t.Fatalf("items=%+v", items)
	}
}
