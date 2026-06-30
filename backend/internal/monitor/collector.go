package monitor

import (
	"context"
	"fmt"
	"sync"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// Collector 是一种 (type,protocol) 组合的采集实现。一个实例服务该组合下所有 target；
// target 间差异由 MonitorTarget 字段驱动（与 site.Adapter ↔ SiteMetadata 同构）。
// 不持有凭据明文：由调用方用 Oper 解密后通过 creds 参数注入。
type Collector interface {
	// String 返回采集器标识，用于日志。
	String() string
	// Collect 执行一次完整采集，返回指标快照。
	Collect(ctx context.Context, t *models.MonitorTarget, creds Credentials) (Metrics, error)
	// Probe 探活：仅判定设备在线可达，不做重采集。
	Probe(ctx context.Context, t *models.MonitorTarget, creds Credentials) (bool, error)
}

// Registry 按 (type,protocol) 注册采集器（参考 site.Registry 的 map 注册模式）。
type Registry struct {
	mu         sync.RWMutex
	collectors map[string]Collector
}

// NewRegistry 构造空注册表。
func NewRegistry() *Registry {
	return &Registry{collectors: make(map[string]Collector)}
}

func registryKey(typeStr, protocol string) string { return typeStr + "|" + protocol }

// Register 注册一个采集器到 (typeStr,protocol)。同 key 覆盖。
func (r *Registry) Register(typeStr, protocol string, c Collector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.collectors[registryKey(typeStr, protocol)] = c
}

// CollectorFor 查找采集器。
func (r *Registry) CollectorFor(typeStr, protocol string) (Collector, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.collectors[registryKey(typeStr, protocol)]
	return c, ok
}

// ResolveProtocol 当 target.Protocol 为空时按 type/host 推断默认协议。
func ResolveProtocol(t *models.MonitorTarget) string {
	if t.Protocol != "" {
		return t.Protocol
	}
	switch t.Type {
	case "nas":
		return "api"
	case "camera":
		return "onvif"
	default: // host：本机无 Host 地址 → local；否则 ssh
		if t.Host == "" {
			return "local"
		}
		return "ssh"
	}
}

// ErrUnsupported 表示无采集器匹配该 (type,protocol)。
var ErrUnsupported = fmt.Errorf("no collector registered for this type/protocol")
