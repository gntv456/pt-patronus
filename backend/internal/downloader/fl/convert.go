package fl

import (
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

func toTorrent(p torrentProps) downloader.Torrent {
	// percentComplete 可能是 0..100 或 0..1；>1 则 /100
	prog := p.PercentComplete
	if prog > 1.0001 {
		prog = prog / 100
	}
	cat := ""
	if len(p.Tags) > 0 {
		cat = p.Tags[0]
	}
	var added time.Time
	if p.DateAdded > 0 {
		// 可能是秒或毫秒
		ts := int64(p.DateAdded)
		if ts > 1e12 {
			ts = ts / 1000
		}
		added = time.Unix(ts, 0)
	}
	var completed time.Time
	if p.DateFinished > 0 {
		ts := int64(p.DateFinished)
		if ts > 1e12 {
			ts = ts / 1000
		}
		completed = time.Unix(ts, 0)
	}
	hash := strings.ToLower(p.Hash)
	return downloader.Torrent{
		Hash:          hash,
		Name:          p.Name,
		Size:          p.SizeBytes,
		Downloaded:    p.DownTotal,
		Uploaded:      p.UpTotal,
		Progress:      prog,
		Ratio:         p.Ratio,
		State:         flState(p.Status, prog, p.IsComplete),
		DLRate:        p.DownRate,
		UPRate:        p.UpRate,
		SavePath:      p.Directory,
		Category:      cat,
		Tags:          p.Tags,
		AddedAt:       added,
		NumSeeds:      p.SeedsConnected,
		NumLeechs:     p.PeersConnected,
		NumComplete:   p.SeedsTotal,
		NumIncomplete: p.PeersTotal,
		Priority:      p.Priority,
		Sequential:    p.IsSequential,
		CompletedAt:   completed,
	}
}

func flState(status []string, progress float64, complete bool) string {
	has := func(cands ...string) bool {
		for _, s := range status {
			ls := strings.ToLower(s)
			for _, c := range cands {
				if ls == c {
					return true
				}
			}
		}
		return false
	}
	switch {
	case has("error", "e"):
		return "error"
	case has("checking", "ch"):
		return "checking"
	case has("downloading", "d", "ad"):
		return "downloading"
	case has("seeding", "sd", "au"):
		return "seeding"
	case has("stopped", "p", "s"):
		return "paused"
	default:
		if complete || progress >= 1 {
			return "seeding"
		}
		return "paused"
	}
}

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

// trackerTypeStatus jesec TorrentTrackerType → 统一状态字符串（1=HTTP 2=UDP 3=DHT）。
// Flood 不暴露 tier，故用 Status 携带协议类型。
func trackerTypeStatus(t int) string {
	switch t {
	case 1:
		return "http"
	case 2:
		return "udp"
	case 3:
		return "dht"
	default:
		return "unknown"
	}
}

// unionStrings 合并两个字符串切片并去重，保持出现顺序（a 在前，b 中新出现的追加）。
// 供 AddTags / AddTrackers 的读-改-写 diff 使用。
func unionStrings(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, s := range a {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	for _, s := range b {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// subtractStrings 从 a 中剔除 b 包含的项，保持 a 的顺序。供 RemoveTags / RemoveTrackers 使用。
func subtractStrings(a, b []string) []string {
	rm := make(map[string]bool, len(b))
	for _, s := range b {
		if s != "" {
			rm[s] = true
		}
	}
	out := make([]string, 0, len(a))
	for _, s := range a {
		if s == "" || rm[s] {
			continue
		}
		out = append(out, s)
	}
	return out
}
