package chain

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/site/assessment"
	"github.com/hxpt/ptpatronus/internal/core/safego"
)

// assessDef 把 site.SiteMetadata 投影为 assessment.Def。
func assessDef(def *site.SiteMetadata) assessment.Def {
	d := assessment.Def{Schema: def.Schema, ID: def.ID}
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
		safego.Go("assessment:434", func() {
			defer wg.Done()
			card := c.assessSite(ctx, sem, &s, force)
			if card.SiteID != 0 {
				emit(card.normalized())
			}
		})
	}
	wg.Wait()
}
