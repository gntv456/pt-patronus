package chain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/site/assessment"

	"golang.org/x/sync/singleflight"
)

// 考核缓存参数（对齐原版：内存 30min、DB 6h）。
const (
	assessMemTTL      = 30 * time.Minute
	assessDBTTL       = 6 * time.Hour
	assessTimeout     = 30 * time.Second
	assessConcurrency = 5
)

// assessHTTP 把 httpclient.Client 适配为 assessment.HTTPDoer。
type assessHTTP struct{ c *httpclient.Client }

func (w assessHTTP) Get(ctx context.Context, url, cookie string, ssrf bool) (*assessment.Response, error) {
	r, err := w.c.Get(ctx, url, cookie, ssrf)
	if err != nil {
		return nil, err
	}
	return &assessment.Response{StatusCode: r.StatusCode, Body: r.Body, FinalURL: r.FinalURL}, nil
}

func (w assessHTTP) Do(ctx context.Context, method, url string, body []byte, headers map[string]string, cookie string, ssrf bool) (*assessment.Response, error) {
	r, err := w.c.Do(ctx, method, url, body, headers, cookie, ssrf)
	if err != nil {
		return nil, err
	}
	return &assessment.Response{StatusCode: r.StatusCode, Body: r.Body, FinalURL: r.FinalURL}, nil
}

// assessCacheEntry 内存缓存条目。
type assessCacheEntry struct {
	tasks []assessment.Task
	at    time.Time
}

// AssessmentCard 考核 UI 卡片（API 返回）。
type AssessmentCard struct {
	SiteID          uint                `json:"site_id"`
	SiteName        string              `json:"site_name"`
	Status          string              `json:"status"` // SAFE/PROGRESS/DANGER/EXEMPT/UNKNOWN
	ProgressPercent float64             `json:"progress_percent"`
	RemainingDays   int                 `json:"remaining_days"`
	StartTime       int64               `json:"start_time"` // epoch ms（0=未知）
	EndTime         int64               `json:"end_time"`   // epoch ms（0=未知）；考核截止时间
	Metrics         []assessment.Metric `json:"metrics"`
	AssessmentType  string              `json:"assessment_type"`
	Description     string              `json:"description,omitempty"` // 考核说明（馒头"考核期間不享受所有魔力加成"等）
	Error           string              `json:"error,omitempty"`
}

// normalized 保证 Metrics 非 nil：nil 切片 JSON 序列化为 null，前端 .length 会崩
// （EXEMPT/UNKNOWN 卡片无指标时 Metrics 为 nil）。所有对外出口（AssessAll/AssessOne）统一调用。
func (c AssessmentCard) normalized() AssessmentCard {
	if c.Metrics == nil {
		c.Metrics = []assessment.Metric{}
	}
	return c
}

// AssessmentChain 编排站点考核（3 级缓存 内存→DB→网络 + per-platform 适配）。
type AssessmentChain struct {
	siteCh *SiteChain
	reg    *assessment.Registry
	op     *oper.AssessmentOper
	hc     assessment.HTTPDoer
	notice *notice.Service
	mu     sync.Mutex
	cache  map[uint]assessCacheEntry
	sf     singleflight.Group // 同 siteID 考核并发抓取合并（防击穿）
}

// NewAssessmentChain 构造。ns 为站内通知中心（考核完成/失败发通知；nil 跳过）。
func NewAssessmentChain(siteCh *SiteChain, reg *assessment.Registry, op *oper.AssessmentOper, hc *httpclient.Client, ns *notice.Service) *AssessmentChain {
	return &AssessmentChain{siteCh: siteCh, reg: reg, op: op, hc: assessHTTP{c: hc}, notice: ns, cache: map[uint]assessCacheEntry{}}
}

// AssessAll 跨站并发聚合考核（按 progress 降序）。单站 miss 走 singleflight 合并同 siteID 并发回源。
func (c *AssessmentChain) AssessAll(ctx context.Context, force bool) []AssessmentCard {
	sites, err := c.siteCh.sites.ListActive()
	if err != nil {
		slog.Warn("assessment list sites failed", "err", err)
		return nil
	}
	cards := make([]AssessmentCard, len(sites))
	sem := make(chan struct{}, assessConcurrency)
	var wg sync.WaitGroup
	for i, s := range sites {
		wg.Add(1)
		go func(i int, s models.Site) {
			defer wg.Done()
			cards[i] = c.assessSite(ctx, sem, &s, force)
		}(i, s)
	}
	wg.Wait()

	out := make([]AssessmentCard, 0, len(cards))
	for _, card := range cards {
		if card.SiteID != 0 {
			out = append(out, card.normalized())
		}
	}
	// progress 降序
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1].ProgressPercent < out[j].ProgressPercent; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// AssessOne 单站考核（强制网络刷新；单站不限并发，sem=nil）。
func (c *AssessmentChain) AssessOne(ctx context.Context, s *models.Site) AssessmentCard {
	return c.assessSite(ctx, nil, s, true).normalized()
}

// assessSite 单站 3 级缓存：内存→DB→网络。sem 为聚合级并发闸（nil 表示不限，单站用）。
// force=true 绕过缓存与 singleflight 直接网络回源；否则 miss 时 singleflight 合并同 siteID
// 并发回源（防击穿，sem 仅 leader 占用）。3 级缓存包整个回源——避免 waiter 各自 BatchUpsert + 发通知。
func (c *AssessmentChain) assessSite(ctx context.Context, sem chan struct{}, s *models.Site, force bool) AssessmentCard {
	def := c.siteCh.defFor(s)
	adapter := c.reg.Get(def.Schema)
	if !adapter.Supported() {
		return AssessmentCard{SiteID: s.ID, SiteName: s.Name, Status: "EXEMPT",
			ProgressPercent: 100, AssessmentType: "该平台无考核"}
	}
	if force {
		return c.fetchAssessSite(ctx, sem, s, def, adapter)
	}
	// 快速路径：内存命中直接返回（不经 singleflight）。
	c.mu.Lock()
	e, ok := c.cache[s.ID]
	c.mu.Unlock()
	if ok && time.Since(e.at) <= assessMemTTL {
		return convertTaskToCard(s.ID, s.Name, primaryTask(e.tasks), "")
	}
	// miss → singleflight 合并同 siteID 并发回源。
	card, _ := sfDo(&c.sf, strconv.Itoa(int(s.ID)), func() (AssessmentCard, error) {
		// double-check 内存：leader 完成后紧随的 caller 重跑 fn，缓存可能已满。
		c.mu.Lock()
		ce, ok := c.cache[s.ID]
		c.mu.Unlock()
		if ok && time.Since(ce.at) <= assessMemTTL {
			return convertTaskToCard(s.ID, s.Name, primaryTask(ce.tasks), ""), nil
		}
		// double-check DB（6h）：另一调用可能在 leader 期间刚写 DB。
		if latest, err := c.op.LatestFetchedAt(s.ID); err == nil && !latest.IsZero() && time.Since(latest) <= assessDBTTL {
			dbTasks, _ := c.op.FindBySite(s.ID)
			tasks := dbTasksToAssess(dbTasks)
			c.mu.Lock()
			c.cache[s.ID] = assessCacheEntry{tasks: tasks, at: time.Now()}
			c.mu.Unlock()
			return convertTaskToCard(s.ID, s.Name, primaryTask(tasks), ""), nil
		}
		return c.fetchAssessSite(ctx, sem, s, def, adapter), nil
	})
	return card
}

// fetchAssessSite 网络抓取考核 + BatchUpsert + 状态转换通知 + 内存缓存。
// sem 非空时占一个槽（聚合级并发闸；nil 表示不限，单站 AssessOne 用）。
func (c *AssessmentChain) fetchAssessSite(ctx context.Context, sem chan struct{}, s *models.Site, def *site.SiteMetadata, adapter assessment.Adapter) AssessmentCard {
	if sem != nil {
		sem <- struct{}{}
		defer func() { <-sem }()
	}
	siteCtx, cancel := context.WithTimeout(ctx, assessTimeout)
	defer cancel()
	token := c.siteCh.credential(s, def)
	tasks, err := adapter.FetchAssessments(siteCtx, c.hc, assessDef(def), s.BaseURL, token)
	if err != nil {
		if errors.Is(err, site.ErrNotLoggedIn) {
			return AssessmentCard{SiteID: s.ID, SiteName: s.Name, Status: "UNKNOWN", Error: "未登录"}
		}
		return AssessmentCard{SiteID: s.ID, SiteName: s.Name, Status: "UNKNOWN", Error: err.Error()}
	}
	// 持久化 + 内存缓存
	if len(tasks) > 0 {
		// 读旧状态（BatchUpsert 是 delete-then-insert，旧数据会被清）
		oldTasks, _ := c.op.FindBySite(s.ID)
		oldStatus := make(map[string]string, len(oldTasks))
		for _, ot := range oldTasks {
			oldStatus[ot.TaskName] = ot.Status
		}
		if err := c.op.BatchUpsert(s.ID, assessToDB(tasks), time.Now()); err != nil {
			slog.Warn("assessment BatchUpsert failed, results not persisted", "site", s.ID, "err", err)
		}
		// 状态转换检测 → 通知（首次该任务无旧记录则跳过，避免初始化刷屏）
		if c.notice != nil {
			for i := range tasks {
				t := &tasks[i]
				newSt := string(t.Status)
				oldSt := oldStatus[t.Name]
				if oldSt == "" || oldSt == newSt {
					continue
				}
				switch newSt {
				case string(assessment.StatusCompleted):
					if e := c.notice.Notice(ctx, notice.Input{
						Source: notice.SourceAssessPass,
						Key:    fmt.Sprintf("assess:%d:%s:pass", s.ID, t.Name),
						Title:  "考核完成：" + s.Name,
						Body:   t.Name + " 已完成",
						Level:  "success",
					}, true); e != nil {
						slog.Warn("assessment notice failed", "site", s.ID, "task", t.Name, "event", "pass", "err", e)
					}
				case string(assessment.StatusFailed):
					if e := c.notice.Notice(ctx, notice.Input{
						Source: notice.SourceAssessFail,
						Key:    fmt.Sprintf("assess:%d:%s:fail", s.ID, t.Name),
						Title:  "考核失败：" + s.Name,
						Body:   t.Name + " 未通过",
						Level:  "danger",
					}, true); e != nil {
						slog.Warn("assessment notice failed", "site", s.ID, "task", t.Name, "event", "fail", "err", e)
					}
				}
			}
		}
	}
	c.mu.Lock()
	c.cache[s.ID] = assessCacheEntry{tasks: tasks, at: time.Now()}
	c.mu.Unlock()

	if len(tasks) == 0 {
		return AssessmentCard{SiteID: s.ID, SiteName: s.Name, Status: "EXEMPT",
			ProgressPercent: 100, AssessmentType: "暂无活跃任务"}
	}
	return convertTaskToCard(s.ID, s.Name, primaryTask(tasks), "")
}

// assessDef 把 site.SiteMetadata 投影为 assessment.Def。
func assessDef(def *site.SiteMetadata) assessment.Def {
	d := assessment.Def{Schema: def.Schema}
	d.UserInfo.DetailPath = def.UserInfo.DetailPath
	return d
}

// primaryTask 取首个 ACTIVE/COMPLETED 任务，否则首个。
func primaryTask(tasks []assessment.Task) *assessment.Task {
	if len(tasks) == 0 {
		return nil
	}
	for i := range tasks {
		if tasks[i].Status == assessment.StatusActive || tasks[i].Status == assessment.StatusCompleted {
			return &tasks[i]
		}
	}
	return &tasks[0]
}

// convertTaskToCard 把任务转为 UI 卡片（progress% + 状态阈值按指标计数整数除法）。
func convertTaskToCard(siteID uint, siteName string, t *assessment.Task, errMsg string) AssessmentCard {
	card := AssessmentCard{SiteID: siteID, SiteName: siteName, Error: errMsg}
	if t == nil {
		card.Status = "EXEMPT"
		card.ProgressPercent = 100
		return card
	}
	card.Metrics = t.Metrics
	card.AssessmentType = t.Name
	card.Description = t.Description
	card.StartTime = t.StartTime
	card.EndTime = t.EndTime

	// progress% 只对 required>0 指标取均值
	hasTarget := false
	completedCount := 0
	totalTarget := 0
	var progressSum, progressCnt float64
	for _, m := range t.Metrics {
		if m.Required > 0 {
			hasTarget = true
			totalTarget++
			if m.IsCompleted {
				completedCount++
			}
			ratio := m.Current / m.Required
			if ratio < 0 {
				ratio = 0
			}
			if ratio > 1 {
				ratio = 1
			}
			progressSum += ratio
			progressCnt++
		}
	}
	switch t.Status {
	case assessment.StatusCompleted:
		card.ProgressPercent = 100
	case assessment.StatusActive, assessment.StatusUnknown:
		if progressCnt > 0 {
			card.ProgressPercent = math.Round(progressSum / progressCnt * 100)
		}
	case assessment.StatusFailed:
		card.ProgressPercent = 0
	}

	// UI 状态阈值（按指标计数整数除法，对齐原版 convertTaskToCard）
	switch t.Status {
	case assessment.StatusCompleted:
		card.Status = "SAFE"
	case assessment.StatusFailed:
		card.Status = "DANGER"
	case assessment.StatusExempt:
		card.Status = "EXEMPT"
	case assessment.StatusUnknown:
		card.Status = "UNKNOWN"
	default: // ACTIVE
		switch {
		case !hasTarget:
			card.Status = "PROGRESS"
		case completedCount == totalTarget:
			card.Status = "SAFE"
		case completedCount*10 < totalTarget*3:
			card.Status = "DANGER"
		default: // >= total*6/10 或中间
			card.Status = "PROGRESS"
		}
	}

	// 剩余天数
	if t.EndTime > 0 {
		remain := t.EndTime - time.Now().UnixMilli()
		if remain > 0 {
			card.RemainingDays = int(math.Ceil(float64(remain) / float64(24*time.Hour/time.Millisecond)))
		} else {
			card.RemainingDays = -1
		}
	}
	return card
}

// ── assessment.Task ↔ models.AssessmentTask 转换 ──

func assessToDB(tasks []assessment.Task) []models.AssessmentTask {
	out := make([]models.AssessmentTask, 0, len(tasks))
	for _, t := range tasks {
		metricsJSON, _ := json.Marshal(t.Metrics)
		out = append(out, models.AssessmentTask{
			TaskName:    t.Name,
			Description: t.Description,
			StartTime:   t.StartTime,
			EndTime:     t.EndTime,
			Status:      string(t.Status),
			MetricsJSON: string(metricsJSON),
			RawJSON:     t.RawJSON,
		})
	}
	return out
}

func dbTasksToAssess(dbTasks []models.AssessmentTask) []assessment.Task {
	out := make([]assessment.Task, 0, len(dbTasks))
	for _, dt := range dbTasks {
		var metrics []assessment.Metric
		_ = json.Unmarshal([]byte(dt.MetricsJSON), &metrics)
		out = append(out, assessment.Task{
			Name: dt.TaskName, Description: dt.Description,
			StartTime: dt.StartTime, EndTime: dt.EndTime,
			Status: assessment.Status(dt.Status), Metrics: metrics, FetchedAt: dt.FetchedAt,
		})
	}
	return out
}

// AssessAllStream 流式推送跨站考核：每站完成即 emit(card)。供 SSE 端点「加载一个显示一个」。
// 与 AssessAll 不同：不排序（流式按到达顺序显示，渐进加载下排序反而使卡片跳动）；不走额外的
// singleflight 编排——assessSite 内部每站已有 3 级缓存（内存→DB→网络）+ per-site singleflight，
// 足以压低并发回源压力。force=true 各站强制网络刷新。
// 注意：emit 会被多个站点 goroutine 并发调用，调用方（handler）须自行串行化实际写入。
func (c *AssessmentChain) AssessAllStream(ctx context.Context, force bool, emit func(AssessmentCard)) {
	sites, err := c.siteCh.sites.ListActive()
	if err != nil {
		slog.Warn("assessment list sites failed", "err", err)
		return
	}
	sem := make(chan struct{}, assessConcurrency)
	var wg sync.WaitGroup
	for _, s := range sites {
		wg.Add(1)
		go func(s models.Site) {
			defer wg.Done()
			card := c.assessSite(ctx, sem, &s, force)
			if card.SiteID != 0 {
				emit(card.normalized())
			}
		}(s)
	}
	wg.Wait()
}
