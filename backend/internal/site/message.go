// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

import (
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// MailboxBoxParam 各邮箱的 box= 参数（STAFF 走独立页 staffbox.php，由调用方处理）。
func MailboxBoxParam(m MailboxType) string {
	switch m {
	case MailboxSent:
		return "-1"
	case MailboxSystem:
		return "-2"
	case MailboxCheater:
		return "-3"
	case MailboxReports:
		return "-5"
	}
	return "" // inbox
}

var (
	msgDateRe   = regexp.MustCompile(`(?i)(\d{4}[-/]\d{1,2}[-/]\d{1,2}|\d{1,2}月\d{1,2}[日号]|\d+\s*(小时|天|周)前|\d+\s*天\d+\s*时前|\d+\s*月\d+\s*天前|yesterday|today|\d+\s*(hours?|days?|weeks?)\s+ago)`)
	msgIDRe     = regexp.MustCompile(`[?&]id=(\d+)`)
	pureDigitRe = regexp.MustCompile(`^\d+$`) // 纯数字 uid 链接，sender 提取时跳过取真实用户名
)

// ParseMessages 从私信列表页解析（NexusPHP table tr:has(td.rowfollow)）。
// siteID/siteName/mailbox/fetchedAt 由调用方在每条上盖章。
func ParseMessages(doc *goquery.Document, mailbox MailboxType) []Message {
	if doc == nil {
		return nil
	}
	// staffbox.php 是独立页面，结构不同于 messages.php：链接为 viewpm（非 viewmessage）、
	// itemId 来自 setanswered[]。通用 a[href*="viewmessage"] 选择器匹配不到，需专门解析。
	if mailbox == MailboxStaff {
		return parseStaffboxMessages(doc, mailbox, time.Now())
	}
	now := time.Now()
	var out []Message
	// 用直接子 td.rowfollow 计数：部分 NexusPHP 站（如好学）HTML 标签不闭合/嵌套，
	// goquery 会把整个消息表（搜索栏+表头+所有行+分页+操作栏）解析进一个外层 <tr>，
	// 其「后代」td.rowfollow 多达数十个、viewmessage 命中首条 → 被误判为一条假消息
	// （sender 取到搜索栏的「收件箱发件箱」）。改数直接子，外层 tr 直接子 td.rowfollow 远 <4 即排除。
	rows := doc.Find(`table tr:has(td.rowfollow)`).FilterFunction(func(_ int, tr *goquery.Selection) bool {
		return tr.Children().Filter("td.rowfollow").Length() >= 4
	})
	if rows.Length() == 0 {
		rows = doc.Find(`table tr`).FilterFunction(func(_ int, tr *goquery.Selection) bool {
			return tr.Find("td").Length() >= 3 && tr.Find(`a[href*="viewmessage"]`).Length() > 0
		})
	}
	if rows.Length() == 0 {
		// 现代 div 布局（憨憨 HHan 皮肤）：无 table，行为
		// div.grid > [checkbox 行为][已读图标][<a href="messages.php?action=viewmessage&id=N">主题</a>][发讯者][<span title="时间">时间</span>]。
		// 复用下方 tr 解析需 td/cells 结构，这里独立解析（cells 无 td 可取）。
		return parseMessagesDivRows(doc, mailbox, now)
	}
	rows.Each(func(idx int, tr *goquery.Selection) {
		cells := tr.Find("td")
		if cells.Length() < 3 {
			return
		}
		first := cells.First().Text()
		// 表头跳过
		fl := strings.TrimSpace(first)
		if regexp.MustCompile(`(?i)^(Status|Subject|Sender|Date|Action|状态|主题|发件人|日期)`).MatchString(fl) ||
			tr.HasClass("colhead") {
			return
		}
		// title 链接
		titleLink := tr.Find(`a[href*="viewmessage"]`).First()
		if titleLink.Length() == 0 {
			return
		}
		title := strings.TrimSpace(titleLink.Text())
		if title == "" || title == "No Subject" {
			return
		}
		href, _ := titleLink.Attr("href")
		// sender：优先扫 tr 内 userdetails 链接取用户名——适配 PTTime 等皮肤（td 序含图标/回复数
		// 格导致 cells.Eq(2) 错位成主题）。跳过纯数字 uid 链接（PTTime 同时给 uid 与用户名两个链接）。
		sender := ""
		tr.Find(`a[href*="userdetails.php"]`).Each(func(_ int, a *goquery.Selection) {
			if t := strings.TrimSpace(a.Text()); t != "" && !pureDigitRe.MatchString(t) {
				sender = t
			}
		})
		if sender == "" && cells.Length() > 2 {
			sender = strings.TrimSpace(cells.Eq(2).Text())
		}
		if sender == "" {
			sender = strings.TrimSpace(cells.Eq(0).Text())
		}
		if sender == "" {
			sender = "未知"
		}
		// date（从第 4 格起扫）
		date := detectMsgDate(cells, 3)
		// unread
		unread := tr.Find("img.unreadpm, img.newpm").Length() > 0 || tr.HasClass("unread")
		// itemId / deleteUrl
		itemID := ""
		if iv, ok := tr.Find(`input[name="id[]"], input[type="checkbox"]`).First().Attr("value"); ok {
			itemID = iv
		}
		deleteURL := ""
		if du, ok := tr.Find(`a[href*="action=delete"]`).First().Attr("href"); ok {
			deleteURL = du
		}
		out = append(out, Message{
			ID:           0, // 由调用方按 siteID*100000+mailboxIdx*10000+idx 合成
			Sender:       sender,
			Title:        title,
			Date:         date,
			URL:          href,
			IsRead:       !unread,
			Mailbox:      mailbox,
			FetchedAt:    now,
			AdminActions: &AdminActions{DeleteURL: deleteURL, ItemID: itemID},
		})
	})
	return out
}

// parseMessagesDivRows 解析现代 div 布局的消息行（憨憨 HHan 皮肤）。
// 行容器是 #mail-table-display 的直接子 div（表头/分页不带 viewmessage 链接，天然过滤）。
// 结构：<div class="grid ..."><div>checkbox name="messages[]" value=ID</div><div>已读图标</div>
//
//	<div><a href="messages.php?action=viewmessage&id=ID">主题</a></div><div>发讯者</div>
//	<div><span title="YYYY-MM-DD HH:MM:SS">相对时间</span></div></div>
//
// sender 无 userdetails 链接时取主题链接所在格的下一格文本（系统消息为「系统」）。
func parseMessagesDivRows(doc *goquery.Document, mailbox MailboxType, now time.Time) []Message {
	var out []Message
	doc.Find(`div a[href*="viewmessage"]`).Each(func(idx int, a *goquery.Selection) {
		title := strings.TrimSpace(a.Text())
		if title == "" || title == "No Subject" {
			return
		}
		row := a.Closest("div.grid")
		if row.Length() == 0 {
			row = a.Parent()
		}
		href, _ := a.Attr("href")
		sender := ""
		row.Find(`a[href*="userdetails.php"]`).Each(func(_ int, u *goquery.Selection) {
			if t := strings.TrimSpace(u.Text()); t != "" && !pureDigitRe.MatchString(t) {
				sender = t
			}
		})
		if sender == "" {
			// 主题链接父格的下一个兄弟格为发讯者列
			if cell := a.Parent(); cell.Length() > 0 {
				sender = strings.TrimSpace(cell.Next().Text())
			}
		}
		if sender == "" {
			sender = "未知"
		}
		date := ""
		if sp := row.Find(`span[title]`).First(); sp.Length() > 0 {
			date = strings.TrimSpace(sp.AttrOr("title", ""))
		}
		itemID := ""
		if iv, ok := row.Find(`input[name="messages[]"], input[name="id[]"], input[type="checkbox"]`).First().Attr("value"); ok {
			itemID = iv
		}
		unread := row.Find("img.unreadpm, img.newpm").Length() > 0
		out = append(out, Message{
			ID:           0,
			Sender:       sender,
			Title:        title,
			Date:         date,
			URL:          href,
			IsRead:       !unread,
			Mailbox:      mailbox,
			FetchedAt:    now,
			AdminActions: &AdminActions{ItemID: itemID},
		})
	})
	return out
}

// parseStaffboxMessages 解析 staffbox.php（管理组信箱）。页面结构不同于 messages.php：
//
//	<tr><td class=rowfollow><a href="staffbox.php?action=viewpm&pmid=ID">主题</a></td>
//	    <td class=rowfollow>发件人</td><td class=rowfollow>日期</td>
//	    <td class=rowfollow>已回复</td><td class=rowfollow><input name="setanswered[]" value="ID"></td></tr>
//
// 链接为 viewpm 而非 viewmessage、itemId 来自 setanswered[]，故 ParseMessages 的通用选择器
// 匹配不到——移植自旧版 PTPatronus（Kotlin）parseStaffboxMessages。
func parseStaffboxMessages(doc *goquery.Document, mailbox MailboxType, now time.Time) []Message {
	rows := doc.Find(`table tr`).FilterFunction(func(_ int, tr *goquery.Selection) bool {
		return tr.Find("td.rowfollow").Length() >= 4 && tr.Find(`a[href*="viewpm"]`).Length() > 0
	})
	var out []Message
	rows.Each(func(_ int, tr *goquery.Selection) {
		cells := tr.Find("td.rowfollow")
		if cells.Length() < 4 {
			return
		}
		titleLink := tr.Find(`a[href*="viewpm"]`).First()
		title := strings.TrimSpace(titleLink.Text())
		if title == "" || title == "No Subject" {
			return
		}
		// 表头兜底（通常已被 FilterFunction 的 viewpm 条件排除）
		if regexp.MustCompile(`(?i)^(Status|Subject|Sender|Date|主题|发件人|日期|回复)`).MatchString(title) {
			return
		}
		href, _ := titleLink.Attr("href")
		// 发讯者：优先取用户名链接文本（<span class="nowrap"><a><b>用户</b></a></span>），避开整格空白
		sender := "系统"
		if sc := cells.Eq(1); sc.Length() > 0 {
			if a := sc.Find("a").First(); a.Length() > 0 {
				if t := strings.TrimSpace(a.Text()); t != "" {
					sender = t
				}
			} else if t := strings.TrimSpace(sc.Text()); t != "" {
				sender = t
			}
		}
		// 时间：优先取 <span title="精确时间">（好学：<span title="2026-06-24 21:05:24">3天19时前</span>），回退相对文本
		date := "未知"
		if dc := cells.Eq(2); dc.Length() > 0 {
			if t, ok := dc.Find("span[title]").First().Attr("title"); ok && t != "" {
				date = t
			} else if t := strings.TrimSpace(dc.Text()); t != "" {
				date = t
			}
		}
		// 第 4 格为「已回复」标记，有内容视为已处理（已读）
		answered := strings.TrimSpace(cells.Eq(3).Text()) != ""
		itemID := ""
		if iv, ok := tr.Find(`input[name="setanswered[]"], input[type="checkbox"]`).First().Attr("value"); ok {
			itemID = iv
		}
		deleteURL := ""
		if itemID != "" {
			deleteURL = "staffbox.php?action=delete&id=" + itemID
		}
		out = append(out, Message{
			ID:           0, // 由调用方 SynthesizeMsgID 合成
			Sender:       sender,
			Title:        title,
			Date:         date,
			URL:          href,
			IsRead:       answered,
			Mailbox:      mailbox,
			FetchedAt:    now,
			AdminActions: &AdminActions{DeleteURL: deleteURL, ItemID: itemID},
		})
	})
	return out
}

// detectMsgDate 从 cells[startIndex:] 扫描日期格。
func detectMsgDate(cells *goquery.Selection, start int) string {
	date := "未知"
	cells.Each(func(i int, td *goquery.Selection) {
		if i < start || date != "未知" {
			return
		}
		t := strings.TrimSpace(td.Text())
		if t == "" || len(t) <= 2 {
			return
		}
		if msgDateRe.MatchString(t) {
			date = t
		}
	})
	return date
}

// ExtractMsgID 从 viewmessage 链接抽 id 参数。
func ExtractMsgID(url string) string {
	if m := msgIDRe.FindStringSubmatch(url); len(m) > 1 {
		return m[1]
	}
	return ""
}

// ParseMessageContent 从消息详情页提取正文（4 级策略）。
func ParseMessageContent(doc *goquery.Document) string {
	if doc == nil {
		return ""
	}
	// 策略 1：硬编码 NexusPHP 正文选择器
	for _, sel := range []string{
		`table[width="737"] tr:nth-child(3) td`,
		`table[width="737"] td[colspan="2"]`,
		"table.message tr:last-child td",
		`#content table td[colspan="2"]`,
		`table[width="90%"] td[colspan="2"]`,
		`table.embedded td[colspan="2"]`,
	} {
		if s := doc.Find(sel).First(); s.Length() > 0 {
			if t := strings.TrimSpace(s.Text()); len(t) > 10 {
				return truncate(t, 5000)
			}
		}
	}
	// 策略 2：含「主题/subject」表格的最大 td
	best := ""
	doc.Find("table").Each(func(_ int, tbl *goquery.Selection) {
		if !regexp.MustCompile(`(?i)主题|标题|subject|re:`).MatchString(tbl.Text()) {
			return
		}
		tbl.Find(`td[colspan="2"], td`).Each(func(_ int, td *goquery.Selection) {
			t := strings.TrimSpace(td.Text())
			if len(t) > len(best) {
				best = t
			}
		})
	})
	if len(best) > 10 {
		return truncate(best, 5000)
	}
	// 策略 3：body.text 兜底
	return truncate(strings.TrimSpace(doc.Text()), 5000)
}

func truncate(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

// SynthesizeMsgID 按原版公式合成消息 ID（无真实 itemId 时）。
// siteID*100000 + mailboxOrd*10000 + index。mailboxOrd：inbox=0...staff=5。
func SynthesizeMsgID(siteID uint, mailbox MailboxType, index int) uint64 {
	ord := map[MailboxType]int{
		MailboxInbox: 0, MailboxSent: 1, MailboxSystem: 2,
		MailboxCheater: 3, MailboxReports: 4, MailboxStaff: 5,
	}[mailbox]
	return uint64(siteID)*100000 + uint64(ord)*10000 + uint64(index)
}
