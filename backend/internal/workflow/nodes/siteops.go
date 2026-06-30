package nodes

import (
	"context"
	"fmt"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/workflow"
)

// CheckinNode 对指定站点签到（SiteChain.Checkin）。
type CheckinNode struct {
	siteCh *chain.SiteChain
	siteOp *oper.SiteOper
}

func NewCheckinNode(siteCh *chain.SiteChain, siteOp *oper.SiteOper) *CheckinNode {
	return &CheckinNode{siteCh: siteCh, siteOp: siteOp}
}

func (n *CheckinNode) Type() string        { return "checkin" }
func (n *CheckinNode) Name() string        { return "签到" }
func (n *CheckinNode) Category() string    { return "站点" }
func (n *CheckinNode) Description() string { return "对指定站点执行签到" }
func (n *CheckinNode) Icon() string        { return "checkin" }
func (n *CheckinNode) DefaultParams() map[string]any {
	return map[string]any{"site_id": float64(0)}
}
func (n *CheckinNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "site_id", Label: "站点", Type: "site_select", Required: true},
	}
}

func (n *CheckinNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	sid := workflow.ParamInt(params, "site_id")
	if sid <= 0 {
		return fmt.Errorf("未选择站点")
	}
	s, err := n.siteOp.Get(uint(sid))
	if err != nil || s == nil {
		return fmt.Errorf("站点不存在")
	}
	res, err := n.siteCh.Checkin(ctx, s, "manual")
	if err != nil {
		return fmt.Errorf("签到失败: %w", err)
	}
	if res != nil && res.Success {
		ecx.Messages = append(ecx.Messages, fmt.Sprintf("签到成功：%s（+%v）", s.Name, res.Bonus))
	} else {
		msg := "签到失败"
		if res != nil {
			msg = res.Message
		}
		ecx.Messages = append(ecx.Messages, fmt.Sprintf("签到：%s → %s", s.Name, msg))
	}
	return nil
}

// ReseedNode 对指定下载器执行 IYUU 辅种（KeepSeedChain.Reseed）。
type ReseedNode struct {
	keepSeed *chain.KeepSeedChain
}

func NewReseedNode(keepSeed *chain.KeepSeedChain) *ReseedNode { return &ReseedNode{keepSeed: keepSeed} }

func (n *ReseedNode) Type() string        { return "reseed" }
func (n *ReseedNode) Name() string        { return "辅种" }
func (n *ReseedNode) Category() string    { return "站点" }
func (n *ReseedNode) Description() string { return "对指定下载器执行 IYUU 辅种" }
func (n *ReseedNode) Icon() string        { return "seeding" }
func (n *ReseedNode) DefaultParams() map[string]any {
	return map[string]any{"downloader_id": float64(0)}
}
func (n *ReseedNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "downloader_id", Label: "下载器", Type: "downloader_select", Required: true},
	}
}

func (n *ReseedNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	dlID := workflow.ParamInt(params, "downloader_id")
	if dlID <= 0 {
		return fmt.Errorf("未选择下载器")
	}
	tasks, err := n.keepSeed.Reseed(ctx, uint(dlID))
	if err != nil {
		return fmt.Errorf("辅种失败: %w", err)
	}
	ecx.Messages = append(ecx.Messages, fmt.Sprintf("辅种完成：候选 %d 条", len(tasks)))
	return nil
}
