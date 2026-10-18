package filter

import (
	"regexp"
	"strings"

	"github.com/hxpt/ptpatronus/internal/db/oper"
)

// expr 是单个优先级层级表达式（不含 '>'）的 AST 节点。
// 求值时传入规则匹配回调 match(id)，由引擎负责实际的单规则判定（含 TMDB 短路、include/exclude 等）。
type expr interface {
	eval(match func(id string) bool) bool
}

type atomExpr struct{ id string }

func (a *atomExpr) eval(m func(string) bool) bool { return m(a.id) }

type notExpr struct{ x expr }

func (n *notExpr) eval(m func(string) bool) bool { return !n.x.eval(m) }

// binaryExpr 同时表达 and/or；op 为 "and"|"or"。
// 对齐 MP-2 pyparsing infix_notation：'&' 与 '|' **同优先级、左结合**。
type binaryExpr struct {
	op   string
	l, r expr
}

func (b *binaryExpr) eval(m func(string) bool) bool {
	if b.op == "and" {
		return b.l.eval(m) && b.r.eval(m)
	}
	return b.l.eval(m) || b.r.eval(m)
}

// tokenRe 匹配规则 id：字母开头后跟字母数字，或 数字开头+字母+字母数字（兼容 4K / 1080P）。
// 对齐 MP-2 Combine(Word(alphas, alphanums) | (Word(nums) + Word(alphas, alphanums)))。
// 不加 ^ 锚定——parser 逐位置扫描时用 loc[0]==0 约束「当前位置即 token 开头」，
// extractTokens 则用 FindAllString 在整条 rule_string 上抓全部 id。
var tokenRe = regexp.MustCompile(`[A-Za-z][A-Za-z0-9]*|[0-9]+[A-Za-z][A-Za-z0-9]*`)

type ruleParser struct {
	s   string
	pos int
}

// SplitLevels 把 rule_string 按 '>' 切优先级层（trim，拒绝空层）。
func SplitLevels(ruleString string) ([]string, error) {
	parts := strings.Split(ruleString, ">")
	levels := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t == "" {
			return nil, oper.InputReasonf("规则表达式里有空层级：%q", ruleString)
		}
		levels = append(levels, t)
	}
	return levels, nil
}

// Parse 解析单层表达式，返回 AST。空字符串报错。
func Parse(level string) (expr, error) {
	p := &ruleParser{s: level}
	p.skipSpaces()
	e, err := p.parseBinary()
	if err != nil {
		return nil, err
	}
	p.skipSpaces()
	if p.pos != len(p.s) {
		return nil, oper.InputReasonf("表达式 %q 在位置 %d 有没吃完的字符 %q，检查这里的括号或运算符", level, p.pos, p.s[p.pos:])
	}
	return e, nil
}

// parseBinary: parseNot (('&'|'|') parseNot)* —— & | 同优先级、左结合。
func (p *ruleParser) parseBinary() (expr, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for {
		p.skipSpaces()
		if p.pos >= len(p.s) || (p.s[p.pos] != '&' && p.s[p.pos] != '|') {
			break
		}
		op := p.s[p.pos]
		p.pos++
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		opStr := "and"
		if op == '|' {
			opStr = "or"
		}
		left = &binaryExpr{op: opStr, l: left, r: right}
	}
	return left, nil
}

// parseNot: '!' parseNot | parseAtom —— '!' 右结合、优先级高于 & |。
func (p *ruleParser) parseNot() (expr, error) {
	p.skipSpaces()
	if p.pos < len(p.s) && p.s[p.pos] == '!' {
		p.pos++
		x, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return &notExpr{x: x}, nil
	}
	return p.parseAtom()
}

// parseAtom: '(' parseBinary ')' | <token>。
func (p *ruleParser) parseAtom() (expr, error) {
	p.skipSpaces()
	if p.pos >= len(p.s) {
		return nil, oper.InputReasonf("表达式意外结束：括号或运算符没写完")
	}
	if p.s[p.pos] == '(' {
		p.pos++ // 跳过 '('
		e, err := p.parseBinary()
		if err != nil {
			return nil, err
		}
		p.skipSpaces()
		if p.pos >= len(p.s) || p.s[p.pos] != ')' {
			return nil, oper.InputReasonf("缺少右括号")
		}
		p.pos++ // 跳过 ')'
		return e, nil
	}
	loc := tokenRe.FindStringIndex(p.s[p.pos:])
	if loc == nil || loc[0] != 0 {
		return nil, oper.InputReasonf("位置 %d 处应该是规则 id，实际写的是 %q", p.pos, p.s[p.pos:])
	}
	tok := p.s[p.pos : p.pos+loc[1]]
	p.pos += loc[1]
	return &atomExpr{id: tok}, nil
}

func (p *ruleParser) skipSpaces() {
	for p.pos < len(p.s) && (p.s[p.pos] == ' ' || p.s[p.pos] == '\t') {
		p.pos++
	}
}

// extractTokens 提取 rule_string 中出现的全部规则 id（用于校验引用是否存在）。
func extractTokens(ruleString string) []string {
	return tokenRe.FindAllString(ruleString, -1)
}
