package groupchat

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)


// Engine 编排多站喊话：站点派发 → 逐条 Send → 奖励副作用 → PollFeedback → 失败重试 → 通知。
// 由 plugins 层 group-chat-zone 的 ExecuteAction/Schedule 触发。
type Engine struct {
	rt  Runtime
	hc  *httpclient.Client // 可空：lazy New 兜底
	cfg map[string]any
}

func NewEngine(rt Runtime, hc *httpclient.Client, cfg map[string]any) *Engine {
	return &Engine{rt: rt, hc: hc, cfg: cfg}
}

// client 返回可用 HTTP 客户端；hc 为空时 lazy 新建(plugins 层 production 会注入)。
func (e *Engine) client() (*httpclient.Client, error) {
	if e.hc != nil {
		return e.hc, nil
	}
	c, err := httpclient.New(httpclient.Options{FollowRedirect: true, Timeout: 20 * time.Second})
	if err != nil {
		return nil, err
	}
	e.hc = c
	return c, nil
}

// Run 主任务：跑非织梦站。织梦在独立调度(zm_cron)下运行；引导期(zm_cron 空)随主任务跑一次以初始化。
func (e *Engine) Run(ctx context.Context) {
	hc, err := e.client()
	if err != nil {
		e.rt.Log(logError, "run", "http client unavailable: "+err.Error(), nil)
		return
	}
	all := e.rt.SiteList()
	selected := e.resolveSelectedSites(all)
	if len(selected) == 0 {
		e.rt.Log(logWarning, "run", "no sites selected (check chat_sites)", nil)
		return
	}
	allMsgs := ParseSitesMessages(strCfg(e.cfg, "sites_messages"))
	zmBootstrapping := hasZmSite(selected) && strings.TrimSpace(strCfg(e.cfg, "zm_cron")) == ""
	globalInterval := intCfg(e.cfg, "interval_cnt", 2)

	var results []SiteResult
	var failed []FailedMessage
	for _, site := range selected {
		isZm := strings.Contains(site.Name, "织梦")
		if isZm && !zmBootstrapping {
			continue // 织梦已走独立调度，主任务跳过
		}
		msgs, interval := MessagesForSite(allMsgs, site)
		if len(msgs) == 0 {
			continue
		}
		handler := Dispatch(site)
		if handler == nil {
			continue
		}
		if interval == 0 {
			interval = globalInterval
		}
		res, errs := e.runSite(ctx, hc, site, handler, msgs, interval)
		results = append(results, res)
		failed = append(failed, errs...)
	}

	if zmBootstrapping {
		e.scheduleNextZm(ctx, hc, selected)
	}
	if boolCfg(e.cfg, "notify", true) && len(results) > 0 {
		body := ComposeReport(ReportInput{Results: results})
		e.rt.Notice(ctx, "群聊区任务完成", body, levelFor(results))
	}
	if len(failed) > 0 {
		e.enqueueRetry(ctx, failed)
	}
	e.rt.Log(logInfo, "run", fmt.Sprintf("groupchat main run: %d sites, %d failed", len(results), len(failed)), map[string]any{"failed": len(failed)})
}

// RunZm 织梦独立任务：仅织梦站，zm_interval 间隔，跑完按邮件时间+24h 更新 zm_cron。
func (e *Engine) RunZm(ctx context.Context) {
	hc, err := e.client()
	if err != nil {
		e.rt.Log(logError, "zm", "http client unavailable: "+err.Error(), nil)
		return
	}
	all := e.rt.SiteList()
	selected := e.resolveSelectedSites(all)
	allMsgs := ParseSitesMessages(strCfg(e.cfg, "sites_messages"))
	zmInterval := intCfg(e.cfg, "zm_interval", 60)

	var results []SiteResult
	var failed []FailedMessage
	for _, site := range selected {
		if !strings.Contains(site.Name, "织梦") {
			continue
		}
		msgs, _ := MessagesForSite(allMsgs, site)
		if len(msgs) == 0 {
			continue
		}
		handler := Dispatch(site)
		if handler == nil {
			continue
		}
		res, errs := e.runSite(ctx, hc, site, handler, msgs, zmInterval)
		results = append(results, res)
		failed = append(failed, errs...)
	}
	if boolCfg(e.cfg, "notify", true) && len(results) > 0 {
		body := ComposeReport(ReportInput{Results: results})
		e.rt.Notice(ctx, "织梦喊话完成", body, levelFor(results))
	}
	e.scheduleNextZm(ctx, hc, selected)
	if len(failed) > 0 {
		e.enqueueRetry(ctx, failed)
	}
	e.rt.Log(logInfo, "zm", fmt.Sprintf("groupchat zm run: %d sites, %d failed", len(results), len(failed)), nil)
}

// runSite 跑单站：cookie 解密 → 奖励副作用(发前)→ 逐条 Send+PollFeedback → 收集结果/失败。
func (e *Engine) runSite(ctx context.Context, hc *httpclient.Client, site SiteRef, handler Handler, msgs []string, interval int) (SiteResult, []FailedMessage) {
	res := SiteResult{Site: site}
	var failed []FailedMessage
	cookie, err := e.rt.SiteCookie(site.ID)
	if err != nil {
		for _, m := range msgs {
			failed = append(failed, FailedMessage{SiteID: site.ID, SiteName: site.Name, Message: m, Interval: interval, Error: "cookie: " + err.Error()})
		}
		res.Failed = failed
		return res, failed
	}
	c := Ctx{HC: hc, Site: site, Cookie: cookie, Cfg: e.cfg, Username: site.Username}

	// 发前奖励副作用(仅对实现了对应接口且开关开启的 handler 生效)
	res.BonusText = e.tryBonus(ctx, handler, c)
	res.LotteryText = e.tryLottery(ctx, handler, c)
	e.tryBlessing(ctx, handler, c)

	getFb := boolCfg(e.cfg, "get_feedback", true)
	for i, msg := range msgs {
		fb, sErr := handler.Send(ctx, c, msg)
		if sErr != nil {
			failed = append(failed, FailedMessage{SiteID: site.ID, SiteName: site.Name, Message: msg, Interval: interval, Error: sErr.Error()})
			continue
		}
		if fb == nil {
			fb = &Feedback{}
		}
		if getFb {
			if p, ok := handler.(Poller); ok {
				if fb2, pErr := p.PollFeedback(ctx, c, msg, fb); pErr == nil && fb2 != nil {
					fb = fb2
				}
			}
		}
		res.Feedback = append(res.Feedback, *fb)
		if i < len(msgs)-1 && interval > 0 {
			if cancelled := sleepCtx(ctx, time.Duration(interval)*time.Second); cancelled {
				break
			}
		}
	}
	res.Failed = failed
	return res, failed
}

// tryBonus 青蛙每日福利(实现 DailyBonusBuyer 且 qingwa_daily_bonus 开)。
func (e *Engine) tryBonus(ctx context.Context, h Handler, c Ctx) string {
	b, ok := h.(DailyBonusBuyer)
	if !ok || !boolCfg(e.cfg, "qingwa_daily_bonus", false) {
		return ""
	}
	txt, err := b.BuyDailyBonus(ctx, c)
	if err != nil {
		return "失败：" + err.Error()
	}
	return txt
}

// tryLottery LongPT 每日抽奖(实现 LotteryPlayer 且 longpt_daily_lottery 开)。
func (e *Engine) tryLottery(ctx context.Context, h Handler, c Ctx) string {
	l, ok := h.(LotteryPlayer)
	if !ok || !boolCfg(e.cfg, "longpt_daily_lottery", false) {
		return ""
	}
	txt, err := l.PlayLottery(ctx, c)
	if err != nil {
		return "失败：" + err.Error()
	}
	return txt
}

// tryBlessing 13City 诸神赐福勋章(实现 BlessingMedal 且 thirteencity_auto_buy_blessing 开)。
func (e *Engine) tryBlessing(ctx context.Context, h Handler, c Ctx) {
	b, ok := h.(BlessingMedal)
	if !ok || !boolCfg(e.cfg, "thirteencity_auto_buy_blessing", false) {
		return
	}
	_, _ = b.EnsureBlessing(ctx, c)
}

// scheduleNextZm 抓织梦邮件时间 → next=mail+24h → 写回 zm_cron(M H D *)触发调度重排。
// 失败保留旧 cron(下次再试)。这是织梦"邮件时间+24h 独立调度"的核心。
func (e *Engine) scheduleNextZm(ctx context.Context, hc *httpclient.Client, selected []SiteRef) {
	var zmSite SiteRef
	for _, s := range selected {
		if strings.Contains(s.Name, "织梦") {
			zmSite = s
			break
		}
	}
	if zmSite.ID == 0 {
		return
	}
	handler := Dispatch(zmSite)
	mf, ok := handler.(MailTimeFetcher)
	if !ok {
		return
	}
	cookie, err := e.rt.SiteCookie(zmSite.ID)
	if err != nil {
		e.rt.Log(logWarning, "zm", "zm cookie: "+err.Error(), nil)
		return
	}
	c := Ctx{HC: hc, Site: zmSite, Cookie: cookie, Cfg: e.cfg, Username: zmSite.Username}
	mailTime, err := mf.LatestMailTime(ctx, c)
	if err != nil {
		e.rt.Log(logWarning, "zm", "zm mail time fetch failed: "+err.Error(), nil)
		return
	}
	next := mailTime.Add(24 * time.Hour)
	cron := fmt.Sprintf("%d %d %d *", next.Minute(), next.Hour(), next.Day())
	if err := e.rt.SetConfig("zm_cron", cron); err != nil {
		e.rt.Log(logError, "zm", "set zm_cron failed: "+err.Error(), nil)
		return
	}
	e.rt.Log(logInfo, "zm", fmt.Sprintf("zm next: %s (cron %s)", next.Format("2006-01-02 15:04"), cron), nil)
}

// —— 失败重试(KV 持久化 + time.AfterFunc 一次性递归) ——

const kvFailedMessages = "failed_messages"
const kvRetryPasses = "retry_passes_left"

func (e *Engine) enqueueRetry(ctx context.Context, failed []FailedMessage) {
	retryCount := intCfg(e.cfg, "retry_count", 2)
	if retryCount <= 0 || len(failed) == 0 {
		_ = e.rt.KVSet(kvFailedMessages, []FailedMessage{})
		return
	}
	_ = e.rt.KVSet(kvFailedMessages, failed)
	_ = e.rt.KVSet(kvRetryPasses, retryCount)
	retryInterval := intCfg(e.cfg, "retry_interval", 10)
	next := time.Now().Add(time.Duration(retryInterval) * time.Minute)
	if boolCfg(e.cfg, "retry_notify", false) {
		e.rt.Notice(ctx, "喊话失败将重试", ComposeRetryNotice(failed, next), "warning")
	}
	e.rt.Log(logInfo, "retry", fmt.Sprintf("enqueued %d failed, retry at %s", len(failed), next.Format("15:04:05")), nil)
	time.AfterFunc(time.Duration(retryInterval)*time.Minute, func() {
		e.executeRetry(context.Background())
	})
}

func (e *Engine) executeRetry(ctx context.Context) {
	// AfterFunc 裸跑；HTML 解析 panic 不应崩进程级 goroutine（断重试链）。
	defer func() {
		if r := recover(); r != nil {
			slog.Error("groupchat retry panic", "error", r)
		}
	}()
	var failed []FailedMessage
	if err := e.rt.KVGet(kvFailedMessages, &failed); err != nil || len(failed) == 0 {
		return
	}
	hc, err := e.client()
	if err != nil {
		e.rt.Log(logError, "retry", "http client unavailable: "+err.Error(), nil)
		return
	}
	all := e.rt.SiteList()
	var still []FailedMessage
	for _, fm := range failed {
		site, ok := findSiteByID(all, fm.SiteID)
		if !ok {
			continue
		}
		handler := Dispatch(site)
		if handler == nil {
			continue
		}
		cookie, err := e.rt.SiteCookie(site.ID)
		if err != nil {
			still = append(still, fm)
			continue
		}
		c := Ctx{HC: hc, Site: site, Cookie: cookie, Cfg: e.cfg, Username: site.Username}
		if _, err := handler.Send(ctx, c, fm.Message); err != nil {
			fm.Error = err.Error()
			still = append(still, fm)
		}
	}
	passes := 0
	_ = e.rt.KVGet(kvRetryPasses, &passes)
	if len(still) == 0 {
		_ = e.rt.KVSet(kvFailedMessages, []FailedMessage{})
		_ = e.rt.KVDelete(kvRetryPasses)
		e.rt.Log(logInfo, "retry", "all failed messages recovered", nil)
		return
	}
	_ = e.rt.KVSet(kvFailedMessages, still)
	if passes > 1 {
		_ = e.rt.KVSet(kvRetryPasses, passes-1)
		retryInterval := intCfg(e.cfg, "retry_interval", 10)
		time.AfterFunc(time.Duration(retryInterval)*time.Minute, func() {
			e.executeRetry(context.Background())
		})
		e.rt.Log(logInfo, "retry", fmt.Sprintf("%d still failed, %d passes left", len(still), passes-1), nil)
	} else {
		_ = e.rt.KVDelete(kvRetryPasses)
		if boolCfg(e.cfg, "notify", true) {
			e.rt.Notice(ctx, "喊话重试终止", fmt.Sprintf("仍有 %d 条失败，已达重试上限。", len(still)), "warning")
		}
	}
}
