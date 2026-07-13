// Package dirwatch 实时监视本地目录，发现媒体文件即喂给整理流水线（补 DoneHook + cron
// 覆盖不到的手动投放 / 非托管来源）。fsnotify 事件驱动 + debounce；compatibility 模式轮询兜底。
package dirwatch

import "encoding/json"

// KVStore 抽象 oper.SystemConfigOper（Get/Set by key），与 aiagent/llmgateway 同构。
type KVStore interface {
	Get(key string) (string, error)
	Set(key, value string) error
}

// Config 目录监视配置（存 SystemConfig `dirwatch.config`）。
type Config struct {
	Enabled         bool         `json:"enabled"`
	Dirs            []WatchedDir `json:"dirs"`
	MonitorMode     string       `json:"monitor_mode"` // auto(fsnotify) | compatibility(轮询)
	DebounceMs      int          `json:"debounce_ms"`
	PollIntervalSec int          `json:"poll_interval_sec"`
}

// WatchedDir 单个被监视目录。
type WatchedDir struct {
	Path        string `json:"path"`
	LibraryPath string `json:"library_path"`
}

// ConfigKey SystemConfig 存储键。
const ConfigKey = "dirwatch.config"

// DefaultConfig 默认配置。
func DefaultConfig() Config {
	return Config{Enabled: false, MonitorMode: "auto", DebounceMs: 2000, Dirs: []WatchedDir{}}
}

// Load 从 KV 读配置；缺失/损坏回退默认。
func Load(kv KVStore) Config {
	def := DefaultConfig()
	if kv == nil {
		return def
	}
	raw, err := kv.Get(ConfigKey)
	if err != nil || raw == "" {
		return def
	}
	var c Config
	if json.Unmarshal([]byte(raw), &c) != nil {
		return def
	}
	if c.DebounceMs <= 0 {
		c.DebounceMs = def.DebounceMs
	}
	if c.MonitorMode == "" {
		c.MonitorMode = "auto"
	}
	if c.Dirs == nil {
		c.Dirs = []WatchedDir{}
	}
	return c
}

// Save 写配置到 KV。
func Save(kv KVStore, c Config) error {
	if kv == nil {
		return nil
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return kv.Set(ConfigKey, string(b))
}

// Redacted 返回配置（无敏感字段，直接对外）。
func Redacted(c Config) map[string]any {
	return map[string]any{
		"enabled":           c.Enabled,
		"dirs":              c.Dirs,
		"monitor_mode":      c.MonitorMode,
		"debounce_ms":       c.DebounceMs,
		"poll_interval_sec": c.PollIntervalSec,
	}
}
