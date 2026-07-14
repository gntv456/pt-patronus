package chain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
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
func (c *ContractChain) List() ([]models.ContractRule, error)      { return c.op.List() }
func (c *ContractChain) Get(id uint) (*models.ContractRule, error) { return c.op.Get(id) }
func (c *ContractChain) Upsert(r *models.ContractRule) error       { return c.op.Upsert(r) }
func (c *ContractChain) Delete(id uint) error                      { return c.op.Delete(id) }

// CheckResult 单次契约检查结果。
type CheckResult struct {
	RuleID          uint     `json:"rule_id"`
	SiteID          uint     `json:"site_id"`
	SiteName        string   `json:"site_name"`
	Status          string   `json:"status"` // ok | breach | error
	Detail          string   `json:"detail,omitempty"`
	Ratio           float64  `json:"ratio"`
	Seeding         int      `json:"seeding"`
	UploadGB        float64  `json:"upload_gb"`
	SeedingOfficial int      `json:"seeding_official,omitempty"` // 官种做种数（拉到明细时）
	SeedingSizeGB   float64  `json:"seeding_size_gb"`            // 做种总体积（GiB）
	Unsupported     string   `json:"unsupported,omitempty"`      // 不支持官种识别时的降级提示
	Breaches        []string `json:"breaches,omitempty"`
}

// seedingBase 是契约做种维度（数量/体积）的统计基数。由 Check 按 OfficialOnly 切换：
// 官种契约用官种子集，否则用全量。MinRatio/MinUploadGB 仍读整站 UserInfo（官种维度无意义）。
type seedingBase struct {
	Count int
	Size  int64
}

// evalBreaches 纯函数：判定 UserInfo + 做种基数对契约规则的下限违例。返回违例描述切片（空=达标）。
func evalBreaches(r *models.ContractRule, ui *site.UserInfo, base seedingBase) []string {
	var b []string
	if r.MinRatio > 0 && ui.Ratio < r.MinRatio {
		b = append(b, fmt.Sprintf("分享率 %.2f < %.2f", ui.Ratio, r.MinRatio))
	}
	if r.MinSeeding > 0 && base.Count < r.MinSeeding {
		b = append(b, fmt.Sprintf("做种 %d < %d", base.Count, r.MinSeeding))
	}
	if r.MinUploadGB > 0 && ui.Uploaded < int64(r.MinUploadGB)<<30 {
		b = append(b, fmt.Sprintf("上传 %.1f GiB < %d GiB", float64(ui.Uploaded)/float64(1<<30), r.MinUploadGB))
	}
	if r.MinSeedingSizeGB > 0 && base.Size < int64(r.MinSeedingSizeGB)<<30 {
		b = append(b, fmt.Sprintf("做种体积 %.1f GiB < %d GiB", float64(base.Size)/float64(1<<30), r.MinSeedingSizeGB))
	}
	return b
}

// contractPeriodKey 计算契约当前考核周期的告警去重 key（Period 按周/月重置）。
//   - Period="" → 当日（每日最多告警一次，与原行为一致）
//   - weekly/monthly + 无 StartTime → 自然周(ISO)/自然月对齐
//   - weekly/monthly + StartTime → 从生效日起的周/月序号
//
// 同一周期 key 不变 → 告警去重；周期跨越 key 变化 → 重新允许告警。
func contractPeriodKey(rule *models.ContractRule, now time.Time) string {
	if rule.StartTime != nil && !rule.StartTime.IsZero() {
		base := *rule.StartTime
		switch rule.Period {
		case "weekly":
			return "w" + strconv.Itoa(int(now.Sub(base)/(7*24*time.Hour)))
		case "monthly":
			return "m" + strconv.Itoa(int(now.Year()-base.Year())*12+int(now.Month()-base.Month()))
		}
	}
	switch rule.Period {
	case "weekly":
		y, w := now.ISOWeek()
		return fmt.Sprintf("%d-W%02d", y, w)
	case "monthly":
		return now.Format("2006-01")
	default:
		return now.Format("2006-01-02")
	}
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
	now := time.Now()
	// 开始时间未到 → 不考核
	if rule.StartTime != nil && !rule.StartTime.IsZero() && now.Before(*rule.StartTime) {
		if e := c.op.MarkChecked(ruleID, "ok", "未到生效时间"); e != nil {
			slog.Warn("contract MarkChecked failed", "rule", ruleID, "err", e)
		}
		return &CheckResult{RuleID: ruleID, SiteID: rule.SiteID, Status: "ok", Detail: "未到生效时间"}, nil
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
	// 做种基数：默认整站汇总；官种契约或体积下限时拉逐条明细切基数
	base := seedingBase{Count: ui.Seeding, Size: ui.SeedingSize}
	res := &CheckResult{
		RuleID: ruleID, SiteID: rule.SiteID, SiteName: st.Name,
		Ratio: ui.Ratio, Seeding: ui.Seeding, UploadGB: float64(ui.Uploaded) / float64(1<<30),
		SeedingSizeGB: float64(ui.SeedingSize) / float64(1<<30),
	}
	if rule.OfficialOnly || rule.MinSeedingSizeGB > 0 {
		stats, derr := c.siteCh.SeedingStats(ctx, st)
		switch {
		case derr != nil && !errors.Is(derr, ErrNotSupported) && !errors.Is(derr, site.ErrSeedingListUnsupported):
			// 网络瞬时失败：沿用 ui 汇总基数，不阻断（下次 cron 重试）
			slog.Warn("contract SeedingStats failed", "rule", ruleID, "err", derr)
		case stats != nil && stats.Supported:
			res.SeedingOfficial = stats.Official.Count
			res.SeedingSizeGB = float64(stats.Total.Size) / float64(1<<30)
			if rule.OfficialOnly {
				base.Count = stats.Official.Count
				base.Size = stats.Official.Size
			} else {
				base.Size = stats.Total.Size // 仅体积下限：用明细总体积（更准）
			}
		default:
			// 不支持逐行解析（非 nexusphp / 纯汇总站）→ 按全量统计并提示
			res.Unsupported = "该站不支持官种识别，已按全量做种统计"
		}
	}
	breaches := evalBreaches(rule, ui, base)
	res.Breaches = breaches
	if len(breaches) > 0 {
		res.Status = "breach"
		res.Detail = strings.Join(breaches, "；")
		if res.Unsupported != "" {
			res.Detail += "（" + res.Unsupported + "）"
		}
		if e := c.op.MarkChecked(ruleID, "breach", res.Detail); e != nil {
			slog.Warn("contract MarkChecked failed", "rule", ruleID, "status", "breach", "err", e)
		}
		c.Emit(ctx, "ContractBreach", map[string]any{"site_id": rule.SiteID, "breaches": breaches})
		if c.notice != nil {
			_ = c.notice.Notice(ctx, notice.Input{
				Source: "contract",
				Key:    fmt.Sprintf("contract:%d:%s", rule.SiteID, contractPeriodKey(rule, now)),
				Title:  fmt.Sprintf("保种契约告警 · %s", st.Name),
				Body:   "未达标：" + strings.Join(breaches, "；"),
				Level:  "warning",
			}, true) // 当周期内去重
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
