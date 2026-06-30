package schemas

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/site"
)

// Checkin 实现 site.CheckinCapable：POST attendance.php 签到。
// NexusPHP 标准：成功→HTML 含「签到成功」+「获得 X 火花/魔力」；已签→stderr 页。
func (n *NexusPHP) Checkin(ctx context.Context, def *site.SiteMetadata, baseURL, cookie string) (*site.CheckinResult, error) {
	def = site.EnsureDefaults(def)
	resp, err := n.hc.PostForm(ctx, joinURL(baseURL, "attendance.php"), cookie, "", true)
	if err != nil {
		return nil, err
	}
	if resp.IsCloudflare() {
		return nil, fmt.Errorf("blocked by cloudflare on %s", baseURL)
	}
	body := string(resp.Body)

	// 签到成功
	if strings.Contains(body, "签到成功") {
		return &site.CheckinResult{
			Success: true,
			Message: "签到成功",
			Bonus:   parseCheckinBonus(body),
		}, nil
	}
	// 已签到（账号仍存活，视为成功）
	lower := strings.ToLower(body)
	if strings.Contains(body, "已经签到") || strings.Contains(body, "已签到") || strings.Contains(lower, "already") {
		return &site.CheckinResult{Success: true, Message: "今天已签到"}, nil
	}
	// 未登录
	doc, _ := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if loggedOut(resp, doc, def.NoLogin) {
		return &site.CheckinResult{Success: false, Message: site.ErrNotLoggedIn.Error()}, nil
	}
	return &site.CheckinResult{Success: false, Message: fmt.Sprintf("签到响应未识别(状态%d)", resp.StatusCode)}, nil
}

var bonusRe = regexp.MustCompile(`获得\s*(\d+(?:\.\d+)?)\s*(?:个)?\s*(?:火花|魔力|bonus)`)

func parseCheckinBonus(body string) float64 {
	m := bonusRe.FindStringSubmatch(body)
	if len(m) >= 2 {
		f, _ := strconv.ParseFloat(m[1], 64)
		return f
	}
	return 0
}
