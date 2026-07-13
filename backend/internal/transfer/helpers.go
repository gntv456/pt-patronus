package transfer

import (
	"strconv"
	"strings"
)

func atoiSafe(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func atoi(n int) string { return strconv.Itoa(n) }

func isAllDigit(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
