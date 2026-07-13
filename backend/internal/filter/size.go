package filter

import (
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/site"
)

const sizeUnit = 1024 * 1024 // MB → bytes（对齐 MP-2 _SIZE_UNIT）

// 体积范围操作符（对齐 MP-2 _parse_size_range）。
const (
	opBetween = "between"
	opGTE     = "gte"
	opLTE     = "lte"
	opUnknown = "unknown" // 纯数字等无法识别格式 → 不匹配
)

// parseSizeRange 解析体积范围（输入 MB）。返回 (op, minBytes, maxBytes)。
//   - "1000-5000" → between
//   - ">1000"     → gte（≥1000MB）
//   - "<5000"     → lte（≤5000MB）
//   - 纯数字 "1000" → unknown（MP-2 同样不匹配，要求显式 >/< 或区间）
func parseSizeRange(sizeRange string) (op string, min, max int64) {
	s := strings.TrimSpace(sizeRange)
	if s == "" {
		return opUnknown, 0, 0
	}
	if strings.Contains(s, "-") {
		parts := strings.SplitN(s, "-", 2)
		mn, ok1 := mbToBytes(parts[0])
		mx, ok2 := mbToBytes(parts[1])
		if !ok1 || !ok2 {
			return opUnknown, 0, 0
		}
		return opBetween, mn, mx
	}
	if strings.HasPrefix(s, ">") {
		mn, ok := mbToBytes(s[1:])
		if !ok {
			return opUnknown, 0, 0
		}
		return opGTE, mn, 0
	}
	if strings.HasPrefix(s, "<") {
		mx, ok := mbToBytes(s[1:])
		if !ok {
			return opUnknown, 0, 0
		}
		return opLTE, 0, mx
	}
	return opUnknown, 0, 0
}

// mbToBytes "1000" → 1000 * sizeUnit bytes。
func mbToBytes(s string) (int64, bool) {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || f < 0 {
		return 0, false
	}
	return int64(f * sizeUnit), true
}

// matchSize 判断总体积（或单集体积）是否落在 sizeRange 内。
// episodeCount 为剧集总集数（MP-2 用 MetaInfo.total_episode）；P1 传 1=按总体积比较。
func matchSize(sizeStr, sizeRange string, episodeCount int) bool {
	if strings.TrimSpace(sizeRange) == "" {
		return true
	}
	if episodeCount < 1 {
		episodeCount = 1
	}
	total := site.ParseBytes(sizeStr)
	per := total / int64(episodeCount)
	op, min, max := parseSizeRange(sizeRange)
	switch op {
	case opBetween:
		return per >= min && per <= max
	case opGTE:
		return per >= min
	case opLTE:
		return per <= max
	}
	return false
}

// parsePublishTime 解析发布时间规则（分钟）。单值 [x]=age 下限（>=x）；范围 [a,b]=闭区间。
func parsePublishTime(pub string) (vals []float64, ok bool) {
	pub = strings.TrimSpace(pub)
	if pub == "" {
		return nil, false
	}
	for _, part := range strings.Split(pub, "-") {
		f, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return nil, false
		}
		vals = append(vals, f)
	}
	return vals, true
}
