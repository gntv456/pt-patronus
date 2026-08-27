package chain

// 工作台聚合（对标 MSaber「天序工作台」）：一次拉取仪表盘首屏所需的今日域指标——
// 流转节点（站点/订阅/下载/整理/媒体库/观影六域计数）、今日待办、今日动态、今日摘要、
// 媒体库态势与体检分。纯 DB 聚合（媒体库计数除外，走默认媒体服务器 Libraries + 缓存），
// 单接口返回避免前端六次串行请求。

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// WorkbenchNode 流转节点（站点/订阅/下载/整理/媒体库/观影）。
type WorkbenchNode struct {
	Key   string `json:"key"`   // site | subscribe | download | organize | library | watch
	Label string `json:"label"` // 站点 | 订阅 | 下载 | 整理 | 媒体库 | 观影
	Count int    `json:"count"` // 域内关键计数（详见 Summary 对应字段）
	Hint  string `json:"hint"`  // 计数含义（UI 副标题）
}

// WorkbenchTodo 今日待办（可行动事项：失败整理/H&R 待处理/无数据站点）。
type WorkbenchTodo struct {
	Kind   string `json:"kind"` // transfer_failed | no_data_site | hr_risk
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
	Link   string `json:"link,omitempty"` // 前端路由（点击跳处理页）
	Level  string `json:"level"`          // info | warning | danger
}

// WorkbenchEvent 今日动态（事件流，带可选进度）。
type WorkbenchEvent struct {
	Source    string    `json:"source"`
	Title     string    `json:"title"`
	Level     string    `json:"level"`
	CreatedAt time.Time `json:"created_at"`
	Progress  *int      `json:"progress,omitempty"` // 云端任务进度 0-100（非任务事件为空）
}

// WorkbenchMediaLibrary 媒体库态势（默认媒体服务器库计数；未配置时 Configured=false）。
type WorkbenchMediaLibrary struct {
	Configured bool               `json:"configured"`
	Servers    []MediaLibraryStat `json:"servers"`
}

// MediaLibraryStat 单服务器库计数（电影/剧集由 Library.Type 聚合）。
type MediaLibraryStat struct {
	ServerID   uint      `json:"server_id"`
	ServerName string    `json:"server_name"`
	Type       string    `json:"type"` // emby | jellyfin | plex
	Movies     int       `json:"movies"`
	Series     int       `json:"series"`
	LibraryCnt int       `json:"library_count"`
	SyncedAt   time.Time `json:"synced_at"` // 本次聚合时间
}

// Workbench 工作台聚合响应。
type Workbench struct {
	// 流转节点（今日域计数）
	Nodes []WorkbenchNode `json:"nodes"`
	// 今日待办
	Todos []WorkbenchTodo `json:"todos"`
	// 今日动态（最近 20 条，含带进度的云端任务）
	Events []WorkbenchEvent `json:"events"`
	// 今日摘要
	Summary struct {
		SitesActive       int   `json:"sites_active"`        // 启用站点数
		SitesNoData       int   `json:"sites_no_data"`       // 无采集数据站点
		SubscriptionsNew  int   `json:"subscriptions_new"`   // 今日新增订阅
		SubscriptionsHits int   `json:"subscription_hits"`   // 今日订阅命中
		DownloadsDone     int   `json:"downloads_done"`      // 今日整理成功
		OrganizeFailed    int   `json:"organize_failed"`     // 今日整理失败
		CloudTasksDone    int   `json:"cloud_tasks_done"`    // 今日云端任务成功
		CloudTasksPending int   `json:"cloud_tasks_pending"` // 当前排队中云端任务
		UploadedDelta     int64 `json:"uploaded_delta"`      // 今日上传增量（跨站）
		DownloadedDelta   int64 `json:"downloaded_delta"`    // 今日下载增量（跨站）
		PlayedToday       int   `json:"played_today"`        // 今日播放 webhook 事件
		StrmGenerated     int   `json:"strm_generated"`      // 今日 STRM 生成记录数
	} `json:"summary"`
	// 媒体库态势
	MediaLibrary WorkbenchMediaLibrary `json:"media_library"`
	// 体检分（轻量：无外部探测，仅 DB/队列派生；完整诊断走 /system/doctor）
	HealthScore int       `json:"health_score"`
	GeneratedAt time.Time `json:"generated_at"`
}

// greeting 由服务端本地时间生成问候语（前端也有兜底）。
func workbenchGreeting(t time.Time) string {
	h := t.Hour()
	switch {
	case h < 5:
		return "夜深了"
	case h < 9:
		return "早上好"
	case h < 12:
		return "上午好"
	case h < 14:
		return "中午好"
	case h < 18:
		return "下午好"
	case h < 23:
		return "晚上好"
	default:
		return "夜深了"
	}
}

// Workbench 聚合工作台数据。msLibraries 为媒体服务器库枚举回调（nil=跳过媒体库态势）。
// 纯 DB 部分单函数聚合；媒体库态势经回调走各服务器 API（默认仅默认服务器，避免全量扫库）。
func (c *DashboardChain) Workbench(ctx context.Context, msLibraries func(ctx context.Context) WorkbenchMediaLibrary) (*Workbench, error) {
	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	out := &Workbench{GeneratedAt: now}
	db := c.db

	// ---- 站点域 ----
	var sites []models.Site
	if err := db.Find(&sites).Error; err != nil {
		slog.Warn("workbench: list sites failed", "err", err)
	}
	active := 0
	for _, s := range sites {
		if s.IsActive {
			active++
		}
	}
	// 无数据站点（同 Overview 判定：启用 + 非论坛 + 无统计）——用每站最新统计判断
	noData := 0
	if len(sites) > 0 {
		var latest []models.SiteStat
		db.Where("id IN (SELECT MAX(id) FROM site_stats GROUP BY site_id)").Find(&latest)
		statMap := make(map[uint]models.SiteStat, len(latest))
		for _, st := range latest {
			statMap[st.SiteID] = st
		}
		for _, s := range sites {
			if !s.IsActive || isForumPlatform(s.Platform) {
				continue
			}
			if st, ok := statMap[s.ID]; ok && (st.Uploaded > 0 || st.Downloaded > 0 || st.Seeding > 0) {
				continue
			}
			noData++
			out.Todos = append(out.Todos, WorkbenchTodo{
				Kind: "no_data_site", Title: s.Name, Detail: "无采集数据（Cookie 失效或未采集）",
				Link: "/sites", Level: "warning",
			})
		}
	}
	out.Summary.SitesActive = active
	out.Summary.SitesNoData = noData

	// ---- 订阅域 ----
	var subNew, subHits int64
	db.Model(&models.Subscription{}).Where("created_at >= ?", dayStart).Count(&subNew)
	db.Model(&models.SubscriptionHit{}).Where("created_at >= ?", dayStart).Count(&subHits)
	out.Summary.SubscriptionsNew = int(subNew)
	out.Summary.SubscriptionsHits = int(subHits)

	// ---- 整理域（今日成功/失败）----
	var orgDone, orgFailed int64
	db.Model(&models.TransferHistory{}).Where("created_at >= ? AND status = ?", dayStart, "success").Count(&orgDone)
	db.Model(&models.TransferHistory{}).Where("created_at >= ? AND status = ?", dayStart, "failed").Count(&orgFailed)
	out.Summary.DownloadsDone = int(orgDone)
	out.Summary.OrganizeFailed = int(orgFailed)
	if orgFailed > 0 {
		out.Todos = append(out.Todos, WorkbenchTodo{
			Kind: "transfer_failed", Title: "整理失败待处理",
			Detail: "今日有 N 项整理失败", Link: "/transfer", Level: "danger",
		})
		out.Todos[len(out.Todos)-1].Detail = replaceCount(out.Todos[len(out.Todos)-1].Detail, int(orgFailed))
	}

	// ---- 云端任务（今日成功 + 当前排队；动态进度）----
	var cloudDone, cloudPending int64
	db.Model(&models.CloudTask{}).Where("updated_at >= ? AND status = ?", dayStart, "success").Count(&cloudDone)
	db.Model(&models.CloudTask{}).Where("status IN ?", []string{"pending", "running"}).Count(&cloudPending)
	out.Summary.CloudTasksDone = int(cloudDone)
	out.Summary.CloudTasksPending = int(cloudPending)

	// ---- 今日上下行增量（对比昨日同期快照的跨站聚合差值；无昨日数据=当日总量）----
	var todayAgg, ydayAgg struct{ Up, Down int64 }
	today := now.Format("2006-01-02")
	yday := now.AddDate(0, 0, -1).Format("2006-01-02")
	// 各站最新快照聚合为「当前」；按天聚合两张表相减近似增量
	type dayAgg struct {
		Day  string
		Up   int64
		Down int64
	}
	var rows []dayAgg
	db.Raw("SELECT substr(snap_at,1,10) AS day, SUM(uploaded) AS up, SUM(downloaded) AS down FROM site_stats WHERE snap_at >= ? GROUP BY substr(snap_at,1,10) ORDER BY day DESC LIMIT 2", yday+" 00:00:00").Scan(&rows)
	for _, r := range rows {
		if r.Day == today {
			todayAgg.Up, todayAgg.Down = r.Up, r.Down
		} else {
			ydayAgg.Up, ydayAgg.Down = r.Up, r.Down
		}
	}
	out.Summary.UploadedDelta = todayAgg.Up - ydayAgg.Up
	out.Summary.DownloadedDelta = todayAgg.Down - ydayAgg.Down

	// ---- 今日 STRM 生成 ----
	var strmCnt int64
	db.Model(&models.StrmRecord{}).Where("created_at >= ?", dayStart).Count(&strmCnt)
	out.Summary.StrmGenerated = int(strmCnt)

	// ---- 今日播放（webhook 事件通知近似）----
	var played int64
	db.Model(&models.Notice{}).
		Where("created_at >= ? AND source = ?", dayStart, "media_play").Count(&played)
	out.Summary.PlayedToday = int(played)

	// ---- 今日动态：notice 最近 20 条 + 进行中的云端任务（带进度）----
	var notices []models.Notice
	db.Where("created_at >= ?", dayStart).Order("created_at DESC").Limit(20).Find(&notices)
	for _, n := range notices {
		out.Events = append(out.Events, WorkbenchEvent{
			Source: n.Source, Title: n.Title, Level: n.Level, CreatedAt: n.CreatedAt,
		})
	}
	if cloudPending > 0 {
		var running []models.CloudTask
		db.Where("status IN ?", []string{"pending", "running"}).Order("updated_at DESC").Limit(10).Find(&running)
		for _, t := range running {
			p := t.Progress
			out.Events = append(out.Events, WorkbenchEvent{
				Source: "cloud_task", Title: t.Kind + " · " + t.CloudPath,
				Level:     map[bool]string{true: "info", false: "warning"}[t.Status == "running"],
				CreatedAt: t.UpdatedAt, Progress: &p,
			})
		}
	}

	// ---- 媒体库态势（回调注入；nil/失败=未配置）----
	if msLibraries != nil {
		out.MediaLibrary = msLibraries(ctx)
	} else {
		out.MediaLibrary = WorkbenchMediaLibrary{Configured: false, Servers: []MediaLibraryStat{}}
	}
	if out.MediaLibrary.Servers == nil {
		out.MediaLibrary.Servers = []MediaLibraryStat{}
	}

	// ---- 流转节点 ----
	mediaMovies, mediaSeries := 0, 0
	for _, s := range out.MediaLibrary.Servers {
		mediaMovies += s.Movies
		mediaSeries += s.Series
	}
	out.Nodes = []WorkbenchNode{
		{Key: "site", Label: "站点", Count: active, Hint: "启用站点"},
		{Key: "subscribe", Label: "订阅", Count: int(subNew) + int(subHits), Hint: "今日新增+命中"},
		{Key: "download", Label: "下载", Count: int(orgDone), Hint: "今日整理成功"},
		{Key: "organize", Label: "整理", Count: int(orgDone) + int(orgFailed), Hint: "今日整理总量"},
		{Key: "library", Label: "媒体库", Count: mediaMovies + mediaSeries, Hint: "电影+剧集"},
		{Key: "watch", Label: "观影", Count: int(played), Hint: "今日播放"},
	}

	// ---- 体检分（轻量派生：整理失败/无数据站点/云端积压扣分；满检走 /system/doctor）----
	score := 100
	if orgFailed > 0 {
		score -= min(30, 5*int(orgFailed))
	}
	if noData > 0 {
		score -= min(30, 10*noData)
	}
	if cloudPending > 10 {
		score -= 15
	}
	if len(sites) == 0 {
		score -= 20
	}
	if score < 0 {
		score = 0
	}
	out.HealthScore = score

	if out.Todos == nil {
		out.Todos = []WorkbenchTodo{}
	}
	if out.Events == nil {
		out.Events = []WorkbenchEvent{}
	}
	return out, nil
}

// replaceCount 把 Detail 里的 "N" 占位替换为实际数。
func replaceCount(s string, n int) string {
	return strings.Replace(s, "N", strconv.Itoa(n), 1)
}

// MediaLibraryOverview 枚举全部媒体服务器库计数（DashboardChain.Workbench 的
// msLibraries 回调实现；单服务器失败跳过不阻断，Emby/JF 按 LibraryItems.Type 聚合）。
func (c *MediaServerChain) MediaLibraryOverview(ctx context.Context) WorkbenchMediaLibrary {
	out := WorkbenchMediaLibrary{Configured: false, Servers: []MediaLibraryStat{}}
	servers, err := c.List()
	if err != nil {
		return out
	}
	if len(servers) == 0 {
		return out
	}
	out.Configured = true
	for _, s := range servers {
		stat := MediaLibraryStat{ServerID: s.ID, ServerName: s.Name, Type: s.Type, SyncedAt: time.Now()}
		cli, _, err := c.client(s.ID)
		if err != nil {
			slog.Debug("workbench: media server client failed", "server", s.Name, "err", err)
			out.Servers = append(out.Servers, stat)
			continue
		}
		libs, err := cli.Libraries(ctx)
		if err != nil {
			slog.Debug("workbench: media server libraries failed", "server", s.Name, "err", err)
			out.Servers = append(out.Servers, stat)
			continue
		}
		stat.LibraryCnt = len(libs)
		for _, lib := range libs {
			items, err := cli.LibraryItems(ctx, lib.ID)
			if err != nil {
				continue
			}
			for _, it := range items {
				if it.Type == "movie" {
					stat.Movies++
				} else if it.Type == "series" {
					stat.Series++
				}
			}
		}
		out.Servers = append(out.Servers, stat)
	}
	return out
}
