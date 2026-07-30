package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/mcp"
	"github.com/hxpt/ptpatronus/internal/version"
)

// registerHostMCPTools 在 MCP 服务端注册宿主能力工具（只读优先，安全可暴露给 AI Agent）。
// 工具结果统一序列化为压缩 JSON 文本块，由 mcp.Server 包装进 MCP content。
func registerHostMCPTools(srv *mcp.Server, siteOp *oper.SiteOper, siteCh *chain.SiteChain,
	subCh *chain.SubscribeChain, dlOp *oper.DownloaderOper) {

	// system.info：版本 + 站点/订阅/下载器计数概要。
	srv.RegisterTool(mcp.Tool{
		Name:        "system.info",
		Description: "Return PTPatronus version and counts of configured sites, subscriptions and downloaders.",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		Handler: func(ctx context.Context, _ map[string]any) (string, error) {
			sites, _ := siteOp.List()
			subs, _ := subCh.List()
			dls, _ := dlOp.List()
			return mcpJSON(map[string]any{
				"app":           "PTPatronus",
				"version":       version.Version,
				"sites":         len(sites),
				"subscriptions": len(subs),
				"downloaders":   len(dls),
			}), nil
		},
	})

	// sites.list：列出已配置站点（不含密钥；cookie/token 在 model 里 json:"-"）。
	srv.RegisterTool(mcp.Tool{
		Name:        "sites.list",
		Description: "List configured PT sites (id, name, platform, base_url, username).",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		Handler: func(ctx context.Context, _ map[string]any) (string, error) {
			sites, err := siteOp.List()
			if err != nil {
				return "", err
			}
			return mcpJSON(sites), nil
		},
	})

	// subscriptions.list：列出媒体订阅。
	srv.RegisterTool(mcp.Tool{
		Name:        "subscriptions.list",
		Description: "List media subscriptions (title, type, year, source, status).",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		Handler: func(ctx context.Context, _ map[string]any) (string, error) {
			subs, err := subCh.List()
			if err != nil {
				return "", err
			}
			return mcpJSON(subs), nil
		},
	})

	// downloaders.list：列出下载器（不含密码，PasswordEnc json:"-"）。
	srv.RegisterTool(mcp.Tool{
		Name:        "downloaders.list",
		Description: "List downloaders (id, name, type, url, is_default).",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		Handler: func(ctx context.Context, _ map[string]any) (string, error) {
			dls, err := dlOp.List()
			if err != nil {
				return "", err
			}
			return mcpJSON(dls), nil
		},
	})

	// torrents.search：跨站搜索种子（只读；不自动下载）。
	srv.RegisterTool(mcp.Tool{
		Name:        "torrents.search",
		Description: "Search torrents across configured PT sites by keyword. Returns site/title/size/seeders/download_url. Read-only — does not download.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"keyword":  map[string]any{"type": "string", "description": "search keyword"},
				"category": map[string]any{"type": "string", "description": "movie | tv (optional)"},
			},
			"required": []string{"keyword"},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			keyword := strings.TrimSpace(fmt.Sprint(args["keyword"]))
			if keyword == "" || keyword == "<nil>" {
				return "", fmt.Errorf("keyword is required")
			}
			category := strings.TrimSpace(fmt.Sprint(args["category"]))
			if category == "<nil>" {
				category = ""
			}
			results := siteCh.SearchSites(ctx, keyword, category, nil)
			return mcpJSON(results), nil
		},
	})
}

// mcpJSON 序列化为压缩 JSON 字符串（mcp 工具结果文本块）。
func mcpJSON(v any) string { return mcp.CompactJSON(v) }
