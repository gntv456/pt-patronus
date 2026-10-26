package site

import (
	"fmt"
	"io/fs"
	"log/slog"
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
	diag     definitionDiagnostics
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
	seen := map[string]string{}
	for _, e := range entries {
		if e.IsDir() {
			// 递归没做：子目录里的 *.yaml 一个都不读。用户把自定义定义放进
			// definitions/extra/ 时的表现是「程序当它不存在」，必须出声指出被跳过的目录。
			skipped := path.Join(dir, e.Name())
			slog.Warn("site definitions subdirectory skipped; only top-level *.yaml is loaded", "dir", skipped)
			r.recordDiag(DefinitionDiagnostic{Kind: DiagSkippedSubdir, File: skipped,
				Detail: "子目录不会被读取，里面的站点定义等于没放"})
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
			// 读不出/解析失败过去 return err，app_setup 于是把整个外部目录
			// 退回内置：其余自定义站点跟着没，账面零记录、界面显示一切正常。
			slog.Warn("site definition unreadable; this one skipped",
				"file", file, "err", err)
			r.recordDiag(DefinitionDiagnostic{Kind: DiagLoadFailed, File: file,
				Detail: "读不出来（仅这一份没生效）：" + err.Error()})
			continue
		}
		var def SiteMetadata
		if err := yaml.Unmarshal(data, &def); err != nil {
			slog.Warn("site definition failed to parse; this one skipped",
				"file", file, "err", err)
			r.recordDiag(DefinitionDiagnostic{Kind: DiagLoadFailed, File: file,
				Detail: "解析失败（仅这一份没生效）：" + err.Error()})
			continue
		}
		// 未知键不报错也不出声（整块配置静默蒸发），这里只逐条 Warn + 记账，不改加载判定。
		r.warnUnknownDefinitionKeys(file, data)
		if def.ID == "" {
			continue
		}
		// 只盯「同一目录里两个文件同 id」：实测是后加载者静默胜出，前者的整份配置蒸发。
		// 跨调用（外部目录顶掉内置同名站点）是自定义覆盖的正常用法，不在这里出声。
		if prevFile, dup := seen[def.ID]; dup {
			slog.Warn("site definition id duplicated within this directory; later file overrides earlier one",
				"id", def.ID, "kept", file, "ignored", prevFile)
			r.recordDiag(DefinitionDiagnostic{Kind: DiagDuplicateID, Site: def.ID, File: file,
				Detail: "与 " + prevFile + " 用了同一个站点 id，先加载那份整个不生效"})
		}
		seen[def.ID] = file
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
