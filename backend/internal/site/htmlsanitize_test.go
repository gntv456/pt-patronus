package site

import (
	"strings"
	"testing"
)

func TestSanitizeHTML(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		notContain string
	}{
		{"strips script", `<p>ok</p><script>alert(1)</script>`, "alert(1)"},
		{"strips onerror", `<img src=x onerror="alert(1)">`, "onerror"},
		{"strips onclick", `<a href="/" onclick="evil()">x</a>`, "onclick"},
		{"neutralizes js url", `<a href="javascript:alert(1)">x</a>`, "javascript:"},
		{"strips iframe", `<iframe src=evil></iframe><p>safe</p>`, "iframe"},
		{"strips svg", `<svg onload="alert(1)"></svg>`, "onload"},
	}
	for _, c := range cases {
		got := SanitizeHTML(c.in)
		if strings.Contains(got, c.notContain) {
			t.Errorf("%s: SanitizeHTML(%q) = %q still contains %q", c.name, c.in, got, c.notContain)
		}
	}
	// 保留安全排版标签
	if got := SanitizeHTML("<p>hello <b>world</b></p>"); !strings.Contains(got, "<b>world</b>") {
		t.Errorf("safe markup dropped: %q", got)
	}
	// 正常 https 链接保留
	if got := SanitizeHTML(`<a href="https://example.com/a">link</a>`); !strings.Contains(got, "https://example.com/a") {
		t.Errorf("safe url dropped: %q", got)
	}
	if SanitizeHTML("") != "" || SanitizeHTML("   ") != "" {
		t.Error("empty/blank should return empty")
	}
}
