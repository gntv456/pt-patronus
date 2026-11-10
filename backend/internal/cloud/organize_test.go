// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package cloud

import (
	"context"
	"strings"
	"testing"
)

// orgFake 识别桩：按关键字返回 movie/tv。
type orgFake struct{}

func (orgFake) Recognize(_ context.Context, name string) (*OrganizeMeta, error) {
	switch {
	case strings.Contains(name, "Dune"):
		return &OrganizeMeta{Title: "沙丘", Year: 2021, Type: "movie"}, nil
	case strings.Contains(name, "S01E02"):
		return &OrganizeMeta{Title: "庆余年", Type: "tv", Season: "S01", Episodes: []string{"S01E02"}}, nil
	}
	return nil, nil // 未识别
}

// fmFake 文件管理桩（记录 rename/move 调用）。
type fmFake struct {
	renames []string
	moves   [][2]string // src → dstDir
}

func (f *fmFake) Mkdir(context.Context, string, string) error { return nil }
func (f *fmFake) Move(_ context.Context, src, dst string) error {
	f.moves = append(f.moves, [2]string{src, dst})
	return nil
}
func (f *fmFake) Copy(context.Context, string, string) error { return nil }
func (f *fmFake) Rename(_ context.Context, src, new string) error {
	f.renames = append(f.renames, src+"→"+new)
	return nil
}
func (f *fmFake) Delete(context.Context, string) error                    { return nil }
func (f *fmFake) OfflineAdd(context.Context, string, string) error        { return nil }
func (f *fmFake) SaveShare(context.Context, string, string, string) error { return nil }

// listFake Client.List 桩。
type listFake struct{ fmFake }

func (listFake) Type() string                          { return "fake" }
func (listFake) Test(context.Context) (string, error)  { return "", nil }
func (listFake) Quota(context.Context) (*Quota, error) { return &Quota{}, nil }
func (l *listFake) List(_ context.Context, dir string) ([]File, error) {
	if dir == "/待整理" {
		return []File{
			{Name: "Dune.2021.1080p.mkv", Path: "/待整理/Dune.2021.1080p.mkv", Size: 1},
			{Name: "庆余年.S01E02.1080p.mkv", Path: "/待整理/庆余年.S01E02.1080p.mkv", Size: 1},
			{Name: "poster.jpg", Path: "/待整理/poster.jpg", Size: 1},
			{Name: "random.bin", Path: "/待整理/random.bin", Size: 1},
		}, nil
	}
	return nil, nil
}

func TestCloudOrganize(t *testing.T) {
	lf := &listFake{}
	res, err := Organize(context.Background(), lf, orgFake{}, OrganizeOptions{
		DriveID: 1, SrcDir: "/待整理",
		DstMovieDir: "/媒体库/电影", DstTVDir: "/媒体库/剧集",
		MediaOnly: true, Rename: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Scanned != 4 {
		t.Errorf("scanned=%d want 4", res.Scanned)
	}
	if res.Moved != 2 {
		t.Errorf("moved=%d want 2 (Dune→电影, 庆余年→剧集)", res.Moved)
	}
	if res.Skipped != 2 {
		t.Errorf("skipped=%d want 2 (jpg 非媒体 + random 未识别)", res.Skipped)
	}
	// 目标目录断言
	wantMovie := "/媒体库/电影/沙丘 (2021)"
	wantTV := "/媒体库/剧集/庆余年/S01"
	foundM, foundTV := false, false
	for _, m := range lf.moves {
		if m[1] == wantMovie {
			foundM = true
		}
		if m[1] == wantTV {
			foundTV = true
		}
	}
	if !foundM {
		t.Errorf("movie dest missing: want %s, moves=%v", wantMovie, lf.moves)
	}
	if !foundTV {
		t.Errorf("tv dest missing: want %s, moves=%v", wantTV, lf.moves)
	}
}

func TestOrganizeTarget(t *testing.T) {
	d, n := organizeTarget(OrganizeOptions{DstMovieDir: "/电影"}, &OrganizeMeta{Title: "A/B: 敌", Year: 2020, Type: "movie"}, "x.mkv")
	if d != "/电影/A B  敌 (2020)" || n != "x.mkv" {
		t.Errorf("movie target=%q name=%q", d, n)
	}
	d, _ = organizeTarget(OrganizeOptions{DstTVDir: "/剧集"}, &OrganizeMeta{Title: "剧", Type: "tv", Season: "S02"}, "y.mkv")
	if d != "/剧集/剧/S02" {
		t.Errorf("tv target=%q", d)
	}
	// 未配置目标目录 → 跳过
	if d, _ := organizeTarget(OrganizeOptions{}, &OrganizeMeta{Title: "x", Type: "movie"}, "z"); d != "" {
		t.Errorf("unconfigured should skip, got %q", d)
	}
}
