package site

import (
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// compileFilterPattern 解析抓取规则 args 的 "pattern:::N" 形式（N=取第几个捕获组，
// 默认 1，0 取全匹配）并编译正则。
//
// 编译失败过去在两个调用点都是裸 `continue`：整条 filter 不生效，原始父节点文本继续往下
// 被 parse_size/parse_number 抽数，于是得到「错值」而不是「没值」（同文件 regexp 支的注释
// 自己描述了这条危害）。definitions/audiobookbay.yaml 的 subtitle 就用了 RE2 不支持的
// 先行断言 `(?=…)`，长期处于这个状态。
//
// 现在与「未命中」同一终态：调用方拿到 ok=false 就给空值——宁可没值，不可错值。
func compileFilterPattern(args string) (re *regexp.Regexp, groupIdx int, ok bool) {
	pat := args
	groupIdx = 1
	if i := strings.Index(pat, ":::"); i >= 0 {
		if gi, err := strconv.Atoi(pat[i+3:]); err == nil && gi >= 0 {
			groupIdx = gi
			pat = pat[:i]
		}
	}
	compiled, err := regexp.Compile(pat)
	if err != nil {
		warnBadFilterPattern(pat, err)
		return nil, groupIdx, false
	}
	return compiled, groupIdx, true
}

// badFilterPatterns 去重：抓取是热路径（每次搜索每个字段都过这里），
// 同一条坏规则每轮刷一次日志等于没日志。
var badFilterPatterns sync.Map

func warnBadFilterPattern(pat string, err error) {
	if _, loaded := badFilterPatterns.LoadOrStore(pat, struct{}{}); loaded {
		return
	}
	slog.Warn("site filter regexp does not compile; this rule yields no value instead of raw text",
		"pattern", pat, "err", err)
}
