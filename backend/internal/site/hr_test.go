// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

import (
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
)

func TestParseHRDuration(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"2天03:04:05", 2*24*time.Hour + 3*time.Hour + 4*time.Minute + 5*time.Second},
		{"2day(s)03:04:05", 2*24*time.Hour + 3*time.Hour + 4*time.Minute + 5*time.Second},
		{"3:04:05", 3*time.Hour + 4*time.Minute + 5*time.Second},
		{"4:05", 4*time.Minute + 5*time.Second},
		{"12小时30分", 12*time.Hour + 30*time.Minute},
		{"1h 30m 5s", time.Hour + 30*time.Minute + 5*time.Second},
		{"---", 0},
		{"", 0},
	}
	for _, c := range cases {
		got := ParseHRDuration(c.in)
		if got != c.want {
			t.Errorf("ParseHRDuration(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestIsHRDangerous(t *testing.T) {
	day := 24 * time.Hour
	cases := []struct {
		name      string
		t         HRTorrent
		warn      time.Duration
		dangerous bool
	}{
		{"unreached", HRTorrent{Status: "未达标"}, day, true},
		{"reached", HRTorrent{Status: "已达标"}, day, false},
		{"pardoned", HRTorrent{Status: "豁免"}, day, false},
		{"inspecting near deadline", HRTorrent{Status: "监控中", DeadlineLeft: "10:00:00"}, day, true},
		{"inspecting far deadline", HRTorrent{Status: "监控中", DeadlineLeft: "2天00:00:00"}, day, false},
		{"inspecting no deadline", HRTorrent{Status: "监控中"}, day, false},
		{"warn zero no time alert", HRTorrent{Status: "监控中", DeadlineLeft: "1:00:00"}, 0, false},
		{"english unreached", HRTorrent{Status: "Unreached"}, day, true},
	}
	for _, c := range cases {
		if got := IsHRDangerous(c.t, c.warn); got != c.dangerous {
			t.Errorf("%s: got %v, want %v", c.name, got, c.dangerous)
		}
	}
}

func TestParseHR(t *testing.T) {
	html := `<table><tr><th>标题</th></tr>
		<tr>
		  <td><a href="details.php?id=42">Some Torrent</a></td>
		  <td>1.5 GB</td><td>1.2</td><td>12:00:00</td><td>监控中</td><td>10:00:00</td>
		</tr></table>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	def := &SiteMetadata{
		HR: HRDef{
			Enabled: true,
			Rows:    ElementQuery{Selector: "table tr"},
			Fields: map[string]ElementQuery{
				"title":    {Selector: "a"},
				"detail":   {Selector: "a", Attr: "href"},
				"size":     {Selector: "td:nth-child(2)"},
				"ratio":    {Selector: "td:nth-child(3)"},
				"seedtime": {Selector: "td:nth-child(4)"},
				"status":   {Selector: "td:nth-child(5)"},
				"deadline": {Selector: "td:nth-child(6)"},
			},
		},
	}
	got := ParseHR(doc, def, "https://x.example")
	if len(got) != 1 {
		t.Fatalf("expected 1 HR torrent, got %d", len(got))
	}
	hr := got[0]
	if hr.Title != "Some Torrent" || hr.Status != "监控中" || hr.Ratio != 1.2 {
		t.Errorf("unexpected HR: %+v", hr)
	}
	if hr.DetailURL != "https://x.example/details.php?id=42" {
		t.Errorf("detail url = %q", hr.DetailURL)
	}
	if ParseHRDuration(hr.DeadlineLeft) != 10*time.Hour {
		t.Errorf("deadline parse = %v", ParseHRDuration(hr.DeadlineLeft))
	}
}
