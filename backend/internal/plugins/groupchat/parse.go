package groupchat

import (
	"strings"
)

// SiteMessages 一条站点的喊话消息配置(解析自 sites_messages textarea)。
type SiteMessages struct {
	SiteName string   // 站点名(子串匹配，如"好学"匹配"好学PT")
	Messages []string // 该站要发的消息列表
	Interval int      // 自定义间隔秒(0=用全局 interval_cnt)
}

// ParseSitesMessages 解析配置文本，每行格式 "SiteName|msg1|msg2|...|Ns"。
// 尾部 "|Ns" 表示自定义间隔秒。# 开头注释，空行跳过。转义：消息内若含 "|" 暂不支持(原版同此限制)。
func ParseSitesMessages(raw string) []SiteMessages {
	var out []SiteMessages
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 2 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		if name == "" {
			continue
		}
		msgs := make([]string, 0, len(parts)-1)
		interval := 0
		for _, p := range parts[1:] {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			// 尾部 "Ns" 形式（数字+s）作为间隔
			if len(p) > 1 && strings.HasSuffix(p, "s") {
				if n, ok := atoiPositive(strings.TrimSuffix(p, "s")); ok {
					interval = n
					continue
				}
			}
			msgs = append(msgs, p)
		}
		if len(msgs) == 0 {
			continue
		}
		out = append(out, SiteMessages{SiteName: name, Messages: msgs, Interval: interval})
	}
	return out
}

// MessagesForSite 汇总所有匹配 site 的消息集(站名子串匹配，大小写不敏感)。
// 返回合并后的消息列表 + 自定义间隔(取首条匹配的 Interval，0=用全局)。
func MessagesForSite(all []SiteMessages, site SiteRef) ([]string, int) {
	var msgs []string
	interval := 0
	lname := strings.ToLower(site.Name)
	for _, sm := range all {
		if strings.Contains(lname, strings.ToLower(sm.SiteName)) ||
			strings.Contains(strings.ToLower(sm.SiteName), lname) {
			msgs = append(msgs, sm.Messages...)
			if interval == 0 {
				interval = sm.Interval
			}
		}
	}
	return msgs, interval
}

// MatchSiteByName 从站点列表里找名字含 name 子串的(双向子串，大小写不敏感)。
func MatchSiteByName(sites []SiteRef, name string) (SiteRef, bool) {
	lname := strings.ToLower(name)
	for _, s := range sites {
		ls := strings.ToLower(s.Name)
		if strings.Contains(ls, lname) || strings.Contains(lname, ls) {
			return s, true
		}
	}
	return SiteRef{}, false
}

func atoiPositive(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	if n <= 0 {
		return 0, false
	}
	return n, true
}
