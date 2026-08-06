package chain


import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

func bfEvalExpr(expr string, tr *downloader.Torrent, now time.Time) (bool, string, error) {
	vals := bfExprVars(tr, now)
	parser := &bfExprParser{s: expr, vals: vals}
	ok, err := parser.parseOr()
	if err != nil {
		return false, "", err
	}
	parser.skip()
	if parser.pos < len(parser.s) {
		return false, "", fmt.Errorf("多余输入 @%d", parser.pos)
	}
	if !ok {
		return false, "", nil
	}
	return true, "表达式:" + expr, nil
}

func bfExprVars(tr *downloader.Torrent, now time.Time) map[string]float64 {
	seedH := 0.0
	if !tr.AddedAt.IsZero() {
		seedH = now.Sub(tr.AddedAt).Hours()
	}
	inactiveH := 0.0
	if last := bfLastActive(*tr); !last.IsZero() {
		inactiveH = now.Sub(last).Hours()
	}
	// 数据缺失守卫：用 NaN 表示"该值不可信/未知"，表达式引擎用 Go 原生 float 比较，
	// NaN 与任何数比较均返回 false（恰好"不命中删除"），与基础规则守卫语义一致。
	//   - 下载中（!bfIsSeedingDone）的 up_rate 不可靠（瞬态低速）→ NaN
	//   - swarm 数据缺失（NumComplete<=0，TR/旧 qB 不上报）→ swarm_seeders/leechers 设 NaN
	upRateKB := float64(tr.UPRate) / 1024.0
	if !bfIsSeedingDone(tr) {
		upRateKB = math.NaN()
	}
	swarmSeeders := float64(tr.NumComplete)
	swarmLeechers := float64(tr.NumIncomplete)
	if tr.NumComplete <= 0 {
		swarmSeeders = math.NaN()
		swarmLeechers = math.NaN()
	}
	return map[string]float64{
		"ratio":          tr.Ratio,
		"seed_hours":     seedH,
		"upload_mb":      float64(tr.Uploaded) / float64(1<<20),
		"up_rate_kb":     upRateKB,
		"inactive_hours": inactiveH,
		"swarm_seeders":  swarmSeeders,
		"swarm_leechers": swarmLeechers,
	}
}

type bfExprParser struct {
	s    string
	pos  int
	vals map[string]float64
}

func (p *bfExprParser) skip() {
	for p.pos < len(p.s) {
		switch p.s[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

func (p *bfExprParser) parseOr() (bool, error) {
	left, err := p.parseAnd()
	if err != nil {
		return false, err
	}
	for {
		p.skip()
		if p.pos+1 < len(p.s) && p.s[p.pos:p.pos+2] == "||" {
			p.pos += 2
			right, err := p.parseAnd()
			if err != nil {
				return false, err
			}
			left = left || right
			continue
		}
		return left, nil
	}
}

func (p *bfExprParser) parseAnd() (bool, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return false, err
	}
	for {
		p.skip()
		if p.pos+1 < len(p.s) && p.s[p.pos:p.pos+2] == "&&" {
			p.pos += 2
			right, err := p.parsePrimary()
			if err != nil {
				return false, err
			}
			left = left && right
			continue
		}
		return left, nil
	}
}

func (p *bfExprParser) parsePrimary() (bool, error) {
	p.skip()
	if p.pos >= len(p.s) {
		return false, fmt.Errorf("意外结束")
	}
	if p.s[p.pos] == '(' {
		p.pos++
		v, err := p.parseOr()
		if err != nil {
			return false, err
		}
		p.skip()
		if p.pos >= len(p.s) || p.s[p.pos] != ')' {
			return false, fmt.Errorf("缺少 )")
		}
		p.pos++
		return v, nil
	}
	ident, err := p.readIdent()
	if err != nil {
		return false, err
	}
	val, ok := p.vals[ident]
	if !ok {
		return false, fmt.Errorf("未知变量 %s", ident)
	}
	p.skip()
	op, err := p.readOp()
	if err != nil {
		return false, err
	}
	p.skip()
	num, err := p.readNumber()
	if err != nil {
		return false, err
	}
	switch op {
	case ">=":
		return val >= num, nil
	case "<=":
		return val <= num, nil
	case ">":
		return val > num, nil
	case "<":
		return val < num, nil
	case "==", "=":
		return val == num, nil
	default:
		return false, fmt.Errorf("未知运算符 %s", op)
	}
}

func (p *bfExprParser) readIdent() (string, error) {
	p.skip()
	start := p.pos
	for p.pos < len(p.s) {
		c := p.s[p.pos]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' || (c >= '0' && c <= '9' && p.pos > start) {
			p.pos++
			continue
		}
		break
	}
	if p.pos == start {
		return "", fmt.Errorf("期望变量名 @%d", p.pos)
	}
	return p.s[start:p.pos], nil
}

func (p *bfExprParser) readOp() (string, error) {
	p.skip()
	if p.pos >= len(p.s) {
		return "", fmt.Errorf("期望运算符")
	}
	if p.pos+1 < len(p.s) {
		two := p.s[p.pos : p.pos+2]
		switch two {
		case ">=", "<=", "==":
			p.pos += 2
			return two, nil
		}
	}
	switch p.s[p.pos] {
	case '>', '<', '=':
		op := string(p.s[p.pos])
		p.pos++
		return op, nil
	}
	return "", fmt.Errorf("期望运算符 @%d", p.pos)
}

func (p *bfExprParser) readNumber() (float64, error) {
	p.skip()
	start := p.pos
	if p.pos < len(p.s) && (p.s[p.pos] == '+' || p.s[p.pos] == '-') {
		p.pos++
	}
	for p.pos < len(p.s) {
		c := p.s[p.pos]
		if (c >= '0' && c <= '9') || c == '.' {
			p.pos++
			continue
		}
		break
	}
	if p.pos == start {
		return 0, fmt.Errorf("期望数字 @%d", p.pos)
	}
	return strconv.ParseFloat(p.s[start:p.pos], 64)
}

