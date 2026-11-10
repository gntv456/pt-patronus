// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

import (
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// ── 邀请库存解析（invite.php 页）──

var (
	// 邀请链接文本：永久(主) + 可选临时(括号)。兼容「邀请[X]: 5(2)」「Invites: 5 (2)」等。
	inviteCountRe = regexp.MustCompile(`(?:邀请|邀請|探视权|查看权|invite)[^\d]*?(\d+)(?:\s*[（(]\s*(\d+)\s*[)）])?`)
	// 显式无邀请权限（覆盖 canInvite=true）。
	inviteNoPermRe = regexp.MustCompile(`(?i)(?:没有[^\n]{0,12}邀请|暂无[^\n]{0,8}邀请[^\n]{0,8}权限|您目前没有[^\n]{0,8}邀请|不具备[^\n]{0,8}邀请[^\n]{0,6}资格|需要[^\n]{0,10}等级[^\n]{0,10}邀请|邀请功能[^\n]{0,10}开启|没有剩余[^\n]{0,8}邀请)`)
	// 魔力/积分（前缀或后缀形式）。
	bonusRe = regexp.MustCompile(`(?:魔力|bonus|积分|火花|蟹币|元宝)[^\d]*([\d,]+\.?\d*)`)
)

// ParseInviteStatus 从 invite.php 页解析邀请库存（4 策略，对齐原版 parseNexusPhpInvitePage）。
// NexusPHP 不设 shop 价格（buyable 恒 0）。
func ParseInviteStatus(doc *goquery.Document) *InviteStatus {
	if doc == nil {
		return &InviteStatus{}
	}
	st := &InviteStatus{PermanentInvitePrice: -1, TemporaryInvitePrice: -1}
	fullText := strings.TrimSpace(doc.Text())

	// 策略 1：侧栏/用户弹窗内邀请数。NexusPHP 标准 #info_block；新版皮肤（好学 cute-top-profile
	// 等）邀请数在 header 用户弹窗（实测「邀请 [发送]: 1120(3)」= 永久1120/临时3）。
	blk := doc.Find("#info_block")
	if blk.Length() == 0 {
		blk = doc.Find("[class*='cute-top-profile']").First()
	}
	if blk.Length() == 0 {
		blk = doc.Find("[class*='menu-base-info']").First() // 财神等站用户弹窗
	}
	if blk.Length() == 0 {
		blk = doc.Find("[class*='user-info'], [class*='top-profile'], [class*='profile'], header").First()
	}
	if blk.Length() > 0 {
		matched := false
		// 先精确：invite 链接父节点（#info_block 风格）
		if link := blk.Find(`a[href*="invite.php"]`); link.Length() > 0 {
			if m := inviteCountRe.FindStringSubmatch(link.Parent().Text()); len(m) > 1 {
				st.PermanentCount = atoiSafe(m[1])
				if len(m) > 2 {
					st.TemporaryCount = atoiSafe(m[2])
				}
				matched = true
			}
		}
		// 回退：容器全文（cute-top-profile 邀请数与链接同区，父节点可能不含数字）
		if !matched {
			if m := inviteCountRe.FindStringSubmatch(blk.Text()); len(m) > 1 {
				st.PermanentCount = atoiSafe(m[1])
				if len(m) > 2 {
					st.TemporaryCount = atoiSafe(m[2])
				}
			}
		}
	}

	// 策略 2：发药表单 → canInvite
	hasForm := doc.Find(`form[action*="takeinvite.php"]`).Length() > 0 ||
		doc.Find(`form[name*="invite"]`).Length() > 0
	if hasForm {
		st.CanInvite = true
		if st.Reason == "" {
			st.Reason = "检测到发药表单"
		}
	}

	// 注：原「策略4 全页 fullText 回退」已去掉——它用 inviteCountRe 在整页找「邀请后第一个数字」，
	// 会误抓花名册里的 userdetails id / 时间数字（好学 invite.php 无 #info_block 时抓到 15），
	// 致 perm 虚高、canInvite 误判。剩余邀请只信策略1（#info_block 侧栏）；无侧栏的站 perm=0（诚实）。

	// 有剩余 → canInvite
	if st.PermanentCount > 0 || st.TemporaryCount > 0 {
		st.CanInvite = true
		if st.Reason == "" {
			st.Reason = formatInviteReason(st.PermanentCount, st.TemporaryCount)
		}
	}

	// 策略 3：显式无权限（最高优先级，覆盖）
	if loc := inviteNoPermRe.FindString(fullText); loc != "" {
		st.CanInvite = false
		st.Reason = strings.TrimSpace(loc)
	}

	// 魔力/积分
	if m := bonusRe.FindStringSubmatch(fullText); len(m) > 1 {
		st.Bonus = parseBonusFloat(m[1])
	}
	return st
}

// ── 被邀请人解析（invite.php?menu=invitee 单页）──

var (
	inviteeHeaderRe = regexp.MustCompile(`(?i)(?:Username|用户名|Email|邮箱|加入日期|状态|Uploaded|上传|Downloaded|下载|Ratio|分享率|邀请人|操作|User|名字|时间|注册)`)
	emailRe         = regexp.MustCompile(`(?i)[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}`)
	ratioTextRe     = regexp.MustCompile(`^(\d{1,4}(?:\.\d{1,4})?)$`)
	// userdetails 链接里的 id（用于排除"自己"）：userdetails.php?id=123。
	inviteeIDRe = regexp.MustCompile(`(?i)[?&]id=(\d+)`)
	// 禁用：属性或文本含 banned/disabled/禁用 等（扫描行 HTML，非纯文本）。
	inviteeDisabledRe = regexp.MustCompile(`(?i)(?:\browbanned\b|\buserbanned\b|\bbanned\b|\bdisabled\b|\bsuspended\b|\bforbidden\b|disabled\.png|disable\.png|banned\.png|class\s*=\s*[^"']*(?:banned|disabled)|alt\s*=\s*[^"']*(?:disabled|banned)|title\s*=\s*[^"']*(?:disabled|banned)|(?:已\s*)?(?:禁用|封禁|停用)|被\s*(?:禁用|封禁|停用|禁止)|未启用)`)
	inviteePendingRe  = regexp.MustCompile(`(?i)(?:\bpending\b|\bunconfirmed\b|\bnot\s+confirmed\b|待确认|待確認|未确认|未確認|未激活)`)
)

// ParseInvitees 从 invite.php?menu=invitee 单页解析被邀请人列表。
// 行匹配用「直接子 td」（ChildrenFiltered）而非后代 td（Find）：空态/提示行的 wrapper
// <tr><td><table>…</table></td></tr> 会让 Find 数到嵌套表格的 td 而被误判为数据行
// （用户曾报空邀请页解析出整页文本作用户名的假行）。
func ParseInvitees(doc *goquery.Document) []InviteeInfo {
	if doc == nil {
		return nil
	}
	var out []InviteeInfo
	rows := doc.Find("table tr").FilterFunction(func(_ int, tr *goquery.Selection) bool {
		return tr.ChildrenFiltered("td").Length() >= 3 && len([]rune(strings.TrimSpace(tr.Text()))) > 10
	})
	if rows.Length() == 0 {
		rows = doc.Find("table.bittable tr, table.main tr, table td.tableborder tr, div#outer tr").
			FilterFunction(func(_ int, tr *goquery.Selection) bool {
				return tr.ChildrenFiltered("td").Length() >= 3 && len([]rune(strings.TrimSpace(tr.Text()))) > 10
			})
	}
	rows.Each(func(_ int, tr *goquery.Selection) {
		text := strings.TrimSpace(tr.Text())
		if inviteeHeaderRe.MatchString(text) && len([]rune(text)) < 40 {
			return // 表头
		}
		cells := tr.ChildrenFiltered("td")
		if cells.Length() < 3 {
			return
		}
		// username + user_id（id 用于排除"自己"——邀请人页不该含邀请人自己）
		link := tr.Find(`a[href*="userdetails"]`).First()
		name := strings.TrimSpace(link.Text())
		firstCell := strings.TrimSpace(cells.First().Text())
		if name == "" {
			name = firstCell
		}
		// 空态/提示行防御：无 userdetails 链接且首格不是纯邮箱时，首格文本可能是
		// 整页提示文案（如「当前没有已发邀请」外围行），不作用户名。
		_, hasLink := link.Attr("href")
		if !hasLink && emailRe.FindString(firstCell) == "" {
			return
		}
		if len([]rune(name)) < 2 || len([]rune(name)) > 50 {
			return
		}
		userID := ""
		if href, ok := link.Attr("href"); ok {
			if m := inviteeIDRe.FindStringSubmatch(href); len(m) > 1 {
				userID = m[1]
			}
		}
		// 流量（前两个体积串 → uploaded/downloaded）
		var sizes []int64
		cells.Each(func(_ int, td *goquery.Selection) {
			if len(sizes) >= 2 {
				return
			}
			if b := ParseBytes(strings.TrimSpace(td.Text())); b > 0 {
				sizes = append(sizes, b)
			}
		})
		var up, down int64
		if len(sizes) > 0 {
			up = sizes[0]
		}
		if len(sizes) > 1 {
			down = sizes[1]
		}
		// 分享率
		ratio := ""
		cells.Each(func(_ int, td *goquery.Selection) {
			if ratio != "" {
				return
			}
			t := strings.TrimSpace(td.Text())
			if t == "∞" || strings.EqualFold(t, "inf") {
				ratio = t
				return
			}
			if ratioTextRe.MatchString(t) {
				ratio = t
			}
		})
		// 禁用/状态：扫描行 HTML（含属性）
		html, _ := tr.Html()
		stateHTML := text + " " + html
		disabled := inviteeDisabledRe.MatchString(stateHTML)
		status := "confirmed"
		switch {
		case disabled:
			status = "disabled"
		case inviteePendingRe.MatchString(stateHTML):
			status = "pending"
		}
		// email
		email := ""
		if m := emailRe.FindString(text); m != "" {
			email = m
		}
		out = append(out, InviteeInfo{
			UserID: userID, Username: name, Email: email,
			Uploaded: up, Downloaded: down, Ratio: ratio,
			Status: status, Enabled: !disabled,
		})
	})
	// 去重：按 username（小写）保留首次，防御页面冗余行/无效翻页累加。
	seen := make(map[string]bool)
	deduped := make([]InviteeInfo, 0, len(out))
	for _, iv := range out {
		key := strings.ToLower(strings.TrimSpace(iv.Username))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		deduped = append(deduped, iv)
	}
	return deduped
}

// atoiSafe 容错整数解析。
func atoiSafe(s string) int {
	n := 0
	for _, r := range strings.TrimSpace(s) {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// parseBonusFloat 解析带千分位的浮点。
func parseBonusFloat(s string) float64 {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	f := 0.0
	neg := false
	seenDot := false
	div := 1.0
	for i, r := range s {
		switch {
		case r == '-' && i == 0:
			neg = true
		case r >= '0' && r <= '9':
			if seenDot {
				div *= 10
				f = f + float64(r-'0')/div
			} else {
				f = f*10 + float64(r-'0')
			}
		case r == '.' && !seenDot:
			seenDot = true
		}
	}
	if neg {
		f = -f
	}
	return f
}

func formatInviteReason(perm, temp int) string {
	return "永久=" + itoaSimple(perm) + " 临时=" + itoaSimple(temp)
}

func itoaSimple(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
