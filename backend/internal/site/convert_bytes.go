package site

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var bytesRe = regexp.MustCompile(`(?i)([\d.,]+)\s*([KMGTPE]?i?B?)`)

// ParseBytes 把 "12.4 TB" / "1,234 GB" / "512 MB" / "5 GiB" 解析为字节数（int64）。
// 兼容二进制单位 TiB/GiB（统一去 i 归一到 TB/GB…）。

func ParseBytes(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	m := bytesRe.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	val, err := strconv.ParseFloat(extractNumber(m[1]), 64)
	if err != nil {
		return 0
	}
	// 归一单位：TiB→TB、GiB→GB（去掉二进制标记 i）
	unit := strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(m[2])), "I", "")
	var mult float64 = 1
	switch unit {
	case "K", "KB":
		mult = 1 << 10
	case "M", "MB":
		mult = 1 << 20
	case "G", "GB":
		mult = 1 << 30
	case "T", "TB":
		mult = 1 << 40
	case "P", "PB":
		mult = 1 << 50
	case "E", "EB":
		mult = 1 << 60
	}
	// 溢出钳制：超大数值（如 "999… TB"）经 float64×mult 转 int64 会翻转为负数，
	// 统一钳到 MaxInt64，调用方（做差/展示）不会拿到负体积。
	if product := val * mult; product >= math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(val * mult)
}

func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

// SumBytes sums all recognizable size tokens in a string.

func SumBytes(s string) int64 {
	var total int64
	for _, m := range bytesRe.FindAllStringSubmatch(s, -1) {
		if len(m) < 3 {
			continue
		}
		total += ParseBytes(m[0])
	}
	return total
}

// LastBytes returns the final recognizable size token in a string.

func LastBytes(s string) int64 {
	matches := bytesRe.FindAllStringSubmatch(s, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		if len(matches[i]) >= 3 {
			return ParseBytes(matches[i][0])
		}
	}
	return 0
}

// queryParam 从 s（URL 或含 query 的串）取指定参数名首个值。
// 先走 url.Parse（标准 URL），取空或失败再正则兜底（[?&]name=value）。
// 取不到则原样返回 s（供后续 filter 继续处理）。
