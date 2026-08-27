package transfer

import "testing"

func TestScoreResolutionDominant(t *testing.T) {
	r := DefaultSortRules()
	ts := []TorrentCandidate{
		{Title: "Movie 720p"}, {Title: "Movie 2160p"}, {Title: "Movie 1080p"},
	}
	got := ScoreTorrents(r, ts)
	if got[0].Candidate.Title != "Movie 2160p" || got[2].Candidate.Title != "Movie 720p" {
		t.Errorf("resolution order wrong: %+v", got)
	}
}

func TestScoreFreeAndSeeders(t *testing.T) {
	r := DefaultSortRules()
	a := TorrentCandidate{Title: "X 1080p", Seeders: 500}            // 70 + 25 = 95
	b := TorrentCandidate{Title: "X 1080p", Free: true, Seeders: 50} // 70 + 10 = 80
	got := ScoreTorrents(r, []TorrentCandidate{b, a})
	if got[0].Candidate != a {
		t.Errorf("more seeders should win: %+v", got)
	}
	// Free 在同做种下胜出
	c := TorrentCandidate{Title: "X 1080p", Seeders: 50}
	got = ScoreTorrents(r, []TorrentCandidate{c, b})
	if got[0].Candidate != b {
		t.Errorf("free tiebreak: %+v", got)
	}
}

func TestScoreSizeCloseness(t *testing.T) {
	r := DefaultSortRules()
	r.TargetSizeGB = 20
	near := TorrentCandidate{Title: "X 1080p", SizeGB: 18}
	far := TorrentCandidate{Title: "X 1080p", SizeGB: 60}
	got := ScoreTorrents(r, []TorrentCandidate{far, near})
	if got[0].Candidate != near {
		t.Errorf("near target size should win: %+v", got)
	}
}

func TestScoreGroupBonus(t *testing.T) {
	r := DefaultSortRules()
	r.GroupBonus = map[string]int{"OurTV": 50, "BadGRP": -100}
	normal := TorrentCandidate{Title: "X 1080p"}
	ours := TorrentCandidate{Title: "X 1080p-OurTV"}
	bad := TorrentCandidate{Title: "X 2160p-BadGRP"} // 2160 100 分 - 100 惩罚 < 1080 70
	got := ScoreTorrents(r, []TorrentCandidate{bad, normal, ours})
	if got[0].Candidate != ours {
		t.Errorf("group bonus: %+v", got)
	}
	if got[2].Candidate != bad {
		t.Errorf("group penalty: %+v", got)
	}
}

func TestDetectResolution(t *testing.T) {
	cases := map[string]string{
		"Movie.2160p.UHD": "2160p", "Show 1080P x264": "1080p",
		"Old 720p": "720p", "nothing here": "unknown", "1080i HDTV": "1080p",
	}
	for in, want := range cases {
		if got := DetectResolution(in); got != want {
			t.Errorf("DetectResolution(%q)=%q want %q", in, got, want)
		}
	}
}

func TestSortRulesRoundtrip(t *testing.T) {
	kv := kvTag{}
	r := DefaultSortRules()
	r.GroupBonus = map[string]int{"G": 9}
	if err := SaveSortRules(kv, r); err != nil {
		t.Fatal(err)
	}
	got := LoadSortRules(kv)
	if got.GroupBonus["G"] != 9 || got.Resolution["1080p"] != 70 {
		t.Errorf("roundtrip: %+v", got)
	}
	// 脏数据回默认
	kv["transfer.sort_rules"] = "{bad"
	got = LoadSortRules(kv)
	if got.Resolution["2160p"] != 100 {
		t.Errorf("bad json should default: %+v", got.Resolution)
	}
}
