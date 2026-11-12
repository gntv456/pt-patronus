// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package groupchat

import (
	"fmt"
	"strings"
	"time"
)

// 奖励类型 → emoji(NotificationIcons 映射)。
var rewardIcons = map[string]string{
	"上传量":  "⬆️",
	"下载量":  "⬇️",
	"魔力值":  "✨",
	"工分":   "🔧",
	"电力":   "⚡",
	"象草":   "🐘",
	"青蛙":   "🐸",
	"VIP":  "👑",
	"彩虹ID": "🌈",
	"火花":   "🔥",
	"啤酒瓶":  "🍺",
}

// IconFor 返回奖励类型对应的 emoji，未知用 📝。
func IconFor(t string) string {
	if ic, ok := rewardIcons[t]; ok {
		return ic
	}
	return "📝"
}

// SiteResult 单站点的喊话结果汇总。
type SiteResult struct {
	Site        SiteRef
	Feedback    []Feedback
	Failed      []FailedMessage
	Skipped     int    // 因权限/重复跳过的消息数
	BonusText   string // 青蛙每日福利结果
	LotteryText string // LongPT 抽奖结果
}

// FailedMessage 一条失败记录(供重试队列持久化)。
type FailedMessage struct {
	SiteID   uint
	SiteName string
	Message  string
	Interval int
	Error    string
}

// ReportInput 组合通知的输入。
type ReportInput struct {
	Results     []SiteResult
	ZhImengNext *time.Time // 织梦下次预计时间(展示倒计时用)，nil 不显示
}

// ComposeReport 组合"全报告"模式通知正文(默认「全报告」分支)。
func ComposeReport(in ReportInput) string {
	var b strings.Builder
	var siteNames []string
	totalFail := 0
	totalReward := 0
	for _, r := range in.Results {
		siteNames = append(siteNames, r.Site.Name)
		for _, f := range r.Feedback {
			totalReward += len(f.Rewards)
		}
		totalFail += len(r.Failed)
	}
	if len(siteNames) == 0 {
		return "本次无站点执行(检查 chat_sites / sites_messages 配置)。"
	}
	fmt.Fprintf(&b, "💬 群聊区任务完成报告\n站点：%s\n", strings.Join(siteNames, "、"))
	if totalReward > 0 {
		fmt.Fprintf(&b, "奖励条目：%d\n", totalReward)
	}
	if totalFail > 0 {
		fmt.Fprintf(&b, "失败：%d 条(将自动重试)\n", totalFail)
	}
	b.WriteString("\n")
	for _, r := range in.Results {
		fmt.Fprintf(&b, "【%s】\n", r.Site.Name)
		anyReward := false
		for _, fb := range r.Feedback {
			for _, rw := range fb.Rewards {
				anyReward = true
				amt := rw.Amount
				if amt != "" {
					fmt.Fprintf(&b, "  %s %s %s\n", IconFor(rw.Type), rw.Type, amt)
				} else {
					fmt.Fprintf(&b, "  %s %s\n", IconFor(rw.Type), rw.Type)
				}
			}
			if !anyReward && strings.TrimSpace(fb.Raw) != "" {
				// 无奖励但有原始反馈，展示一行
				line := strings.TrimSpace(fb.Raw)
				if i := strings.IndexByte(line, '\n'); i > 0 {
					line = line[:i]
				}
				if len([]rune(line)) > 60 {
					line = string([]rune(line)[:60]) + "…"
				}
				fmt.Fprintf(&b, "  📝 %s\n", line)
			}
		}
		if !anyReward && len(r.Feedback) == 0 {
			b.WriteString("  (无反馈)\n")
		}
		if r.BonusText != "" {
			fmt.Fprintf(&b, "  🐸 每日福利：%s\n", r.BonusText)
		}
		if r.LotteryText != "" {
			fmt.Fprintf(&b, "  🎰 每日抽奖：%s\n", r.LotteryText)
		}
	}
	if in.ZhImengNext != nil {
		fmt.Fprintf(&b, "\n⏰ 织梦下次预计：%s\n", in.ZhImengNext.Format("2006-01-02 15:04"))
	}
	return strings.TrimRight(b.String(), "\n")
}

// ComposeRetryNotice 重试排程通知(重试排程通知)。
func ComposeRetryNotice(failed []FailedMessage, next time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "🔄 喊话失败将重试\n下次重试：%s\n失败 %d 条：\n", next.Format("15:04:05"), len(failed))
	bySite := map[string][]string{}
	for _, f := range failed {
		bySite[f.SiteName] = append(bySite[f.SiteName], f.Message)
	}
	for site, msgs := range bySite {
		fmt.Fprintf(&b, "  【%s】%s\n", site, strings.Join(msgs, " / "))
	}
	return strings.TrimRight(b.String(), "\n")
}
