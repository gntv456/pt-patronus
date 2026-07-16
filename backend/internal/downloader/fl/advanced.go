package fl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// Recheck → POST /api/torrents/check-hash
func (c *Client) Recheck(ctx context.Context, hashes []string) error {
	status, _, err := c.raw(ctx, http.MethodPost, "/api/torrents/check-hash",
		mustJSON(map[string]any{"hashes": upperAll(hashes)}), "application/json", true)
	if err != nil {
		return err
	}
	if status == 404 {
		return downloader.ErrNotImplemented
	}
	if status != 200 && status != 202 {
		return fmt.Errorf("recheck status %d", status)
	}
	return nil
}

func (c *Client) Reannounce(ctx context.Context, hashes []string) error {
	status, _, err := c.raw(ctx, http.MethodPost, "/api/torrents/reannounce",
		mustJSON(map[string]any{"hashes": upperAll(hashes)}), "application/json", true)
	if err != nil {
		return err
	}
	if status == 404 {
		return downloader.ErrNotImplemented
	}
	if status != 200 && status != 202 {
		return fmt.Errorf("reannounce status %d", status)
	}
	return nil
}

func (c *Client) SetLocation(ctx context.Context, hashes []string, location string) error {
	status, _, err := c.raw(ctx, http.MethodPost, "/api/torrents/move",
		mustJSON(map[string]any{"hashes": upperAll(hashes), "destination": location, "moveFiles": true}),
		"application/json", true)
	if err != nil {
		return err
	}
	if status == 404 {
		return downloader.ErrNotImplemented
	}
	if status != 200 && status != 202 {
		return fmt.Errorf("move status %d", status)
	}
	return nil
}

func (c *Client) SetForceStart(ctx context.Context, hashes []string, enable bool) error {
	return downloader.ErrNotImplemented
}

func (c *Client) ListCategories(ctx context.Context) ([]string, error) {
	list, err := c.ListTorrents(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, t := range list {
		if t.Category != "" && !seen[t.Category] {
			seen[t.Category] = true
			out = append(out, t.Category)
		}
	}
	return out, nil
}

func (c *Client) ListTags(ctx context.Context) ([]string, error) {
	list, err := c.ListTorrents(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, t := range list {
		for _, tag := range t.Tags {
			if tag != "" && !seen[tag] {
				seen[tag] = true
				out = append(out, tag)
			}
		}
	}
	return out, nil
}

func (c *Client) SetTags(ctx context.Context, hashes []string, tags []string) error {
	status, _, err := c.raw(ctx, http.MethodPatch, "/api/torrents/tags",
		mustJSON(map[string]any{"hashes": upperAll(hashes), "tags": tags}), "application/json", true)
	if err != nil {
		return err
	}
	if status == 404 {
		return downloader.ErrNotImplemented
	}
	if status != 200 && status != 202 {
		return fmt.Errorf("set tags status %d", status)
	}
	return nil
}

func (c *Client) AddTags(ctx context.Context, hashes []string, tags []string) error {
	return c.SetTags(ctx, hashes, tags)
}

func (c *Client) RemoveTags(ctx context.Context, hashes []string, tags []string) error {
	return downloader.ErrNotImplemented
}

func (c *Client) CreateCategory(ctx context.Context, name, savePath string) error {
	return downloader.ErrNotImplemented
}
func (c *Client) EditCategory(ctx context.Context, name, savePath string) error {
	return downloader.ErrNotImplemented
}
func (c *Client) DeleteCategories(ctx context.Context, names []string) error {
	return downloader.ErrNotImplemented
}

func (c *Client) SetShareLimits(ctx context.Context, hashes []string, limits downloader.ShareLimits) error {
	return downloader.ErrNotImplemented
}

func (c *Client) SetQueuePriority(ctx context.Context, hashes []string, action string) error {
	prio := 2
	switch action {
	case "top", "up":
		prio = 3
	case "down":
		prio = 2
	case "bottom":
		prio = 1
	default:
		return fmt.Errorf("invalid queue action: %s", action)
	}
	status, _, err := c.raw(ctx, http.MethodPost, "/api/torrents/priority",
		mustJSON(map[string]any{"hashes": upperAll(hashes), "priority": prio}), "application/json", true)
	if err != nil {
		return err
	}
	if status == 404 {
		return downloader.ErrNotImplemented
	}
	if status != 200 && status != 202 {
		return fmt.Errorf("priority status %d", status)
	}
	return nil
}

func (c *Client) ToggleSequentialDownload(ctx context.Context, hashes []string) error {
	return downloader.ErrNotImplemented
}
func (c *Client) ToggleFirstLastPiece(ctx context.Context, hashes []string) error {
	return downloader.ErrNotImplemented
}

func (c *Client) SetLimits(ctx context.Context, hashes []string, limits downloader.Limits) error {
	return downloader.ErrNotImplemented
}
func (c *Client) GetSpeedLimits(ctx context.Context) (*downloader.Limits, error) {
	return nil, downloader.ErrNotImplemented
}
func (c *Client) SetSpeedLimits(ctx context.Context, limits downloader.Limits) error {
	return downloader.ErrNotImplemented
}

func (c *Client) ListFiles(ctx context.Context, hash string) ([]downloader.TorrentFile, error) {
	status, b, err := c.raw(ctx, http.MethodGet, "/api/torrents/"+strings.ToUpper(hash)+"/contents", nil, "", true)
	if err != nil {
		return nil, err
	}
	if status == 404 {
		return nil, downloader.ErrNotImplemented
	}
	if status != 200 {
		return nil, fmt.Errorf("files status %d", status)
	}
	var files []struct {
		Index    int     `json:"index"`
		Path     string  `json:"path"`
		Filename string  `json:"filename"`
		Size     int64   `json:"sizeBytes"`
		Progress float64 `json:"percentComplete"`
		Priority int     `json:"priority"`
	}
	if err := json.Unmarshal(b, &files); err != nil {
		return nil, err
	}
	out := make([]downloader.TorrentFile, 0, len(files))
	for _, f := range files {
		name := f.Path
		if name == "" {
			name = f.Filename
		}
		prog := f.Progress
		if prog > 1 {
			prog /= 100
		}
		out = append(out, downloader.TorrentFile{
			Index: f.Index, Name: name, Size: f.Size, Progress: prog, Priority: f.Priority,
		})
	}
	return out, nil
}

func (c *Client) SetFilePriority(ctx context.Context, hash string, indices []int, priority int) error {
	status, _, err := c.raw(ctx, http.MethodPatch, "/api/torrents/"+strings.ToUpper(hash)+"/contents",
		mustJSON(map[string]any{"indices": indices, "priority": priority}), "application/json", true)
	if err != nil {
		return err
	}
	if status == 404 {
		return downloader.ErrNotImplemented
	}
	if status != 200 && status != 202 {
		return fmt.Errorf("set file prio status %d", status)
	}
	return nil
}

func (c *Client) ListPeers(ctx context.Context, hash string) ([]downloader.Peer, error) {
	status, b, err := c.raw(ctx, http.MethodGet, "/api/torrents/"+strings.ToUpper(hash)+"/peers", nil, "", true)
	if err != nil {
		return nil, err
	}
	if status == 404 {
		return nil, downloader.ErrNotImplemented
	}
	if status != 200 {
		return nil, fmt.Errorf("peers status %d", status)
	}
	var peers []struct {
		Address string `json:"address"`
		Client  string `json:"clientVersion"`
		DlRate  int64  `json:"downloadRate"`
		UpRate  int64  `json:"uploadRate"`
	}
	if err := json.Unmarshal(b, &peers); err != nil {
		return nil, downloader.ErrNotImplemented
	}
	out := make([]downloader.Peer, 0, len(peers))
	for _, p := range peers {
		out = append(out, downloader.Peer{IP: p.Address, Client: p.Client, DLRate: p.DlRate, UPRate: p.UpRate})
	}
	return out, nil
}

func (c *Client) ListTrackers(ctx context.Context, hash string) ([]downloader.Tracker, error) {
	status, b, err := c.raw(ctx, http.MethodGet, "/api/torrents/"+strings.ToUpper(hash)+"/trackers", nil, "", true)
	if err != nil {
		return nil, err
	}
	if status == 404 {
		return nil, downloader.ErrNotImplemented
	}
	if status != 200 {
		return nil, fmt.Errorf("trackers status %d", status)
	}
	var tr []struct {
		URL  string `json:"url"`
		Tier int    `json:"tier"`
	}
	if err := json.Unmarshal(b, &tr); err != nil {
		return nil, err
	}
	out := make([]downloader.Tracker, 0, len(tr))
	for _, t := range tr {
		out = append(out, downloader.Tracker{URL: t.URL, Tier: t.Tier})
	}
	return out, nil
}

func (c *Client) AddTrackers(ctx context.Context, hash string, urls []string) error {
	return downloader.ErrNotImplemented
}
func (c *Client) RemoveTrackers(ctx context.Context, hash string, urls []string) error {
	return downloader.ErrNotImplemented
}
