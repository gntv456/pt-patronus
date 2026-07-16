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
