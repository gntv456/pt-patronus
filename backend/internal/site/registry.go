package site

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

// Registry 管理站点定义（definitions/*.yaml）与平台适配器（Schema）的映射。
type Registry struct {
	defs     map[string]*SiteMetadata
	adapters map[string]Adapter
}

// NewRegistry 构造空注册表。
func NewRegistry() *Registry {
	return &Registry{
		defs:     map[string]*SiteMetadata{},
		adapters: map[string]Adapter{},
	}
}

// RegisterAdapter 注册一个平台适配器。
func (r *Registry) RegisterAdapter(a Adapter) {
	if a == nil {
		return
	}
	r.adapters[a.Schema()] = a
}

// LoadDefinitions 从目录加载所有 *.yaml / *.yml 站点定义。
func (r *Registry) LoadDefinitions(dir string) error {
	return r.loadDefinitionsFS(os.DirFS(dir), ".")
}

// LoadDefinitionsFS 从文件系统加载所有 *.yaml / *.yml 站点定义。
func (r *Registry) LoadDefinitionsFS(fsys fs.FS, dir string) error {
	if dir == "" {
		dir = "."
	}
	return r.loadDefinitionsFS(fsys, dir)
}

func (r *Registry) loadDefinitionsFS(fsys fs.FS, dir string) error {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return fmt.Errorf("read definitions dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		file := name
		if dir != "." {
			file = path.Join(dir, name)
		}
		data, err := fs.ReadFile(fsys, file)
		if err != nil {
			return err
		}
		var def SiteMetadata
		if err := yaml.Unmarshal(data, &def); err != nil {
			return fmt.Errorf("parse %s: %w", file, err)
		}
		if def.ID == "" {
			continue
		}
		r.defs[def.ID] = &def
	}
	return nil
}

// AddDefinition 直接注入一个定义（便于测试）。
func (r *Registry) AddDefinition(def *SiteMetadata) {
	if def != nil && def.ID != "" {
		r.defs[def.ID] = def
	}
}

// Definition 取站点定义。
func (r *Registry) Definition(id string) (*SiteMetadata, bool) {
	d, ok := r.defs[id]
	return d, ok
}

// AdapterFor 取某平台的适配器。
func (r *Registry) AdapterFor(schema string) (Adapter, bool) {
	a, ok := r.adapters[schema]
	return a, ok
}

// Definitions 返回全部定义。
func (r *Registry) Definitions() []*SiteMetadata {
	out := make([]*SiteMetadata, 0, len(r.defs))
	for _, d := range r.defs {
		out = append(out, d)
	}
	return out
}

// ActiveDefinitions 返回未标记 Dead 的定义。死站（Dead=true）保留信息可展示，
// 但不参与活跃抓取（搜索/用户资料/HR 等）；chain 层用此方法过滤。死站过滤的完整闭环随 M2 落地。
func (r *Registry) ActiveDefinitions() []*SiteMetadata {
	out := make([]*SiteMetadata, 0, len(r.defs))
	for _, d := range r.defs {
		if !d.Dead {
			out = append(out, d)
		}
	}
	return out
}

// MatchByURL 按主机名匹配站点定义：给定站点 baseURL，返回 definitions 中
// URLs/LegacyURLs 命中同 host 的定义 ID。用于 .ptd 等备份恢复时自动挂「正确定义」
// （而非全部退化为默认 nexusphp）—— tnode/特殊 schema 站（如朱雀）走错适配器会彻底抓空，
// nexusphp 站用通用 def 也常抓不全（选择器/字段差异）。未匹配返回 ("", false)，
// 调用方回退到 schema 推断（如 nexusphp 通用）。
func (r *Registry) MatchByURL(rawURL string) (string, bool) {
	host := matchHost(rawURL)
	if host == "" {
		return "", false
	}
	for _, def := range r.defs {
		for _, u := range def.URLs {
			if matchHost(u) == host {
				return def.ID, true
			}
		}
		for _, u := range def.LegacyURLs {
			if matchHost(u) == host {
				return def.ID, true
			}
		}
	}
	return "", false
}

// matchHost 提取 URL 的主机名（小写）；非法/无 host 返回空串。
func matchHost(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}
