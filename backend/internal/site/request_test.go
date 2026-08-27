package site

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// TestParseRequests 求种列表解析：链接行提取 id/标题/已满足标记/票数。
func TestParseRequests(t *testing.T) {
	html := `<html><body><table>
	<tr><td><a href="viewsrequests.php?id=123">某电影 2020 4K</a> (5)</td><td>outstanding</td></tr>
	<tr><td><a href="viewsrequests.php?id=456">某剧 S01</a> (2)</td><td>Fulfilled</td></tr>
	<tr><td>无关行（无链接）</td></tr>
	</table></body></html>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	reqs := ParseRequests(doc)
	if len(reqs) != 2 {
		t.Fatalf("expect 2 requests, got %d", len(reqs))
	}
	if reqs[0].ID != "123" || reqs[0].Title != "某电影 2020 4K" || reqs[0].Filled {
		t.Fatalf("bad first: %+v", reqs[0])
	}
	if !reqs[1].Filled {
		t.Fatalf("fulfilled row not detected: %+v", reqs[1])
	}
	if reqs[0].Votes != 5 {
		t.Fatalf("votes not parsed: %+v", reqs[0])
	}
}

// TestParseRequestsEmpty 空页面/无求种行 → nil。
func TestParseRequestsEmpty(t *testing.T) {
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(`<html><body></body></html>`))
	if reqs := ParseRequests(doc); reqs != nil {
		t.Fatalf("expect nil, got %+v", reqs)
	}
	if ParseRequests(nil) != nil {
		t.Fatal("nil doc must return nil")
	}
}

// TestParamFromHref href 参数提取（含截断）。
func TestParamFromHref(t *testing.T) {
	if got := paramFromHref("viewsrequests.php?id=99&sort=1", "id"); got != "99" {
		t.Fatalf("got %q", got)
	}
	if got := paramFromHref("details.php?id=1", "id"); got != "1" {
		t.Fatalf("got %q", got)
	}
	if got := paramFromHref("nokey.php?x=1", "id"); got != "" {
		t.Fatalf("expect empty, got %q", got)
	}
}

// TestBetween 片段截取。
func TestBetween(t *testing.T) {
	if got := between("标题 (12) 其它", "标题", ")"); got != " (12" {
		t.Fatalf("got %q", got)
	}
	if got := between("abc", "zzz", ")"); got != "" {
		t.Fatalf("missing start must be empty, got %q", got)
	}
}
