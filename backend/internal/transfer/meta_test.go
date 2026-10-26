package transfer

import "testing"

func TestParseMeta_TVSxxExx(t *testing.T) {
	m := ParseMeta("Breaking.Bad.S01E02.1080p.BluRay.x264-GROUP.mkv")
	if m.Season != 1 || m.Episode != 2 {
		t.Fatalf("season/episode: got S%dE%d", m.Season, m.Episode)
	}
	if !m.IsTV {
		t.Fatal("should be TV")
	}
	if m.Resolution != "1080P" {
		t.Fatalf("resolution: got %q", m.Resolution)
	}
	if m.ReleaseGroup != "GROUP" {
		t.Fatalf("group: got %q", m.ReleaseGroup)
	}
	if m.SeasonEpisode != "S01E02" {
		t.Fatalf("seasonEpisode: got %q", m.SeasonEpisode)
	}
	if m.Title == "" {
		t.Fatal("title empty")
	}
}

func TestParseMeta_4KNormalized(t *testing.T) {
	m := ParseMeta("Inception.2010.2160p.UHD.BluRay.mkv")
	if m.Resolution != "2160P" {
		t.Fatalf("expected 2160P, got %q", m.Resolution)
	}
	if m.Year != 2010 {
		t.Fatalf("year: got %d", m.Year)
	}
}

func TestParseMeta_SeasonPack(t *testing.T) {
	m := ParseMeta("The.Office.S05.720p.mkv")
	if m.Season != 5 || m.Episode != 0 || !m.IsTV {
		t.Fatalf("season pack: got S%dE%d tv=%v", m.Season, m.Episode, m.IsTV)
	}
	if m.SeasonEpisode != "S05" {
		t.Fatalf("seasonEpisode: got %q", m.SeasonEpisode)
	}
}

func TestParseMeta_ResolutionFallsBackFromCodec(t *testing.T) {
	m := ParseMeta("Movie.2020.HEVC.x265.mkv")
	if m.Resolution != "" {
		t.Fatalf("codec should not count as resolution, got %q", m.Resolution)
	}
}

// 动漫常见「标题 - 12」绝对集数 + [fansub] 组名：从前既不识别集数，
// 标题还会残留 "[SubsPlease] ... ( ) [ABCD1234]"，按它去搜片必然搜不到。
func TestParseMeta_AnimeAbsoluteEpisode(t *testing.T) {
	m := ParseMeta("[SubsPlease] Sousou no Frieren - 12 (1080p) [ABCD1234].mkv")
	if !m.IsTV || m.Episode != 12 {
		t.Fatalf("absolute episode: got tv=%v E%d", m.IsTV, m.Episode)
	}
	if m.Title != "Sousou no Frieren" {
		t.Fatalf("title: got %q", m.Title)
	}
	if m.ReleaseGroup != "SubsPlease" {
		t.Fatalf("leading fansub tag should be the group, got %q", m.ReleaseGroup)
	}
	if m.Resolution != "1080P" {
		t.Fatalf("resolution: got %q", m.Resolution)
	}

	zh := ParseMeta("[ANi] 葬送的芙莉莲 - 12 [1080P][Baha][WEB-DL][AAC AVC][CHT].mp4")
	if !zh.IsTV || zh.Episode != 12 {
		t.Fatalf("中文动漫 absolute episode: got tv=%v E%d", zh.IsTV, zh.Episode)
	}
	if zh.Title != "葬送的芙莉莲" {
		t.Fatalf("中文 title 残留标签: got %q", zh.Title)
	}
	if zh.ReleaseGroup != "ANi" {
		t.Fatalf("group: got %q", zh.ReleaseGroup)
	}
}

// 正控：标题里的「- 数字」后接年份不算集数（不能把电影误判成剧集）。
func TestParseMeta_DashNumberInMovieTitleStaysMovie(t *testing.T) {
	m := ParseMeta("Death.Race.2.2008.1080p.BluRay.x264-GROUP")
	if m.IsTV || m.Episode != 0 {
		t.Fatalf("movie should stay movie, got tv=%v E%d", m.IsTV, m.Episode)
	}
	if m.Year != 2008 {
		t.Fatalf("year: got %d", m.Year)
	}
}

func TestShouldProcess_Filter(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.ShouldProcess("subscribe", nil) {
		t.Fatal("subscribe should pass")
	}
	if cfg.ShouldProcess("movies", []string{"brushflow:3"}) {
		t.Fatal("brushflow tag must be skipped")
	}
	if cfg.ShouldProcess("movies", []string{"reseed"}) {
		t.Fatal("reseed tag must be skipped")
	}
	if !cfg.ShouldProcess("movies", []string{"organize"}) {
		t.Fatal("organize tag should force pass")
	}
	cfg.Enabled = false
	if cfg.ShouldProcess("subscribe", nil) {
		t.Fatal("disabled must skip all")
	}
}

func TestRenderName_Movie(t *testing.T) {
	cfg := DefaultConfig()
	got := renderName(cfg.MovieRename, namingCtx{Title: "Inception", Year: 2010, Resolution: "1080P", ReleaseGroup: "ESPA"})
	if got != "Inception (2010) - 1080P - ESPA" {
		t.Fatalf("movie rename: got %q", got)
	}
}

func TestRenderName_TVEpisode(t *testing.T) {
	cfg := DefaultConfig()
	got := renderName(cfg.TVRename, namingCtx{Title: "Breaking Bad", Year: 2008, SeasonEpisode: "S01E02", Resolution: "1080P"})
	if got != "Breaking Bad (2008) - S01E02 - 1080P" {
		t.Fatalf("tv rename: got %q", got)
	}
}
