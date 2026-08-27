package chain

import (
	"testing"

	"github.com/hxpt/ptpatronus/internal/transfer"
)

// 回归：云盘路径映射曾用 Rel(DestDir, file) 只保留「季/文件名」，
// 不同影片在 CloudBase 根下互相覆盖（movie.nfo 塌平、Season 1 互撞）。
// 现应从文件绝对路径剥掉库根（MovieDir/TVDir），保留 Title (Year)/Season xx/ 结构。
func TestCloudPathForKeepsLibraryStructure(t *testing.T) {
	cu := transfer.CloudUpload{
		CloudBase: "/115/media",
		MovieDir:  "/media/movies",
		TVDir:     "/media/tv",
	}
	cases := []struct {
		name    string
		destDir string
		file    string
		want    string
	}{
		{
			name:    "电影：保留片目录层",
			destDir: `/media/movies/movie/Inception (2010)`,
			file:    `/media/movies/movie/Inception (2010)/Inception (2010) - 1080P.mkv`,
			want:    "/115/media/movie/Inception (2010)/Inception (2010) - 1080P.mkv",
		},
		{
			name:    "剧集：保留剧根与季目录",
			destDir: `/media/tv/tv/庆余年 (2019)`,
			file:    `/media/tv/tv/庆余年 (2019)/Season 1/庆余年 (2019) - S01E02.mp4`,
			want:    "/115/media/tv/庆余年 (2019)/Season 1/庆余年 (2019) - S01E02.mp4",
		},
		{
			name:    "Windows 反斜杠路径",
			destDir: `D:\media\tv\tv\Show (2020)`,
			file:    `D:\media\tv\tv\Show (2020)\Season 2\Show - S02E01.mkv`,
			want:    "/115/media/tv/Show (2020)/Season 2/Show - S02E01.mkv",
		},
		{
			name:    "附属文件（nfo）同样保留片目录结构",
			destDir: `/media/movies/movie/Movie (1999)`,
			file:    `/media/movies/movie/Movie (1999)/movie.nfo`,
			want:    "/115/media/movie/Movie (1999)/movie.nfo",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cloudPathFor(cu, tc.destDir, tc.file)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("cloudPathFor = %q, want %q", got, tc.want)
			}
		})
	}
}

// 库前缀对不上（如路径映射后的盘符差异）时回退 DestDir 相对路径，标题层仍不丢。
func TestCloudPathForFallsBackToDestDir(t *testing.T) {
	cu := transfer.CloudUpload{CloudBase: "/", MovieDir: "/mnt/remote/movies", TVDir: "/mnt/remote/tv"}
	got, err := cloudPathFor(cu, `/data/library/tv/Foo (2021)`, `/data/library/tv/Foo (2021)/S01E01.mkv`)
	if err != nil {
		t.Fatal(err)
	}
	want := "/Foo (2021)/S01E01.mkv"
	if got != want {
		t.Errorf("cloudPathFor fallback = %q, want %q", got, want)
	}
}
