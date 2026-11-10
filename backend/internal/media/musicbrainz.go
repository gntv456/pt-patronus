// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package media

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const (
	mbAPIBase  = "https://musicbrainz.org/ws/2/release-group"
	mbCoverURL = "https://coverartarchive.org/release-group/%s/front"
)

// MusicBrainz 是音乐封面源（TMDB 无音乐数据时的 fallback）。
type MusicBrainz struct {
	hc *http.Client
}

// NewMusicBrainz 构造。
func NewMusicBrainz() *MusicBrainz {
	return &MusicBrainz{hc: &http.Client{Timeout: 15 * time.Second}}
}

func (m *MusicBrainz) Name() string { return "musicbrainz" }

// Types 实现 TypedSource：MusicBrainz 只产音乐（影视搜索 movie/tv 时会被 Chain 跳过）。
func (m *MusicBrainz) Types() []MediaType { return []MediaType{TypeMusic} }

type mbResult struct {
	ReleaseGroups []struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		FirstRel    string `json:"first-release-date"`
		PrimaryType string `json:"primary-type"`
	} `json:"release-groups"`
}

// Search 按 release-group 搜索音乐。
func (m *MusicBrainz) Search(ctx context.Context, keyword string, year int) ([]MediaInfo, error) {
	u, _ := url.Parse(mbAPIBase)
	q := u.Query()
	q.Set("query", keyword)
	q.Set("fmt", "json")
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "PTPatronus/1.0 ( https://github.com/hxpt/ptpatronus )")
	resp, err := m.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("musicbrainz: status %d", resp.StatusCode)
	}
	var r mbResult
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	out := make([]MediaInfo, 0, len(r.ReleaseGroups))
	for _, rg := range r.ReleaseGroups {
		mi := MediaInfo{
			Source: "musicbrainz", SourceID: rg.ID,
			Title: rg.Title, Type: TypeMusic, ReleaseDate: rg.FirstRel,
			PosterURL: fmt.Sprintf(mbCoverURL, rg.ID),
		}
		mi.Year = yearFromDate(rg.FirstRel)
		if year > 0 && mi.Year > 0 && abs(mi.Year-year) > 1 {
			continue
		}
		out = append(out, mi)
	}
	return out, nil
}

// Detail 取音乐详情（MBID）。MusicBrainz 无单条详情海报，用 Cover Art。
func (m *MusicBrainz) Detail(ctx context.Context, id string) (*MediaInfo, error) {
	return &MediaInfo{
		Source: "musicbrainz", SourceID: id, Type: TypeMusic,
		PosterURL: fmt.Sprintf(mbCoverURL, id),
	}, nil
}
