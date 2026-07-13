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
