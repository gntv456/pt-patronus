package filter

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/hxpt/ptpatronus/internal/db/oper"
)

// reCache 缓存编译后的正则（含 (?i) IGNORECASE）。热路径上每个 pattern 仅编译一次。
// 对齐 MP-2 _compile_ignorecase 的 lru_cache(1024)。
var reCache sync.Map // pattern(string) → *regexp.Regexp

// compilePattern 编译 pattern 为 IGNORECASE 正则；失败返回错误（坏规则会在 Normalize 阶段被拒）。
func compilePattern(pattern string) (*regexp.Regexp, error) {
	if v, ok := reCache.Load(pattern); ok {
		return v.(*regexp.Regexp), nil
	}
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil, err
	}
	reCache.Store(pattern, re)
	return re, nil
}

// reSearch 等价 Python re.search（忽略大小写，找任意位置匹配）。编译失败视为不匹配。
func reSearch(pattern, content string) bool {
	re, err := compilePattern(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(content)
}

// Store 封装 SystemConfigOper，管理 5 个 filter 相关 KV 键的 CRUD。
// 规则/组以 JSON 存 SystemConfig 表（零 schema 迁移）；自定义规则按 id 覆盖内置。
type Store struct {
	op *oper.SystemConfigOper
}

// NewStore 构造。
func NewStore(op *oper.SystemConfigOper) *Store { return &Store{op: op} }

// ---------- 自定义规则 ----------

// GetCustomRules 读取自定义规则（按入库顺序）。
func (s *Store) GetCustomRules() ([]Rule, error) {
	v, err := s.op.Get(KeyCustomFilterRules)
	if err != nil {
		return nil, err
	}
	if v == "" {
		return nil, nil
	}
	var rs []Rule
	if err := json.Unmarshal([]byte(v), &rs); err != nil {
		return nil, fmt.Errorf("解析自定义规则失败：%w", err)
	}
	return rs, nil
}

func (s *Store) saveCustomRules(rs []Rule) error {
	b, err := json.Marshal(rs)
	if err != nil {
		return err
	}
	return s.op.Set(KeyCustomFilterRules, string(b))
}

// AddCustomRule 新增（id 重复则报错）。自定义 id 可与内置相同（覆盖语义），但自定义间不可重复。
func (s *Store) AddCustomRule(in CustomRuleInput) (Rule, error) {
	r, err := NormalizeCustomRule(in)
	if err != nil {
		return Rule{}, err
	}
	rs, err := s.GetCustomRules()
	if err != nil {
		return Rule{}, err
	}
	for _, e := range rs {
		if e.ID == r.ID {
			return Rule{}, fmt.Errorf("规则 id %q 已存在", r.ID)
		}
	}
	rs = append(rs, r)
	if err := s.saveCustomRules(rs); err != nil {
		return Rule{}, err
	}
	return r, nil
}

// UpdateCustomRule 按 oldID 定位并替换（允许改 id；新 id 不可与其他自定义冲突）。
func (s *Store) UpdateCustomRule(oldID string, in CustomRuleInput) (Rule, error) {
	oldID = strings.TrimSpace(oldID)
	r, err := NormalizeCustomRule(in)
	if err != nil {
		return Rule{}, err
	}
	rs, err := s.GetCustomRules()
	if err != nil {
		return Rule{}, err
	}
	idx := -1
	for i, e := range rs {
		if e.ID == oldID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return Rule{}, fmt.Errorf("规则 id %q 不存在", oldID)
	}
	for i, e := range rs {
		if i == idx {
			continue
		}
		if e.ID == r.ID {
			return Rule{}, fmt.Errorf("规则 id %q 已存在", r.ID)
		}
	}
	rs[idx] = r
	if err := s.saveCustomRules(rs); err != nil {
		return Rule{}, err
	}
	return r, nil
}

// DeleteCustomRule 删除；若被任一规则组引用则拒绝（对齐 MP-2 delete_custom_filter_rule）。
func (s *Store) DeleteCustomRule(id string) error {
	id = strings.TrimSpace(id)
	groups, err := s.GetGroups()
	if err != nil {
		return err
	}
	for _, g := range groups {
		for _, tok := range extractTokens(g.RuleString) {
			if tok == id {
				return fmt.Errorf("规则 %q 被规则组 %q 引用，禁止删除", id, g.Name)
			}
		}
	}
	rs, err := s.GetCustomRules()
	if err != nil {
		return err
	}
	out := rs[:0]
	removed := false
	for _, e := range rs {
		if e.ID == id {
			removed = true
			continue
		}
		out = append(out, e)
	}
	if !removed {
		return fmt.Errorf("规则 id %q 不存在", id)
	}
	return s.saveCustomRules(out)
}

// ---------- 规则组 ----------

// GetGroups 读取规则组定义（按入库顺序）。
func (s *Store) GetGroups() ([]RuleGroup, error) {
	v, err := s.op.Get(KeyUserFilterRuleGroups)
	if err != nil {
		return nil, err
	}
	if v == "" {
		return nil, nil
	}
	var gs []RuleGroup
	if err := json.Unmarshal([]byte(v), &gs); err != nil {
		return nil, fmt.Errorf("解析规则组失败：%w", err)
	}
	return gs, nil
}

func (s *Store) saveGroups(gs []RuleGroup) error {
	b, err := json.Marshal(gs)
	if err != nil {
		return err
	}
	return s.op.Set(KeyUserFilterRuleGroups, string(b))
}

// AddGroup 新增规则组（校验语法 + 引用 id 存在 + name 唯一）。
func (s *Store) AddGroup(in RuleGroupInput) (RuleGroup, error) {
	g, err := NormalizeRuleGroup(in)
	if err != nil {
		return RuleGroup{}, err
	}
	known, err := s.KnownIDs()
	if err != nil {
		return RuleGroup{}, err
	}
	if err := ValidateRuleString(g.RuleString, known); err != nil {
		return RuleGroup{}, err
	}
	gs, err := s.GetGroups()
	if err != nil {
		return RuleGroup{}, err
	}
	for _, e := range gs {
		if e.Name == g.Name {
			return RuleGroup{}, fmt.Errorf("规则组 %q 已存在", g.Name)
		}
	}
	gs = append(gs, g)
	if err := s.saveGroups(gs); err != nil {
		return RuleGroup{}, err
	}
	return g, nil
}

// UpdateGroup 按 oldName 定位并替换。
func (s *Store) UpdateGroup(oldName string, in RuleGroupInput) (RuleGroup, error) {
	oldName = strings.TrimSpace(oldName)
	g, err := NormalizeRuleGroup(in)
	if err != nil {
		return RuleGroup{}, err
	}
	known, err := s.KnownIDs()
	if err != nil {
		return RuleGroup{}, err
	}
	if err := ValidateRuleString(g.RuleString, known); err != nil {
		return RuleGroup{}, err
	}
	gs, err := s.GetGroups()
	if err != nil {
		return RuleGroup{}, err
	}
	idx := -1
	for i, e := range gs {
		if e.Name == oldName {
			idx = i
			break
		}
	}
	if idx < 0 {
		return RuleGroup{}, fmt.Errorf("规则组 %q 不存在", oldName)
	}
	for i, e := range gs {
		if i == idx {
			continue
		}
		if e.Name == g.Name {
			return RuleGroup{}, fmt.Errorf("规则组 %q 已存在", g.Name)
		}
	}
	gs[idx] = g
	if err := s.saveGroups(gs); err != nil {
		return RuleGroup{}, err
	}
	// 改名时级联更新全局绑定引用。
	if oldName != g.Name {
		s.cleanupGroupFromBindings(oldName) // 旧名从绑定移除（新名由用户重新绑定）
	}
	return g, nil
}

// DeleteGroup 删除规则组，并清理全局绑定中的悬空引用。
func (s *Store) DeleteGroup(name string) error {
	name = strings.TrimSpace(name)
	gs, err := s.GetGroups()
	if err != nil {
		return err
	}
	out := gs[:0]
	removed := false
	for _, e := range gs {
		if e.Name == name {
			removed = true
			continue
		}
		out = append(out, e)
	}
	if !removed {
		return fmt.Errorf("规则组 %q 不存在", name)
	}
	if err := s.saveGroups(out); err != nil {
		return err
	}
	s.cleanupGroupFromBindings(name)
	return nil
}

// cleanupGroupFromBindings 从三个全局绑定 key 中移除指定组名。
func (s *Store) cleanupGroupFromBindings(name string) {
	for _, key := range []string{KeySearchFilterRuleGroups, KeySubscribeFilterRuleGroups, KeyBestVersionFilterRuleGroups} {
		names, err := s.GetBindings(key)
		if err != nil {
			continue
		}
		changed := false
		out := names[:0]
		for _, n := range names {
			if n == name {
				changed = true
				continue
			}
			out = append(out, n)
		}
		if changed {
			_ = s.SetBindings(key, out)
		}
	}
}

// ---------- 全局绑定 ----------

// GetBindings 读取某全局绑定（组名列表）。
func (s *Store) GetBindings(key string) ([]string, error) {
	v, err := s.op.Get(key)
	if err != nil {
		return nil, err
	}
	if v == "" {
		return nil, nil
	}
	var names []string
	if err := json.Unmarshal([]byte(v), &names); err != nil {
		return nil, fmt.Errorf("解析绑定 %s 失败：%w", key, err)
	}
	return names, nil
}

// SetBindings 写某全局绑定。
func (s *Store) SetBindings(key string, names []string) error {
	b, err := json.Marshal(names)
	if err != nil {
		return err
	}
	return s.op.Set(key, string(b))
}

// ---------- 规则集与组解析 ----------

// RuleSet 返回运行期规则集 = 深拷贝内置 + 自定义覆盖（对齐 MP-2 init_module）。
func (s *Store) RuleSet() (map[string]Rule, error) {
	out := make(map[string]Rule, len(BuiltinRuleSet))
	for _, id := range builtinOrder {
		out[id] = cloneRule(BuiltinRuleSet[id])
	}
	customs, err := s.GetCustomRules()
	if err != nil {
		return nil, err
	}
	for _, c := range customs {
		out[c.ID] = c // 自定义覆盖内置
	}
	return out, nil
}

// KnownIDs 返回内置 ∪ 自定义的 id 集合（供 ValidateRuleString）。
func (s *Store) KnownIDs() (map[string]bool, error) {
	rs, err := s.RuleSet()
	if err != nil {
		return nil, err
	}
	m := make(map[string]bool, len(rs))
	for id := range rs {
		m[id] = true
	}
	return m, nil
}

// ResolveGroups 按 media_type/category 筛选适用组；names 非空时再按名过滤。
// 对齐 MP-2 RuleHelper.get_rule_group_by_media：
//   - 组 media_type 空 → 适用（全局）
//   - 组 media_type 命中媒体类型且 category 空 → 适用
//   - 组 category 命中媒体 category → 适用（最具体）
func (s *Store) ResolveGroups(media *MediaInfo, names []string) ([]RuleGroup, error) {
	all, err := s.GetGroups()
	if err != nil {
		return nil, err
	}
	var nameSet map[string]bool
	if len(names) > 0 {
		nameSet = make(map[string]bool, len(names))
		for _, n := range names {
			nameSet[strings.TrimSpace(n)] = true
		}
	}
	out := make([]RuleGroup, 0, len(all))
	for _, g := range all {
		if nameSet != nil && !nameSet[g.Name] {
			continue
		}
		if media == nil || (media.Type == "" && media.Category == "") {
			// 无媒体信息：仅全局组（media_type 空）适用。
			if g.MediaType == "" {
				out = append(out, g)
			}
			continue
		}
		mt := mediaTypeCN(media.Type)
		switch {
		case g.MediaType == "":
			out = append(out, g)
		case g.MediaType == mt && g.Category == "":
			out = append(out, g)
		case media.Category != "" && g.Category == media.Category:
			out = append(out, g)
		}
	}
	return out, nil
}
