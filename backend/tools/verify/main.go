// Command verify 加载转换产物目录（默认 tools/out），验证全部 yaml 能被本项目
// LoadDefinitions 反序列化、EnsureDefaults 分流不报错，并统计 schema 分布。
//
// 运行：cd /d/PTPatronus/backend && go run ./tools/verify [dir]
package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/hxpt/ptpatronus/internal/site"
)

func main() {
	dir := "tools/out"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	r := site.NewRegistry()
	if err := r.LoadDefinitions(dir); err != nil {
		fmt.Println("LoadDefinitions ERR:", err)
		os.Exit(1)
	}
	defs := r.Definitions()
	fmt.Printf("loaded %d definitions from %s\n", len(defs), dir)

	schemas := map[string]int{}
	dead, withSelRows, withUserInfo := 0, 0, 0
	for _, d := range defs {
		site.EnsureDefaults(d) // 验证 dispatch 分流不 panic
		schemas[d.Schema]++
		if d.Dead {
			dead++
		}
		if d.Selectors.Rows.Selector != "" {
			withSelRows++
		}
		if len(d.UserInfo.Fields) > 0 {
			withUserInfo++
		}
	}
	fmt.Println("EnsureDefaults OK on all (dispatch + idempotent)")
	fmt.Printf("dead: %d | explicit selectors.rows: %d | explicit user_info.fields: %d\n", dead, withSelRows, withUserInfo)

	keys := make([]string, 0, len(schemas))
	for k := range schemas {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Println("schema distribution:")
	for _, k := range keys {
		label := k
		if label == "" {
			label = "(empty/generic)"
		}
		fmt.Printf("  %-20s %d\n", label, schemas[k])
	}
}
