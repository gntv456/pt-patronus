package qb

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// toTorrent 把 qB sync/maindata 里的种子字段映射为统一模型。
// 详情字段（做种数/可用性/队列/标志位等）sync 已返回，零额外请求。
func toTorrent(h string, m map[string]any) downloader.Torrent {
	var completedAt time.Time
	if co := num(m["completion_on"]); co > 0 { // 0/缺失 = 未完成
		completedAt = time.Unix(co, 0)
	}
	var lastActivity time.Time
	if la := num(m["last_activity"]); la > 0 {
		lastActivity = time.Unix(la, 0)
	}
	return downloader.Torrent{
		Hash:       h,
		Name:       str(m["name"]),
		Size:       num(m["size"]),
		Downloaded: num(m["downloaded"]),
		Uploaded:   num(m["uploaded"]),
		Progress:   flo(m["progress"]),
		Ratio:      flo(m["ratio"]),
		State:      str(m["state"]),
		DLRate:     num(m["dlspeed"]),
		UPRate:     num(m["upspeed"]),
		SavePath:   str(m["save_path"]),
		Category:   str(m["category"]),
		Tags:       splitTags(str(m["tags"])),
		AddedAt:    time.Unix(num(m["added_on"]), 0),

		// —— 详情字段（sync/maindata 零额外请求）——
		NumSeeds:         int(num(m["num_seeds"])),
		NumLeechs:        int(num(m["num_leechs"])),
		NumComplete:      int(num(m["num_complete"])),
		NumIncomplete:    int(num(m["num_incomplete"])),
		Availability:     flo(m["availability"]),
		RatioLimit:       flo(m["ratio_limit"]),
		SeedingTimeLimit: int(num(m["seeding_time_limit"])),
		ForceStart:       boolean(m["force_start"]),
		AutoTMM:          boolean(m["auto_tmm"]),
		Sequential:       boolean(m["seq_dl"]),
		FirstLastPiece:   boolean(m["f_l_piece_prio"]),
		Priority:         int(num(m["priority"])),
		CompletedAt:      completedAt,
		LastActivity:     lastActivity,
	}
}

// joinHashes 拼接 qB 要求的 | 分隔 hash 串（空 = all）。
func joinHashes(hashes []string) string {
	if len(hashes) == 0 {
		return "all"
	}
	return strings.Join(hashes, "|")
}

// parseVersion 解析 major.minor（忽略 patch）。
func parseVersion(v string) (major, minor int) {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) >= 1 {
		major, _ = strconv.Atoi(stripNonDigit(parts[0]))
	}
	if len(parts) >= 2 {
		minor, _ = strconv.Atoi(stripNonDigit(parts[1]))
	}
	return
}

func stripNonDigit(s string) string {
	out := ""
	for _, r := range s {
		if r >= '0' && r <= '9' {
			out += string(r)
		}
	}
	return out
}

// str/num/flo：any → 强类型（qB JSON 数字为 float64）。
func str(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func num(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return int64(f)
	}
	return 0
}

func flo(v any) float64 {
	switch n := v.(type) {
	case float64:
		return float64(n)
	case int64:
		return float64(n)
	case int:
		return float64(n)
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	}
	return 0
}

// boolean 处理 sync 中可能为 bool 或 0/1 数值的标志位。
func boolean(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case float64:
		return b != 0
	case int64:
		return b != 0
	case int:
		return b != 0
	}
	return false
}

func splitTags(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, t := range strings.Split(s, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}
