package schemas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// TestGenericJSONSearch：canned JSON（zhuque 形 rows=data.torrents）→ ParseJSONSearch。
func TestGenericJSONSearch(t *testing.T) {
	var gotParam string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotParam = r.URL.Query().Get("keyword")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"torrents":[{"id":1,"title":"T","size":2000,"seeding":3,"leeching":1,"complete":9}]}}`))
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericJSON(hc, "tnode")
	def := &site.SiteMetadata{
		Schema: "tnode", SearchEntry: "/api/torrent/advancedSearch", SearchParam: "keyword",
		Selectors: site.SiteSelectors{
			Rows: site.ElementQuery{Selector: "data.torrents"},
			Fields: map[string]site.ElementQuery{
				"title":   {Selector: "title"},
				"seeders": {Selector: "seeding"},
			},
		},
	}
	ts, err := g.Search(context.Background(), def, srv.URL, "", "kw", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if gotParam != "kw" {
		t.Errorf("keyword param = %q", gotParam)
	}
	if len(ts) != 1 || ts[0].Title != "T" || ts[0].Seeders != 3 {
		t.Errorf("解析错: %+v", ts)
	}
}
