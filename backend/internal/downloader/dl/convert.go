// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package dl

import (
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// dlTorrent 是 Deluge core.get_torrents_status 返回的单种子字段集。
type dlTorrent struct {
	Hash                string  `json:"hash"`
	Name                string  `json:"name"`
	Progress            float64 `json:"progress"` // 0-100
	Ratio               float64 `json:"ratio"`
	TimeAdded           float64 `json:"time_added"` // unix
	CompletedTime       float64 `json:"completed_time"`
	SavePath            string  `json:"save_path"`
	Label               string  `json:"label"`
	State               string  `json:"state"`
	TotalSize           int64   `json:"total_size"`
	UploadPayloadRate   int64   `json:"upload_payload_rate"`
	DownloadPayloadRate int64   `json:"download_payload_rate"`
	TotalUploaded       int64   `json:"total_uploaded"`
	TotalDone           int64   `json:"total_done"`
	NumSeeds            int     `json:"num_seeds"`
	NumPeers            int     `json:"num_peers"`
	TotalSeeds          int     `json:"total_seeds"`
	TotalPeers          int     `json:"total_peers"`
}

// toTorrent 把 Deluge 种子字段映射为统一模型。
func toTorrent(t dlTorrent) downloader.Torrent {
	var added time.Time
	if t.TimeAdded > 0 {
		added = time.Unix(int64(t.TimeAdded), 0)
	}
	var completed time.Time
	if t.CompletedTime > 0 { // 0 = 未完成
		completed = time.Unix(int64(t.CompletedTime), 0)
	}
	return downloader.Torrent{
		Hash:        t.Hash,
		Name:        t.Name,
		Size:        t.TotalSize,
		Downloaded:  t.TotalDone,
		Uploaded:    t.TotalUploaded,
		Progress:    t.Progress / 100,
		Ratio:       t.Ratio,
		State:       dlState(t.State, t.Progress),
		DLRate:      t.DownloadPayloadRate,
		UPRate:      t.UploadPayloadRate,
		SavePath:    t.SavePath,
		Category:    t.Label,
		AddedAt:     added,
		CompletedAt: completed,

		NumSeeds:      t.NumSeeds,
		NumLeechs:     t.NumPeers,
		NumComplete:   t.TotalSeeds,
		NumIncomplete: t.TotalPeers,
	}
}

// dlState 把 Deluge state 字符串映射为统一状态串。
func dlState(state string, progress float64) string {
	switch strings.ToLower(state) {
	case "downloading":
		return "downloading"
	case "seeding":
		return "seeding"
	case "paused":
		return "paused"
	case "queued":
		return "queued"
	case "checking":
		return "checking"
	case "error":
		return "error"
	case "active":
		if progress >= 100 {
			return "seeding"
		}
		return "downloading"
	default:
		if progress >= 100 {
			return "seeding"
		}
		return "paused"
	}
}

// bytesToKB bytes/s → KiB/s（向上取整）。0/负返回 0（=不限）。
func bytesToKB(b int64) int64 {
	if b <= 0 {
		return 0
	}
	return (b + 1023) / 1024
}

// kbToBytes KiB/s → bytes/s。
func kbToBytes(kb int64) int64 {
	if kb <= 0 {
		return 0
	}
	return kb * 1024
}
