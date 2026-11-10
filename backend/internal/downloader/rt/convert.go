// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package rt

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// toTorrent 映射 ruTorrent list 元组（见 PT-depiler torrentData 索引）。
func toTorrent(hash string, row []any) downloader.Torrent {
	isOpen := iv(row, 0)
	isHashChecking := iv(row, 1)
	getState := iv(row, 3)
	name := sv(row, 4)
	size := iv(row, 5)
	completedChunks := iv(row, 6)
	sizeChunks := iv(row, 7)
	downloaded := iv(row, 8)
	uploaded := iv(row, 9)
	ratioRaw := iv(row, 10)
	ul := iv(row, 11)
	dl := iv(row, 12)
	label := decodeLabel(sv(row, 14))
	peersConnected := iv(row, 17)
	seedsActual := iv(row, 18)
	priority := int(iv(row, 20))
	stateChanged := iv(row, 21)
	getHashing := iv(row, 23)
	hashedChunks := iv(row, 24)
	basePath := sv(row, 25)
	isActive := iv(row, 28)
	msg := sv(row, 29)

	chunks := completedChunks
	if isHashChecking != 0 {
		chunks = hashedChunks
	}
	var progress float64
	if sizeChunks > 0 {
		progress = float64(chunks) / float64(sizeChunks)
	}
	ratio := float64(ratioRaw) / 1000

	savePath := basePath
	if i := strings.LastIndex(basePath, "/"); i >= 0 && i+1 < len(basePath) && basePath[i+1:] == name {
		savePath = basePath[:i]
	}

	var added time.Time
	if stateChanged > 0 {
		added = time.Unix(stateChanged, 0)
	}

	return downloader.Torrent{
		Hash:       strings.ToLower(hash),
		Name:       name,
		Size:       size,
		Downloaded: downloaded,
		Uploaded:   uploaded,
		Progress:   progress,
		Ratio:      ratio,
		State:      rtState(isOpen, isHashChecking, getState, isActive, getHashing, progress, msg),
		DLRate:     dl,
		UPRate:     ul,
		SavePath:   savePath,
		Category:   label,
		AddedAt:    added,
		NumSeeds:   int(seedsActual),
		NumLeechs:  int(peersConnected),
		Priority:   priority,
	}
}

func rtState(isOpen, isHashChecking, getState, isActive, getHashing int64, progress float64, msg string) string {
	if isOpen != 0 {
		if getState == 0 || isActive == 0 {
			return "paused"
		}
		if progress >= 1 {
			return "seeding"
		}
		return "downloading"
	}
	if getHashing != 0 {
		return "queued"
	}
	if isHashChecking != 0 {
		return "checking"
	}
	if msg != "" && msg != "Tracker: [Tried all trackers.]" {
		return "error"
	}
	if progress >= 1 {
		return "seeding"
	}
	return "paused"
}

func iv(row []any, i int) int64 {
	if i >= len(row) {
		return 0
	}
	switch v := row[i].(type) {
	case float64:
		return int64(v)
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	}
	return 0
}

func sv(row []any, i int) string {
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

func decodeLabel(s string) string {
	if s == "" {
		return ""
	}
	d, err := url.QueryUnescape(s)
	if err != nil {
		return s
	}
	return d
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

// peerFlags 拼接 ruTorrent peer 标志位为短串（对标 qB flags 字段）：
// I=入站连接 E=加密传输 S=被忽略(snubbed)。
func peerFlags(incoming, encrypted, snubbed bool) string {
	var b strings.Builder
	if incoming {
		b.WriteString("I")
	}
	if encrypted {
		b.WriteString("E")
	}
	if snubbed {
		b.WriteString("S")
	}
	return b.String()
}

// rtTrackerStatus 映射 ruTorrent tracker 字段为状态串。
// enabled: 0=禁用 1=启用；ttype: 1=http 2=udp 3=dht。
// ruTorrent 的 tracker 状态字段不如 qB 丰富（无 updating/not_working 细分），
// 简化为 working/disabled 两态。
func rtTrackerStatus(enabled, ttype int64) string {
	if enabled == 0 {
		return "disabled"
	}
	return "working"
}
