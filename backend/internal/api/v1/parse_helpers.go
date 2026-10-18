package v1

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
)

// ---- 共享解析辅助（包内多 handler 复用）----

func parseID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		errcode.Respond(c, errcode.SiteInvalidID)
		return 0, false
	}
	return uint(id), true
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// parseUintSlice 解析逗号分隔的 uint 列表（如 "1,2,3"），忽略空串/非法项；空输入返回 nil。
func parseUintSlice(s string) []uint {
	if s == "" {
		return nil
	}
	out := make([]uint, 0, 4)
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			continue
		}
		out = append(out, uint(n))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
