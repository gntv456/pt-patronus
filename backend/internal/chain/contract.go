package chain

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/site"
)

// ContractChain 编排保种契约：为站点设分享率/做种数/上传量下限，定时检查 SiteChain.UserInfo，
// 低于任一下限则告警通知（防分享率过低被封；与 H&R 告警同属保号自动化）。
type ContractChain struct {
	Base
	siteCh *SiteChain
	siteOp *oper.SiteOper
	op     *oper.ContractOper
	notice *notice.Service
}

func NewContractChain(b Base, siteCh *SiteChain, siteOp *oper.SiteOper, op *oper.ContractOper, ns *notice.Service) *ContractChain {
	return &ContractChain{Base: b, siteCh: siteCh, siteOp: siteOp, op: op, notice: ns}
}

// 透传 CRUD
func (c *ContractChain) List() ([]models.ContractRule, error)            { return c.op.List() }
func (c *ContractChain) Get(id uint) (*models.ContractRule, error)       { return c.op.Get(id) }
func (c *ContractChain) Upsert(r *models.ContractRule) error             { return c.op.Upsert(r) }
func (c *ContractChain) Delete(id uint) error                            { return c.op.Delete(id) }

// CheckResult 单次契约检查结果。
type CheckResult struct {
	RuleID   uint     `json:"rule_id"`
	SiteID   uint     `json:"site_id"`
	SiteName string   `json:"site_name"`
	Status   string   `json:"status"` // ok | breach | error
	Detail   string   `json:"detail,omitempty"`
	Ratio    float64  `json:"ratio"`
	Seeding  int      `json:"seeding"`
	UploadGB float64  `json:"upload_gb"`
	Breaches []string `json:"breaches,omitempty"`
}

// evalBreaches 纯函数：判定 UserInfo 对契约规则的下限违例。返回违例描述切片（空=达标）。
func evalBreaches(r *models.ContractRule, ui *site.UserInfo) []string {
	var b []string
	if r.MinRatio > 0 && ui.Ratio < r.MinRatio {
		b = append(b, fmt.Sprintf("分享率 %.2f < %.2f", ui.Ratio, r.MinRatio))
	}
	if r.MinSeeding > 0 && ui.Seeding < r.MinSeeding {
		b = append(b, fmt.Sprintf("做种 %d < %d", ui.Seeding, r.MinSeeding))
	}
	if r.MinUploadGB > 0 && ui.Uploaded < int64(r.MinUploadGB)<<30 {
		b = append(b, fmt.Sprintf("上传 %.1f GiB < %d GiB", float64(ui.Uploaded)/float64(1<<30), r.MinUploadGB))
	}
	return b
}

// Check 现场检查单条契约规则。
func (c *ContractChain) Check(ctx context.Context, ruleID uint) (*CheckResult, error) {
	rule, err := c.op.Get(ruleID)
	if err != nil {
		return nil, err
	}
	if !rule.Enabled {
		return &CheckResult{RuleID: ruleID, SiteID: rule.SiteID, Status: "ok", Detail: "规则未启用"}, nil
	}
	st, err := c.siteOp.Get(rule.SiteID)
	if err != nil {
		slog.Warn("contract site lookup failed", "rule", ruleID, "site", rule.SiteID, "err", err)
		if e := c.op.MarkChecked(ruleID, "error", "站点不存在"); e != nil {
			slog.Warn("contract MarkChecked failed", "rule", ruleID, "err", e)
		}
		return &CheckResult{RuleID: ruleID, SiteID: rule.SiteID, Status: "error", Detail: "站点不存在"}, nil
	}
	ui, err := c.siteCh.UserInfo(ctx, st)
	if err != nil {
		slog.Warn("contract UserInfo failed", "rule", ruleID, "site", st.Name, "err", err)
		if e := c.op.MarkChecked(ruleID, "error", err.Error()); e != nil {
			slog.Warn("contract MarkChecked failed", "rule", ruleID, "err", e)
		}
		return &CheckResult{RuleID: ruleID, SiteID: rule.SiteID, SiteName: st.Name, Status: "error", Detail: err.Error()}, nil
	}
	breaches := evalBreaches(rule, ui)
	res := &CheckResult{
		RuleID: ruleID, SiteID: rule.SiteID, SiteName: st.Name,
		Ratio: ui.Ratio, Seeding: ui.Seeding, UploadGB: float64(ui.Uploaded) / float64(1<<30),
		Breaches: breaches,
	}
	if len(breaches) > 0 {
		res.Status = "breach"
		res.Detail = strings.Join(breaches, "；")
		if e := c.op.MarkChecked(ruleID, "breach", res.Detail); e != nil {
			slog.Warn("contract MarkChecked failed", "rule", ruleID, "status", "breach", "err", e)
		}
		c.Emit(ctx, "ContractBreach", map[string]any{"site_id": rule.SiteID, "breaches": breaches})
		if c.notice != nil {
			_ = c.notice.Notice(ctx, notice.Input{
				Source: "contract",
				Key:    fmt.Sprintf("contract:%d:%s", rule.SiteID, time.Now().Format("2006-01-02")),
				Title:  fmt.Sprintf("保种契约告警 · %s", st.Name),
				Body:   "未达标：" + res.Detail,
				Level:  "warning",
			}, true) // 当天去重
		}
	} else {
		res.Status = "ok"
		if e := c.op.MarkChecked(ruleID, "ok", ""); e != nil {
			slog.Warn("contract MarkChecked failed", "rule", ruleID, "status", "ok", "err", e)
		}
	}
	return res, nil
}

// CheckAll 定时入口：遍历所有启用规则检查（单条失败不阻断其余）。
func (c *ContractChain) CheckAll(ctx context.Context) {
	rules, err := c.op.ListEnabled()
	if err != nil || len(rules) == 0 {
		return
	}
	for _, r := range rules {
		if _, err := c.Check(ctx, r.ID); err != nil {
			// 单条失败不阻断
		}
	}
}
