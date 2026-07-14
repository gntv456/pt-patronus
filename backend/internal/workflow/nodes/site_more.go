package nodes

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/workflow"
)

// 本文件封装 SiteChain 的站点业务方法（站免/公告/站内信/H&R/保活/用户资料/连接测试/后宫/全站签到）
// 为工作流节点。9 个节点共用 siteNode 骨架：单站节点解析 site_id → *models.Site 后调 run。

// siteNode 是站点业务节点的通用骨架。按 typ 区分。
type siteNode struct {
	typ      string
	name     string
	desc     string
	icon     string
	siteCh   *chain.SiteChain
	siteOp   *oper.SiteOper
	schema   []workflow.ParamField
	defaults map[string]any
	// run 执行业务；单站节点 s 为解析后的站点，无需站点的节点（checkin_all）s 为 nil。
	run func(ctx context.Context, siteCh *chain.SiteChain, s *models.Site, p map[string]any, ecx *workflow.Context) error
}

func (n *siteNode) Type() string                       { return n.typ }
func (n *siteNode) Name() string                       { return n.name }
func (n *siteNode) Category() string                   { return "站点" }
func (n *siteNode) Description() string                { return n.desc }
func (n *siteNode) Icon() string                       { return n.icon }
func (n *siteNode) DefaultParams() map[string]any      { return n.defaults }
func (n *siteNode) ParamSchema() []workflow.ParamField { return n.schema }

func (n *siteNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	var s *models.Site
	// 含 site_id 字段的节点解析站点；checkin_all 无该字段时跳过。
	if hasField(n.schema, "site_id") {
		var err error
		s, err = resolveSite(n.siteOp, params)
		if err != nil {
			return err
		}
	}
	return n.run(ctx, n.siteCh, s, params, ecx)
}

// resolveSite 由 site_id 解析站点（同 CheckinNode 模式）。
func resolveSite(siteOp *oper.SiteOper, params map[string]any) (*models.Site, error) {
	sid := workflow.ParamInt(params, "site_id")
	if sid <= 0 {
		return nil, fmt.Errorf("未选择站点")
	}
	s, err := siteOp.Get(uint(sid))
	if err != nil || s == nil {
		return nil, fmt.Errorf("站点不存在")
	}
	return s, nil
}

// hasField schema 是否含某字段。
func hasField(schema []workflow.ParamField, name string) bool {
	for _, f := range schema {
		if f.Name == name {
			return true
		}
	}
	return false
}

// writeJSON 把结果序列化写入 ecx.Content，供下游 parse_json/extract_fields 处理。
func writeJSON(ecx *workflow.Context, v any) {
	if b, err := json.Marshal(v); err == nil {
		ecx.Content = string(b)
	}
}

func NewCheckinAllNode(siteCh *chain.SiteChain, siteOp *oper.SiteOper) *siteNode {
	return &siteNode{
		typ: "checkin_all", name: "全站签到", desc: "对所有启用站点并发签到", icon: "checkin",
		siteCh: siteCh, siteOp: siteOp, schema: nil, defaults: map[string]any{},
		run: func(ctx context.Context, siteCh *chain.SiteChain, _ *models.Site, _ map[string]any, ecx *workflow.Context) error {
			if siteCh == nil {
				return fmt.Errorf("站点服务未初始化")
			}
			results := siteCh.CheckinAll(ctx)
			ok := 0
			for _, r := range results {
				if r.Success {
					ok++
				}
			}
			ecx.Messages = append(ecx.Messages, fmt.Sprintf("全站签到：%d/%d 成功", ok, len(results)))
			return nil
		},
	}
}

func NewFetchFreeleechNode(siteCh *chain.SiteChain, siteOp *oper.SiteOper) *siteNode {
	return &siteNode{
		typ: "fetch_freeleech", name: "获取站免", desc: "抓取站点全站免费/促销横幅（写入文本载荷）", icon: "free",
		siteCh: siteCh, siteOp: siteOp,
		schema:   []workflow.ParamField{{Name: "site_id", Label: "站点", Type: "site_select", Required: true}},
		defaults: map[string]any{"site_id": float64(0)},
		run: func(ctx context.Context, siteCh *chain.SiteChain, s *models.Site, _ map[string]any, ecx *workflow.Context) error {
			list, err := siteCh.Freeleech(ctx, s)
			if err != nil {
				return fmt.Errorf("获取站免失败: %w", err)
			}
			writeJSON(ecx, list)
			ecx.Messages = append(ecx.Messages, fmt.Sprintf("站免：%s → %d 条", s.Name, len(list)))
			return nil
		},
	}
}

func NewFetchAnnouncementNode(siteCh *chain.SiteChain, siteOp *oper.SiteOper) *siteNode {
	return &siteNode{
		typ: "fetch_announcement", name: "获取公告", desc: "抓取站点公告（写入文本载荷）", icon: "announce",
		siteCh: siteCh, siteOp: siteOp,
		schema:   []workflow.ParamField{{Name: "site_id", Label: "站点", Type: "site_select", Required: true}},
		defaults: map[string]any{"site_id": float64(0)},
		run: func(ctx context.Context, siteCh *chain.SiteChain, s *models.Site, _ map[string]any, ecx *workflow.Context) error {
			ann, err := siteCh.Announcements(ctx, s)
			if err != nil {
				return fmt.Errorf("获取公告失败: %w", err)
			}
			writeJSON(ecx, ann)
			cnt := 0
			if ann != nil {
				cnt = len(ann.Items)
			}
			ecx.Messages = append(ecx.Messages, fmt.Sprintf("公告：%s → %d 条", s.Name, cnt))
			return nil
		},
	}
}

func NewFetchMessagesNode(siteCh *chain.SiteChain, siteOp *oper.SiteOper) *siteNode {
	return &siteNode{
		typ: "fetch_messages", name: "获取站内信", desc: "抓取站内信列表（写入文本载荷）", icon: "mail",
		siteCh: siteCh, siteOp: siteOp,
		schema: []workflow.ParamField{
			{Name: "site_id", Label: "站点", Type: "site_select", Required: true},
			{Name: "mailbox", Label: "信箱", Type: "select", Options: []workflow.ParamOption{
				{Label: "收件箱", Value: string(site.MailboxInbox)},
				{Label: "发件箱", Value: string(site.MailboxSent)},
				{Label: "系统", Value: string(site.MailboxSystem)},
			}},
		},
		defaults: map[string]any{"site_id": float64(0), "mailbox": string(site.MailboxInbox)},
		run: func(ctx context.Context, siteCh *chain.SiteChain, s *models.Site, p map[string]any, ecx *workflow.Context) error {
			mb := site.MailboxType(workflow.ParamString(p, "mailbox"))
			if mb == "" {
				mb = site.MailboxInbox
			}
			list, err := siteCh.Messages(ctx, s, mb)
			if err != nil {
				return fmt.Errorf("获取站内信失败: %w", err)
			}
			writeJSON(ecx, list)
			ecx.Messages = append(ecx.Messages, fmt.Sprintf("站内信：%s → %d 条", s.Name, len(list)))
			return nil
		},
	}
}

func NewCheckHRNode(siteCh *chain.SiteChain, siteOp *oper.SiteOper) *siteNode {
	return &siteNode{
		typ: "check_hr", name: "检查 H&R", desc: "抓取站点 H/R 考核任务（写入文本载荷）", icon: "hr",
		siteCh: siteCh, siteOp: siteOp,
		schema:   []workflow.ParamField{{Name: "site_id", Label: "站点", Type: "site_select", Required: true}},
		defaults: map[string]any{"site_id": float64(0)},
		run: func(ctx context.Context, siteCh *chain.SiteChain, s *models.Site, _ map[string]any, ecx *workflow.Context) error {
			list, err := siteCh.HR(ctx, s)
			if err != nil {
				return fmt.Errorf("检查 H&R 失败: %w", err)
			}
			writeJSON(ecx, list)
			if ecx.Variables == nil {
				ecx.Variables = map[string]any{}
			}
			ecx.Variables["hr_count"] = len(list)
			ecx.Messages = append(ecx.Messages, fmt.Sprintf("H&R：%s → %d 条", s.Name, len(list)))
			return nil
		},
	}
}

func NewKeepAliveNode(siteCh *chain.SiteChain, siteOp *oper.SiteOper) *siteNode {
	return &siteNode{
		typ: "keep_alive", name: "站点保活", desc: "刷新站点 cookie 会话保活", icon: "keepalive",
		siteCh: siteCh, siteOp: siteOp,
		schema:   []workflow.ParamField{{Name: "site_id", Label: "站点", Type: "site_select", Required: true}},
		defaults: map[string]any{"site_id": float64(0)},
		run: func(ctx context.Context, siteCh *chain.SiteChain, s *models.Site, _ map[string]any, ecx *workflow.Context) error {
			res, err := siteCh.KeepAlive(ctx, s)
			if err != nil {
				return fmt.Errorf("保活失败: %w", err)
			}
			msg := "已保活"
			if res != nil && !res.LoggedIn {
				msg = "未登录"
			}
			ecx.Messages = append(ecx.Messages, fmt.Sprintf("保活：%s → %s", s.Name, msg))
			return nil
		},
	}
}

func NewSiteUserinfoNode(siteCh *chain.SiteChain, siteOp *oper.SiteOper) *siteNode {
	return &siteNode{
		typ: "site_userinfo", name: "用户资料", desc: "抓取站点用户资料（上传/下载/分享率/魔力写入变量）", icon: "user",
		siteCh: siteCh, siteOp: siteOp,
		schema:   []workflow.ParamField{{Name: "site_id", Label: "站点", Type: "site_select", Required: true}},
		defaults: map[string]any{"site_id": float64(0)},
		run: func(ctx context.Context, siteCh *chain.SiteChain, s *models.Site, _ map[string]any, ecx *workflow.Context) error {
			ui, err := siteCh.UserInfo(ctx, s)
			if err != nil {
				return fmt.Errorf("获取用户资料失败: %w", err)
			}
			writeJSON(ecx, ui)
			if ecx.Variables == nil {
				ecx.Variables = map[string]any{}
			}
			ecx.Variables["uploaded"] = ui.Uploaded
			ecx.Variables["downloaded"] = ui.Downloaded
			ecx.Variables["ratio"] = ui.Ratio
			ecx.Variables["bonus"] = ui.Bonus
			ecx.Variables["seeding"] = ui.Seeding
			ecx.Messages = append(ecx.Messages, fmt.Sprintf("用户资料：%s ↑%v ↓%v 分享率 %.2f", s.Name, ui.Uploaded, ui.Downloaded, ui.Ratio))
			return nil
		},
	}
}

func NewSiteTestNode(siteCh *chain.SiteChain, siteOp *oper.SiteOper) *siteNode {
	return &siteNode{
		typ: "site_test", name: "测试连接", desc: "测试站点连接与登录态，用户等级写入变量", icon: "test",
		siteCh: siteCh, siteOp: siteOp,
		schema:   []workflow.ParamField{{Name: "site_id", Label: "站点", Type: "site_select", Required: true}},
		defaults: map[string]any{"site_id": float64(0)},
		run: func(ctx context.Context, siteCh *chain.SiteChain, s *models.Site, _ map[string]any, ecx *workflow.Context) error {
			level, err := siteCh.TestConnection(ctx, s)
			if err != nil {
				return fmt.Errorf("连接失败: %w", err)
			}
			if ecx.Variables == nil {
				ecx.Variables = map[string]any{}
			}
			ecx.Variables["site_level"] = level
			ecx.Messages = append(ecx.Messages, fmt.Sprintf("连接：%s → %s", s.Name, level))
			return nil
		},
	}
}

func NewHaremStatusNode(siteCh *chain.SiteChain, siteOp *oper.SiteOper) *siteNode {
	return &siteNode{
		typ: "harem_status", name: "后宫状态", desc: "抓取站点邀请库存（剩余/可购写入变量）", icon: "harem",
		siteCh: siteCh, siteOp: siteOp,
		schema:   []workflow.ParamField{{Name: "site_id", Label: "站点", Type: "site_select", Required: true}},
		defaults: map[string]any{"site_id": float64(0)},
		run: func(ctx context.Context, siteCh *chain.SiteChain, s *models.Site, _ map[string]any, ecx *workflow.Context) error {
			st, err := siteCh.HaremStatus(ctx, s)
			if err != nil {
				return fmt.Errorf("获取后宫状态失败: %w", err)
			}
			writeJSON(ecx, st)
			if ecx.Variables == nil {
				ecx.Variables = map[string]any{}
			}
			ecx.Variables["invite_remain"] = st.TotalRemain()
			ecx.Variables["invite_buyable"] = st.TotalBuyable()
			ecx.Messages = append(ecx.Messages, fmt.Sprintf("后宫：%s → 剩余 %d / 可购 %d", s.Name, st.TotalRemain(), st.TotalBuyable()))
			return nil
		},
	}
}
