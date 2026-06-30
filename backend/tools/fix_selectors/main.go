// Command fix_selectors 修复 definitions/*.yaml 里 cascidia v1.3.3 不支持的选择器伪类。
// 仅做语义等价的安全改写（幂等，可重跑）：
//   - tr:gt(0) → tr：cascadia 无 :gt；ParseSearch 已跳过表头行（title+download 空），等价。
//   - :has(> x)/:has( > x) → :has(x)：cascadia 不支持 :has 内前导组合子；后代匹配等价
//     （icon 多为 <a> 唯一子代，M3 unit3d defaults 用同样改写）。
//   - 裸 :first（非 :first-child/:first-of-type）→ 删除：cascadia 无裸 :first；
//     Extract 已 .First()，语义等价。
//
// 刻意不动 :last/:eq(N)/:lt(N)——语义非等效（Extract.First ≠ last；:eq 索引 ≠ :nth-child 位置），
// 留 M6 真实页面校准。
//
// 运行：cd backend && go run ./tools/fix_selectors [dir]（dir 默认 definitions）
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	gtRe    = regexp.MustCompile(`:gt\(0\)`)
	hasRe   = regexp.MustCompile(`:has\(\s*>\s*`)
	firstRe = regexp.MustCompile(`(?m):first([^A-Za-z0-9_\-]|$)`) // 裸 :first，保留 :first-child/-of-type
)

func fix(b []byte) ([]byte, bool) {
	orig := string(b)
	s := gtRe.ReplaceAllString(orig, "")
	s = hasRe.ReplaceAllString(s, ":has(")
	s = firstRe.ReplaceAllString(s, "$1")
	return []byte(s), s != orig
}

func main() {
	dir := "definitions"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	changed := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		out, did := fix(b)
		if !did {
			continue
		}
		if err := os.WriteFile(p, out, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}
		changed++
	}
	fmt.Printf("fix_selectors: rewrote %d file(s) in %s\n", changed, dir)
}
