package plugins

import "testing"

// 接入地址来自插件动作的「查看接入地址」：客户端会照着它填。base_url 既可能是
// 完整地址（帮助文案的示例），也可能只是站点根/反代前缀——两种都必须得到同一个
// 可用的 /api/v1/mcp 地址，不能出现 /api/v1/mcp/api/v1/mcp。
func TestMCPEndpointURL(t *testing.T) {
	cases := map[string]string{
		"":                                  "/api/v1/mcp",
		"   ":                               "/api/v1/mcp",
		"https://pt.example.com:8088":       "https://pt.example.com:8088/api/v1/mcp",
		"https://pt.example.com:8088/":      "https://pt.example.com:8088/api/v1/mcp",
		"https://pt.example.com/api/v1/mcp": "https://pt.example.com/api/v1/mcp",
		"https://pt.example.com/mcp/":       "https://pt.example.com/mcp/api/v1/mcp",
	}
	for in, want := range cases {
		if got := mcpEndpointURL(in); got != want {
			t.Errorf("mcpEndpointURL(%q) = %q, want %q", in, got, want)
		}
	}
}
