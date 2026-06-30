// Package assessment 实现「站点考核」抓取：各平台考核任务（新手考核/长期考核/保种/魔力任务）
// 的真实进度。per-platform 适配器 + Registry 分发（MTeam JSON / NexusPHP HTML 跨行聚合 / TNode 不支持）。
// 独立子包，避免膨胀 schemas；由 chain.AssessmentChain 调用。
package assessment

import (
	"context"
	"sync"
	"time"
)

// Status 考核任务原始状态。
type Status string

const (
	StatusActive    Status = "ACTIVE"
	StatusCompleted Status = "COMPLETED"
	StatusFailed    Status = "FAILED"
	StatusExempt    Status = "EXEMPT"
	StatusUnknown   Status = "UNKNOWN"
)

// Metric 是一项考核指标。
type Metric struct {
	Label       string  `json:"label"`
	Required    float64 `json:"required"`
	Current     float64 `json:"current"`
	Unit        string  `json:"unit"`
	IsCompleted bool    `json:"is_completed"`
}

// Task 是一个考核任务。
type Task struct {
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	StartTime   int64     `json:"start_time"` // epoch ms（0=未知）
	EndTime     int64     `json:"end_time"`   // epoch ms（0=未知）
	Metrics     []Metric  `json:"metrics"`
	Status      Status    `json:"status"`
	RawJSON     string    `json:"raw_json,omitempty"`
	FetchedAt   time.Time `json:"fetched_at"`
}

// Def 是考核所需的最小站点定义（从 site.SiteMetadata 投影，避免 assessment 反向依赖 site 包）。
type Def struct {
	Schema   string
	UserInfo struct {
		DetailPath string // NexusPHP userdetails.php
	}
}

// Adapter 是 per-platform 考核适配器契约。
// token：MTeam 为 x-api-key；NexusPHP 为 cookie。返回空表 ≠ 错误（无活跃任务）。
type Adapter interface {
	Platform() string
	Supported() bool
	FetchAssessments(ctx context.Context, hc HTTPDoer, def Def, baseURL, token string) ([]Task, error)
}

// HTTPDoer 是考核适配器需要的 HTTP 子接口（httpclient.Client 已实现）。
type HTTPDoer interface {
	Get(ctx context.Context, rawURL, cookieStr string, ssrf bool) (*Response, error)
	Do(ctx context.Context, method, rawURL string, body []byte, headers map[string]string, cookie string, ssrf bool) (*Response, error)
}

// Response 是 HTTP 响应的最小子集（由调用方从 httpclient.Response 适配构造）。
type Response struct {
	StatusCode int
	Body       []byte
	FinalURL   string
}

// Registry 按 platform（小写）分发考核适配器；未注册返回 NoOp。
type Registry struct {
	mu       sync.RWMutex
	adapters map[string]Adapter
}

// NewRegistry 构造空 Registry。
func NewRegistry() *Registry { return &Registry{adapters: map[string]Adapter{}} }

// Register 注册一个适配器（按 Platform() 小写键）。
func (r *Registry) Register(a Adapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters[a.Platform()] = a
}

// Get 取适配器；未注册返回 NoOp（Supported=false）。
func (r *Registry) Get(platform string) Adapter {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if a, ok := r.adapters[platform]; ok {
		return a
	}
	return NoOpAdapter{platform: platform}
}

// NoOpAdapter 未支持平台的占位。
type NoOpAdapter struct{ platform string }

func (n NoOpAdapter) Platform() string { return n.platform }
func (n NoOpAdapter) Supported() bool  { return false }
func (n NoOpAdapter) FetchAssessments(context.Context, HTTPDoer, Def, string, string) ([]Task, error) {
	return nil, nil
}
