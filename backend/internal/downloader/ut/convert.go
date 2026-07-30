package ut

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// uT STATUS bitfield。
const (
	stStarted  = 1
	stChecking = 2
	stError    = 16
	stPaused   = 32
	stQueued   = 64
)

// magnet btih（hex v1 40 位 / v2 64 位；base32 形式暂不支持）。
var magnetHexRe = regexp.MustCompile(`urn:btih:([a-fA-F0-9]{40,64})`)

func magnetInfoHash(magnet string) string {
	m := magnetHexRe.FindStringSubmatch(magnet)
	if m == nil {
		return ""
	}
	return strings.ToLower(m[1])
}

// strAt/numAt 安全取元组第 i 项（uT JSON 数字为 float64）。
func strAt(row []any, i int) string {
	if i >= len(row) {
		return ""
	}
	switch v := row[i].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return ""
}

func numAt(row []any, i int) int64 {
	if i >= len(row) {
		return 0
	}
	switch v := row[i].(type) {
	case float64:
		return int64(v)
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return int64(f)
	}
	return 0
}

// toTorrent 把 list=1 的元组映射为统一模型。索引见 PT-depiler TorrentData（29 元组）。
// PROGRESS/RATIO 均为 per-mille（0..1000）→ Progress 0..1、Ratio 直接 /1000。
func toTorrent(row []any) downloader.Torrent {
	status := int(numAt(row, 1))
	prog := float64(numAt(row, 4)) / 1000 // PROGRESS per-mille → 0..1
	var added time.Time
	if t := numAt(row, 23); t > 0 {
		added = time.Unix(t, 0)
	}
	var completed time.Time
	if t := numAt(row, 24); t > 0 { // COMPLETION_DATE（0 = 未完成）
		completed = time.Unix(t, 0)
	}
	return downloader.Torrent{
		Hash:        strAt(row, 0),
		Name:        strAt(row, 2),
		Size:        numAt(row, 3),
		Downloaded:  numAt(row, 5),
		Uploaded:    numAt(row, 6),
		Progress:    prog,
		Ratio:       float64(numAt(row, 7)) / 1000, // RATIO per-mille
		State:       utState(status, prog),
		DLRate:      numAt(row, 9), // DOWNSPEED
		UPRate:      numAt(row, 8), // UPSPEED
		SavePath:    strAt(row, 26),
		Category:    strAt(row, 11), // LABEL
		AddedAt:     added,
		CompletedAt: completed,

		// —— 详情字段（list=1 零额外请求）——
		NumSeeds:      int(numAt(row, 15)),             // SEEDS CONNECTED
		NumLeechs:     int(numAt(row, 12)),             // PEERS CONNECTED
		NumComplete:   int(numAt(row, 16)),             // SEEDS IN SWARM
		NumIncomplete: int(numAt(row, 13)),             // PEERS IN SWARM
		Availability:  float64(numAt(row, 17)) / 65536, // AVAILABILITY（1/65536ths）
		Priority:      int(numAt(row, 18)),             // QUEUE ORDER
	}
}

// utState 把 uT STATUS bitfield 映射为统一状态串（优先级 Checking/Error > Started > Paused > Queued）。
func utState(status int, progress float64) string {
	switch {
	case status&stChecking != 0:
		return "checking"
	case status&stError != 0:
		return "error"
	case status&stStarted != 0:
		if progress >= 1 {
			return "seeding"
		}
		return "downloading"
	case status&stPaused != 0:
		return "paused"
	case status&stQueued != 0:
		return "queued"
	default:
		if progress >= 1 {
			return "seeding"
		}
		return "paused"
	}
}

// safeFileName 剥离路径分隔符与控制字符（防 multipart filename 污染），与 qb 同源逻辑。
func safeFileName(s string) string {
	if s == "" {
		return "file.torrent"
	}
	if i := strings.LastIndexAny(s, `/\\`); i >= 0 {
		s = s[i+1:]
	}
	name := strings.Map(func(r rune) rune {
		if r < 0x20 {
			return -1
		}
		return r
	}, s)
	if name == "" || name == "." {
		return "file.torrent"
	}
	return name
}
