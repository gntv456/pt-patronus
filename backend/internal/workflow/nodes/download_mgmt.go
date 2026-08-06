package nodes

import (
	"context"
	"fmt"
	"strings"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/workflow"
)

type dlMgmtNode struct {
	typ      string
	name     string
	desc     string
	icon     string
	dlCh     *chain.DownloadChain
	dlOp     *oper.DownloaderOper
	schema   []workflow.ParamField
	defaults map[string]any
	run      func(ctx context.Context, dl *chain.DownloadChain, id uint, hashes []string, p map[string]any, ecx *workflow.Context) error
}

func (n *dlMgmtNode) Type() string { return n.typ }

func (n *dlMgmtNode) Name() string { return n.name }

func (n *dlMgmtNode) Category() string { return "下载" }

func (n *dlMgmtNode) Description() string { return n.desc }

func (n *dlMgmtNode) Icon() string { return n.icon }

func (n *dlMgmtNode) DefaultParams() map[string]any { return n.defaults }

func (n *dlMgmtNode) ParamSchema() []workflow.ParamField { return n.schema }

func (n *dlMgmtNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	// 前置参数解析（哈希 / 分类 / 标签 / 路径 等空校验）放在 nil 依赖检查之前：
	// 既覆盖测试（nil 依赖 + 合法参数），也保证用户填错的图先于「服务未就绪」报出可读错误。
	hashes := resolveHashes(params, ecx)
	if early := n.runEarlyValidate(params, ecx, hashes); early != nil {
		return early
	}
	// dlOp/dlCh 为构造期注入，正常不会 nil；这里前置判 nil，避免测试与误用场景 panic。
	if n.dlOp == nil || n.dlCh == nil {
		return fmt.Errorf("下载器服务未初始化")
	}
	id, err := resolveDLID(params, n.dlOp)
	if err != nil {
		return err
	}
	return n.run(ctx, n.dlCh, id, hashes, params, ecx)
}

func (n *dlMgmtNode) runEarlyValidate(params map[string]any, ecx *workflow.Context, hashes []string) error {
	// 需要 hashes 的节点类型（list_torrents 列全部、set_speed_limit 全局限速 不需要）。
	switch n.typ {
	case "add_tracker":
		if strings.TrimSpace(render(workflow.ParamString(params, "hash"), ecx)) == "" {
			return fmt.Errorf("未提供任务哈希")
		}
		if len(nonEmptyLines(workflow.ParamString(params, "trackers"))) == 0 {
			return fmt.Errorf("Tracker 为空")
		}
	case "set_category":
		if len(hashes) == 0 {
			return fmt.Errorf("未提供任务哈希")
		}
		if workflow.ParamString(params, "category") == "" {
			return fmt.Errorf("分类为空")
		}
	case "add_tags":
		if len(hashes) == 0 {
			return fmt.Errorf("未提供任务哈希")
		}
		if len(workflow.ParamStringSlice(params, "tags")) == 0 {
			return fmt.Errorf("标签为空")
		}
	case "set_save_path":
		if len(hashes) == 0 {
			return fmt.Errorf("未提供任务哈希")
		}
		if workflow.ParamString(params, "location") == "" {
			return fmt.Errorf("路径为空")
		}
	case "pause_torrents", "resume_torrents", "recheck_torrents", "reannounce_torrents",
		"delete_torrents", "force_start", "set_torrent_limit", "set_share_limit":
		if len(hashes) == 0 {
			return fmt.Errorf("未提供任务哈希")
		}
	}
	return nil
}

func resolveDLID(params map[string]any, dlOp *oper.DownloaderOper) (uint, error) {
	id := uint(workflow.ParamInt(params, "downloader_id"))
	if id == 0 {
		id = defaultDownloaderID(dlOp)
		if id == 0 {
			return 0, fmt.Errorf("未配置下载器")
		}
	}
	return id, nil
}

func defaultDownloaderID(dlOp *oper.DownloaderOper) uint {
	list, err := dlOp.List()
	if err != nil || len(list) == 0 {
		return 0
	}
	for _, d := range list {
		if d.IsDefault {
			return d.ID
		}
	}
	return list[0].ID
}

func resolveHashes(params map[string]any, ecx *workflow.Context) []string {
	raw := strings.TrimSpace(render(workflow.ParamString(params, "hashes"), ecx))
	if raw == "" {
		if v, ok := ecx.Variables["torrent_hashes"]; ok {
			raw = strings.TrimSpace(fmt.Sprintf("%v", v))
		}
	}
	if raw == "" {
		return nil
	}
	var out []string
	for _, p := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t'
	}) {
		if h := strings.TrimSpace(p); h != "" {
			out = append(out, h)
		}
	}
	return out
}

func hashSchema() ([]workflow.ParamField, map[string]any) {
	return []workflow.ParamField{
			{Name: "downloader_id", Label: "下载器", Type: "downloader_select"},
			{Name: "hashes", Label: "任务哈希", Type: "textarea",
				Help: "逗号/换行分隔，支持 {{var.x}}；留空则用上一步「列出种子」的结果"},
		},
		map[string]any{"downloader_id": float64(0), "hashes": ""}
}
