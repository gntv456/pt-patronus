// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package torznab

import (
	"strings"
	"testing"
)

func TestParseRSS(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:torznab="http://torznab.com/schemas/2015/feed">
  <channel>
    <item>
      <title>Foo.2020.1080p</title>
      <link>https://x/dl/1</link>
      <guid>1</guid>
      <enclosure url="https://x/dl/1.torrent" length="1234567890" type="application/x-bittorrent"/>
      <torznab:attr name="seeders" value="12"/>
      <torznab:attr name="imdb" value="tt1234567"/>
    </item>
  </channel>
</rss>`
	rs, err := parseRSS(strings.NewReader(xml))
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 {
		t.Fatalf("len=%d", len(rs))
	}
	if rs[0].Title != "Foo.2020.1080p" || rs[0].Size != 1234567890 || rs[0].ImdbID != "tt1234567" {
		t.Fatalf("%+v", rs[0])
	}
	if rs[0].Link != "https://x/dl/1.torrent" {
		t.Fatalf("link=%s", rs[0].Link)
	}
}

func TestExtractImdbID(t *testing.T) {
	if got := ExtractImdbID("Movie.tt7654321.1080p"); got != "tt7654321" {
		t.Fatalf("got %q", got)
	}
	if got := ExtractImdbID("no id here"); got != "" {
		t.Fatalf("got %q", got)
	}
}
