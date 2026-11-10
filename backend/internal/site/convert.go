// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

import (
	"html"
	"regexp"
	"strconv"
	"strings"
)

var numRe = regexp.MustCompile(`[-+]?[\d,]*\.?\d+`)
var (
	claimPairLabeledRe = regexp.MustCompile(`(?i)(?:认领|認領|Claim)[^0-9]{0,80}(\d[\d,]*)\s*/\s*(\d[\d,]*)`)
	claimPairRe        = regexp.MustCompile(`(\d[\d,]*)\s*/\s*(\d[\d,]*)`)
)

// extractNumber 抽取首个数字串（去千分位逗号）。

func extractNumber(s string) string {
	m := numRe.FindString(s)
	if m == "" {
		return ""
	}
	return strings.ReplaceAll(m, ",", "")
}

// cnUnitMult 中文数量级后缀 → 乘数（n8 等新主题把大数格式化成 "102.2万"，
// 万/亿是显示单位而非文字——parse_number 丢后缀就丢数量级，故在此补偿）。
var cnUnitMult = map[rune]float64{
	'万': 1e4,
	'億': 1e8,
	'亿': 1e8,
}

// extractNumberWithCNUnit 抽取首个数字串并按紧随的中文数量级后缀乘算
// （"102.2万" → 1022000；无后缀行为与 extractNumber 一致）。
// 只认数字后**紧邻**的万/亿（中间隔了别的字就不算数量级，防误乘）。
func extractNumberWithCNUnit(s string) string {
	loc := numRe.FindStringIndex(s)
	if loc == nil {
		return ""
	}
	num := strings.ReplaceAll(s[loc[0]:loc[1]], ",", "")
	rest := strings.TrimLeft(s[loc[1]:], " ")
	// 只看第一个非空白字符：是数量级后缀才乘算，否则原样返回
	// （中间隔了别的字就不算数量级，防误乘；n8 主题 "102.2万" 场景）。
	if runes := []rune(rest); len(runes) > 0 {
		if mult, ok := cnUnitMult[runes[0]]; ok {
			if f, err := strconv.ParseFloat(num, 64); err == nil {
				return strconv.FormatFloat(f*mult, 'f', -1, 64)
			}
		}
	}
	return num
}

// ToInt 把字符串解析为 int（失败返回 0）。

func ToInt(s string) int {
	n, _ := strconv.Atoi(extractNumber(s))
	return n
}

// ToFloat64 解析为 float64。

func ToFloat64(s string) float64 {
	f, _ := strconv.ParseFloat(extractNumber(s), 64)
	return f
}

func ToBool(s string) bool {
	return parseBoolish(s)
}

func parseBoolish(s string) bool {
	normalized := strings.TrimSpace(strings.ToLower(s))
	if normalized == "" {
		return false
	}
	switch normalized {
	case "1", "true", "yes", "y", "on", "donor", "vip", "贵宾", "捐赠者", "捐贈者":
		return true
	case "0", "false", "no", "n", "off", "none", "null", "nil", "-":
		return false
	}
	return strings.Contains(normalized, "donor") ||
		strings.Contains(normalized, "vip") ||
		strings.Contains(normalized, "捐赠") ||
		strings.Contains(normalized, "捐贈")
}

func cleanAvatarValue(s string) string {
	s = strings.TrimSpace(html.UnescapeString(s))
	if s == "" {
		return ""
	}
	lower := strings.ToLower(s)
	if strings.Contains(lower, "default_avatar") || strings.Contains(lower, "defaultavatar") {
		return ""
	}
	return s
}
