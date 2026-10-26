package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/errtext"
	"github.com/hxpt/ptpatronus/internal/mcp"
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
			return mcpJSON(systemInfoPayload(siteOp, subCh, dlOp)), nil
		},
	})

	// sites.list：列出已配置站点（不含密钥；cookie/token 在 model 里 json:"-"）。
	// 载荷与 agent 工具共用 sitesListPayload：精简行 + total + duplicates（同注册域多站点）。
	srv.RegisterTool(mcp.Tool{
		Name:        "sites.list",
		Description: "List configured PT sites (id, name, platform, base_url, username). Includes 'total' and a 'duplicates' map of sites sharing the same registered domain (same site added twice).",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		Handler: func(ctx context.Context, _ map[string]any) (string, error) {
			payload, err := sitesListPayload(siteOp)
			return mcpJSON(payload), err
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
				return "", mcpErr(err)
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
				return "", mcpErr(err)
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
			results, err := siteCh.SearchSites(ctx, keyword, category, nil)
			if err != nil {
				return "", mcpErr(err)
			}
			return mcpJSON(results), nil
		},
	})
}

// mcpJSON 序列化为压缩 JSON 字符串（mcp 工具结果文本块）。
func mcpJSON(v any) string { return mcp.CompactJSON(v) }

// mcpErr 是 MCP/AI 工具失败的统一文案出口（#350）。
//
// 工具 Handler 返回的 err 会被 mcp.Server 原样转成 isError=true 的内容块交给模型，
// 因此链层/oper 的任何一句英文原文都会直接进 LLM 上下文——模型既不知道该改哪个
// 参数，也不知道下一步做什么，只能反复重试同一把工具。
//
// 这里刻意做成 errtext 的薄封装而不是另写翻译表：errtext 已是全仓文案的单一来源
// （识别得了的哨兵翻成可操作中文；站点适配器/下载器客户端自己的原文原样透传——
// 那是用户唯一的线索，不该被二次加工）。再写一份必然与它漂移。
//
// nil 回空串：mcpErr 的调用点都是 `return mcpJSON(x), mcpErr(err)`，nil 时必须
// 返回真正的 nil error，而不是 "<nil>" 这种假错误把成功报成失败。
func mcpErr(err error) error {
	if err == nil {
		return nil
	}
	msg := errtext.Text(err)
	if msg == "" {
		return nil
	}
	return errors.New(msg)
}
