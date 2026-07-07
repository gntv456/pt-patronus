package site

import (
	"fmt"
	"io/fs"
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
