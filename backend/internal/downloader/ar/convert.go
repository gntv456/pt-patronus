package ar

import (
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// rawTask 是 aria2.tellStatus / tellActive 返回的任务。
// 数值字段 Aria2 以字符串返回，故用 string 再 parse。
type rawTask struct {
	GID             string `json:"gid"`
	Status          string `json:"status"`
	TotalLength     string `json:"totalLength"`
	CompletedLength string `json:"completedLength"`
	UploadLength    string `json:"uploadLength"`
	DownloadSpeed   string `json:"downloadSpeed"`
	UploadSpeed     string `json:"uploadSpeed"`
	Dir             string `json:"dir"`
	InfoHash        string `json:"infoHash"`
	NumSeeders      string `json:"numSeeders"`
	Connections     string `json:"connections"`
	Bittorrent      *struct {
		Info *struct {
			Name string `json:"name"`
		} `json:"info"`
		AnnounceList [][]string `json:"announceList"`
	} `json:"bittorrent"`
	Files []struct {
		Index           string `json:"index"`
		Path            string `json:"path"`
		Length          string `json:"length"`
		CompletedLength string `json:"completedLength"`
		Selected        string `json:"selected"`
	} `json:"files"`
}

func parseI64(s string) int64 {
	if s == "" {
		return 0
	}
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

// toTorrent 将 Aria2 任务映射为统一模型。
// Hash 使用 GID：pause/remove/changeOption 等接口均以 GID 寻址。
func toTorrent(t rawTask) downloader.Torrent {
	total := parseI64(t.TotalLength)
	done := parseI64(t.CompletedLength)
	up := parseI64(t.UploadLength)
	var progress float64
	if total > 0 {
		progress = float64(done) / float64(total)
	}
	var ratio float64
	if total > 0 {
		ratio = float64(up) / float64(total)
	}
	name := ""
	if t.Bittorrent != nil && t.Bittorrent.Info != nil {
		name = t.Bittorrent.Info.Name
	}
	return downloader.Torrent{
		Hash:       t.GID,
		InfoHash:   strings.ToLower(t.InfoHash),
		Name:       name,
		Size:       total,
		Downloaded: done,
		Uploaded:   up,
		Progress:   progress,
		Ratio:      ratio,
		State:      arState(t.Status, progress),
		DLRate:     parseI64(t.DownloadSpeed),
		UPRate:     parseI64(t.UploadSpeed),
		SavePath:   t.Dir,
		NumSeeds:   int(parseI64(t.NumSeeders)),
		NumLeechs:  int(parseI64(t.Connections)),
	}
}

// arState 映射 Aria2 status → 统一状态串。
func arState(status string, progress float64) string {
	switch strings.ToLower(status) {
	case "active":
		if progress >= 1 {
			return "seeding"
		}
		return "downloading"
	case "waiting":
		return "queued"
	case "paused":
		return "paused"
	case "error", "removed":
		return "error"
	case "complete":
		if progress >= 1 {
			return "seeding"
		}
		return "paused"
	default:
		return "paused"
	}
}
