package site

import (
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// BadFilterPattern 是一条「永远不可能生效」的抓取规则：regexp 族的 pattern 编译不过。
// Site/Path 指到具体站点与字段，Pattern/Err 指到具体写法与原因。
type BadFilterPattern struct {
	Site    string
	Path    string
	Name    string
	Pattern string
	Err     string
}

// regexpFamilyFilterNames 是走 regexp.Compile 的 filter 名。新增 filter 若也用正则，
// 必须同时进这张表，否则启动体检会漏检它（filters.go 里每个 Compile 调用点都要有对应名字）。
var regexpFamilyFilterNames = map[string]bool{"regexp": true, "sum_matches": true}

// LintFilterPatterns 遍历一个站点定义，找出所有编不出来的 regexp 族 pattern。
//
// 为什么放启动期：规则写错（如用 RE2 不支持的先行断言 (?=…)）过去只有等到真的抓那个站
// 才会露一次头，而那时日志里只有一个光秃秃的 pattern，看不出是哪个站哪个字段；
// 且「抓到第一个数量当本字段的值」是错值而不是空值，界面上完全看不出异常。
// 启动期扫一遍（定义数量有界，不刷屏）就能把 site + 字段路径 + 原因一起报出来。
func LintFilterPatterns(def *SiteMetadata) []BadFilterPattern {
	if def == nil {
		return nil
	}
	var out []BadFilterPattern
	walkFilters(reflect.ValueOf(def), "", map[uintptr]bool{}, func(path string, f Filter) {
		if !regexpFamilyFilterNames[f.Name] || f.Args == "" {
			return
		}
		pat := f.Args
		if i := strings.Index(pat, ":::"); i >= 0 {
			if _, err := strconv.Atoi(pat[i+3:]); err == nil {
				pat = pat[:i]
			}
		}
		if _, err := regexp.Compile(pat); err != nil {
			out = append(out, BadFilterPattern{
				Site: def.ID, Path: path, Name: f.Name, Pattern: pat, Err: err.Error(),
			})
		}
	})
	return out
}

// LintFilterPatterns 体检注册表里所有已加载的站点定义（内置 + 外部目录）。
func (r *Registry) LintFilterPatterns() []BadFilterPattern {
	var out []BadFilterPattern
	for _, def := range r.Definitions() {
		out = append(out, LintFilterPatterns(def)...)
	}
	return out
}

// RecordFilterPatternDiagnostics 体检并把结果记进诊断账面，返回清单供调用方打日志。
// 分两步是有原因的：日志在 app 层（带启动上下文），账面在 site 层（要被 HTTP 端点读）。
func (r *Registry) RecordFilterPatternDiagnostics() []BadFilterPattern {
	bad := r.LintFilterPatterns()
	for _, b := range bad {
		r.recordDiag(DefinitionDiagnostic{Kind: DiagBadRegexp, Site: b.Site,
			Detail: "抓取规则的正则编不出来，这一条永远不会取到值：" + b.Path + " " + b.Pattern + "（" + b.Err + "）"})
	}
	return bad
}

// walkFilters 递归找出所有 Filter 值（结构体里 Filters []Filter 只挂在 ElementQuery 上，
// 但 ElementQuery 嵌在搜索/用户资料/详情等多层里，逐层手写枚举必然漂移）。
func walkFilters(v reflect.Value, path string, seen map[uintptr]bool, fn func(string, Filter)) {
	switch v.Kind() {
	case reflect.Invalid:
	case reflect.Pointer:
		if v.IsNil() {
			return
		}
		p := v.Pointer()
		if seen[p] {
			return
		}
		seen[p] = true
		walkFilters(v.Elem(), path, seen, fn)
	case reflect.Struct:
		if v.Type() == reflect.TypeOf(Filter{}) {
			fn(path, v.Interface().(Filter))
			return
		}
		for i := 0; i < v.NumField(); i++ {
			walkFilters(v.Field(i), path+"."+v.Type().Field(i).Name, seen, fn)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			walkFilters(v.Index(i), path+"[]", seen, fn)
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			walkFilters(v.MapIndex(k), path+"{}", seen, fn)
		}
	}
}
