package filter

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"

	"github.com/hxpt/ptpatronus/internal/db/oper"
)

// reCache 缓存编译后的正则（含 (?i) IGNORECASE）。热路径上每个 pattern 仅编译一次。
// 对齐 MP-2 _compile_ignorecase 的 lru_cache(1024)。
var reCache sync.Map // pattern(string) → *regexp.Regexp

// astCache 缓存 rule_string 解析后的 AST（组定义不常变，Filter 每组每次重解析纯浪费）。
// 声明在本文件因已 import sync；engine.go parseLevels 同包直接引用。
var astCache sync.Map // ruleString(string) → []expr

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

	// 自定义规则/规则组内存缓存：Filter 热路径（搜索/订阅/RSS/刷流每次命中）各读一次 DB First
	// 是瓶颈。saveCustomRules/saveGroups 写后失效；首次 Get 填充。返回拷贝防调用方篡改缓存。
	cacheMu      sync.RWMutex
	rulesCache   []Rule
	rulesLoaded  bool
	groupsCache  []RuleGroup
	groupsLoaded bool
}

// NewStore 构造。
func NewStore(op *oper.SystemConfigOper) *Store { return &Store{op: op} }

// ---------- 自定义规则 ----------

// GetCustomRules 读取自定义规则（按入库顺序）。命中内存缓存免 DB 往返（Filter 热路径）。
func (s *Store) GetCustomRules() ([]Rule, error) {
	s.cacheMu.RLock()
	if s.rulesLoaded {
		out := append([]Rule(nil), s.rulesCache...) // 拷贝防篡改
		s.cacheMu.RUnlock()
		return out, nil
	}
	s.cacheMu.RUnlock()
	v, err := s.op.Get(KeyCustomFilterRules)
	if err != nil {
		return nil, err
	}
	var rs []Rule
	if v != "" {
		if err := json.Unmarshal([]byte(v), &rs); err != nil {
			return nil, fmt.Errorf("解析自定义规则失败：%w", err)
		}
	}
	s.cacheMu.Lock()
	s.rulesCache = append([]Rule(nil), rs...)
	s.rulesLoaded = true
	s.cacheMu.Unlock()
	return rs, nil
}

func (s *Store) saveCustomRules(rs []Rule) error {
	b, err := json.Marshal(rs)
	if err != nil {
		return err
	}
	if err := s.op.Set(KeyCustomFilterRules, string(b)); err != nil {
		return err
	}
	s.cacheMu.Lock()
	s.rulesCache = nil
	s.rulesLoaded = false
	s.cacheMu.Unlock()
	return nil
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
			return Rule{}, oper.InputReasonf("规则 id %q 已存在，请换一个 id 或先删除旧规则", r.ID)
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
		return Rule{}, ruleMissing(oldID)
	}
	for i, e := range rs {
		if i == idx {
			continue
		}
		if e.ID == r.ID {
			return Rule{}, oper.InputReasonf("规则 id %q 已存在，请换一个 id", r.ID)
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
				return oper.InputReasonf("规则 %q 正被规则组 %q 引用，请先从该规则组里去掉它再删除", id, g.Name)
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
		return ruleMissing(id)
	}
	return s.saveCustomRules(out)
}

// ---------- 规则组 ----------

// GetGroups 读取规则组定义（按入库顺序）。命中内存缓存免 DB 往返（Filter 热路径）。
func (s *Store) GetGroups() ([]RuleGroup, error) {
	s.cacheMu.RLock()
	if s.groupsLoaded {
		out := append([]RuleGroup(nil), s.groupsCache...)
		s.cacheMu.RUnlock()
		return out, nil
	}
	s.cacheMu.RUnlock()
	v, err := s.op.Get(KeyUserFilterRuleGroups)
	if err != nil {
		return nil, err
	}
	var gs []RuleGroup
	if v != "" {
		if err := json.Unmarshal([]byte(v), &gs); err != nil {
			return nil, fmt.Errorf("解析规则组失败：%w", err)
		}
	}
	s.cacheMu.Lock()
	s.groupsCache = append([]RuleGroup(nil), gs...)
	s.groupsLoaded = true
	s.cacheMu.Unlock()
	return gs, nil
}

func (s *Store) saveGroups(gs []RuleGroup) error {
	b, err := json.Marshal(gs)
	if err != nil {
		return err
	}
	if err := s.op.Set(KeyUserFilterRuleGroups, string(b)); err != nil {
		return err
	}
	s.cacheMu.Lock()
	s.groupsCache = nil
	s.groupsLoaded = false
	s.cacheMu.Unlock()
	return nil
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
			return RuleGroup{}, oper.InputReasonf("规则组 %q 已存在，请换一个名字", g.Name)
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
		return RuleGroup{}, groupMissing(oldName)
	}
	for i, e := range gs {
		if i == idx {
			continue
		}
		if e.Name == g.Name {
			return RuleGroup{}, oper.InputReasonf("规则组 %q 已存在，请换一个名字", g.Name)
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
		return groupMissing(name)
	}
	if err := s.saveGroups(out); err != nil {
		return err
	}
	s.cleanupGroupFromBindings(name)
	return nil
}

// cleanupGroupFromBindings 从三个全局绑定 key 中移除指定组名。
//
// 这里刻意不把失败上抛成「删除失败」：组已经删掉了，用户重试删除会先撞上
// 「规则组不存在」，绑定里的悬空引用反而永远清不掉。所以按「不改结果、但必须出声」处理——
// 悬空绑定会让那条全局筛选再也找不到规则组（表现为筛选静默不生效），没有这行日志就查不到头绪。
func (s *Store) cleanupGroupFromBindings(name string) {
	for _, key := range []string{KeySearchFilterRuleGroups, KeySubscribeFilterRuleGroups, KeyBestVersionFilterRuleGroups} {
		names, err := s.GetBindings(key)
		if err != nil {
			slog.Warn("规则组绑定读取失败：该绑定可能仍引用已删除的规则组", "key", key, "group", name, "err", err)
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
			if err := s.SetBindings(key, out); err != nil {
				slog.Warn("规则组绑定清理失败：这里仍引用已删除的规则组，对应筛选会静默不生效",
					"key", key, "group", name, "err", err)
			}
		}
	}
}
