package plugins

import "testing"

// TestNormalizeMarketSource 覆盖 GitHub 仓库 URL → raw 转换、简写、tree 分支/子目录、以及
// raw/file/其它 http(s) 源原样返回等分支。
func TestNormalizeMarketSource(t *testing.T) {
	cases := []struct{ in, want string }{
		// GitHub 仓库页 → raw 默认分支 main / 根目录 plugin-market.json
		{"https://github.com/owner/repo", "https://raw.githubusercontent.com/owner/repo/main/plugin-market.json"},
		{"https://github.com/owner/repo/", "https://raw.githubusercontent.com/owner/repo/main/plugin-market.json"},
		// tree/{branch} 与 tree/{branch}/{path}
		{"https://github.com/owner/repo/tree/dev", "https://raw.githubusercontent.com/owner/repo/dev/plugin-market.json"},
		{"https://github.com/owner/repo/tree/dev/market", "https://raw.githubusercontent.com/owner/repo/dev/market/plugin-market.json"},
		// 无 scheme 与 owner/repo 简写
		{"github.com/owner/repo", "https://raw.githubusercontent.com/owner/repo/main/plugin-market.json"},
		{"owner/repo", "https://raw.githubusercontent.com/owner/repo/main/plugin-market.json"},
		{"https://www.github.com/owner/repo", "https://raw.githubusercontent.com/owner/repo/main/plugin-market.json"},
		// 已是 raw / 其它 http 源 / 本地路径 → 原样
		{"https://raw.githubusercontent.com/owner/repo/main/plugin-market.json", "https://raw.githubusercontent.com/owner/repo/main/plugin-market.json"},
		{"https://example.com/market.json", "https://example.com/market.json"},
		{"/local/path/plugin-market.json", "/local/path/plugin-market.json"},
		{"", ""},
	}
	for _, c := range cases {
		got := normalizeMarketSource(c.in)
		if got != c.want {
			t.Errorf("normalizeMarketSource(%q)\n  got  %q\n  want %q", c.in, got, c.want)
		}
	}
}
