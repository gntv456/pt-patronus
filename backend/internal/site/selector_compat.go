package site

import (
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func findCompat(ctx *goquery.Selection, sel string) *goquery.Selection {
	if ctx == nil {
		return &goquery.Selection{}
	}
	if !needsCompatSelector(sel) {
		return ctx.Find(sel)
	}
	steps := splitSelectorSteps(sel)
	if len(steps) == 0 {
		return ctx.Find(sel)
	}
	cur := ctx
	for _, step := range steps {
		base, pseudo, idx := stripJQueryPseudo(step.selector)
		if base == "" {
			base = "*"
		}
		switch step.relation {
		case "child":
			cur = cur.ChildrenFiltered(base)
		case "adjacent":
			cur = cur.NextFiltered(base)
		case "sibling":
			cur = cur.NextAllFiltered(base)
		default:
			cur = cur.Find(base)
		}
		switch pseudo {
		case "first":
			cur = cur.First()
		case "last":
			cur = cur.Last()
		case "eq":
			cur = cur.Eq(idx)
		case "gt":
			if idx+1 >= cur.Length() {
				cur = cur.Slice(cur.Length(), cur.Length())
			} else {
				cur = cur.Slice(idx+1, cur.Length())
			}
		case "lt":
			if idx <= 0 {
				cur = cur.Slice(0, 0)
			} else if idx > cur.Length() {
				cur = cur.Slice(0, cur.Length())
			} else {
				cur = cur.Slice(0, idx)
			}
		}
		if cur.Length() == 0 {
			return cur
		}
	}
	return cur
}

func needsCompatSelector(sel string) bool {
	trimmed := strings.TrimSpace(sel)
	return strings.HasPrefix(trimmed, ">") || strings.HasPrefix(trimmed, "+") || strings.HasPrefix(trimmed, "~") ||
		strings.Contains(sel, ":eq(") || strings.Contains(sel, ":gt(") || strings.Contains(sel, ":lt(") ||
		strings.Contains(sel, ":first") || strings.Contains(sel, ":last")
}

type selectorStep struct {
	selector string
	relation string
}

func splitSelectorSteps(sel string) []selectorStep {
	var out []selectorStep
	var b strings.Builder
	relation := ""
	depth := 0
	var quote rune
	flush := func() {
		token := strings.TrimSpace(b.String())
		if token != "" {
			out = append(out, selectorStep{selector: token, relation: relation})
			relation = ""
		}
		b.Reset()
	}
	for _, r := range sel {
		if quote != 0 {
			b.WriteRune(r)
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
			b.WriteRune(r)
		case '(', '[':
			depth++
			b.WriteRune(r)
		case ')', ']':
			if depth > 0 {
				depth--
			}
			b.WriteRune(r)
		case '>':
			if depth == 0 {
				flush()
				relation = "child"
				continue
			}
			b.WriteRune(r)
		case '+':
			if depth == 0 {
				flush()
				relation = "adjacent"
				continue
			}
			b.WriteRune(r)
		case '~':
			if depth == 0 {
				flush()
				relation = "sibling"
				continue
			}
			b.WriteRune(r)
		case ' ', '\t', '\n', '\r':
			if depth == 0 {
				flush()
				continue
			}
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	flush()
	return out
}

func stripJQueryPseudo(sel string) (base, pseudo string, idx int) {
	sel = strings.TrimSpace(sel)
	if strings.HasSuffix(sel, ":first") {
		return strings.TrimSpace(strings.TrimSuffix(sel, ":first")), "first", 0
	}
	if strings.HasSuffix(sel, ":last") {
		return strings.TrimSpace(strings.TrimSuffix(sel, ":last")), "last", 0
	}
	if strings.HasSuffix(sel, ")") {
		if i := topLevelSuffixIndex(sel, ":eq("); i >= 0 {
			raw := strings.TrimSuffix(sel[i+4:], ")")
			n, _ := strconv.Atoi(strings.TrimSpace(raw))
			return strings.TrimSpace(sel[:i]), "eq", n
		}
		if i := topLevelSuffixIndex(sel, ":gt("); i >= 0 {
			raw := strings.TrimSuffix(sel[i+4:], ")")
			n, _ := strconv.Atoi(strings.TrimSpace(raw))
			return strings.TrimSpace(sel[:i]), "gt", n
		}
		if i := topLevelSuffixIndex(sel, ":lt("); i >= 0 {
			raw := strings.TrimSuffix(sel[i+4:], ")")
			n, _ := strconv.Atoi(strings.TrimSpace(raw))
			return strings.TrimSpace(sel[:i]), "lt", n
		}
	}
	return sel, "", 0
}

func topLevelSuffixIndex(sel, marker string) int {
	depth := 0
	var quote rune
	for i, r := range sel {
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case '(', '[':
			if strings.HasPrefix(sel[i:], marker) && depth == 0 {
				return i
			}
			depth++
		case ')', ']':
			if depth > 0 {
				depth--
			}
		case ':':
			if depth == 0 && strings.HasPrefix(sel[i:], marker) {
				return i
			}
		}
	}
	return -1
}
