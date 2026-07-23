package assessment

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

func TestSunnyptLiveAssessments(t *testing.T) {
	ckPath := filepath.Clean(`D:/PTPatronus/tmp/sunny_cookie.txt`)
	b, err := os.ReadFile(ckPath)
	if err != nil || len(b) < 20 {
		t.Skip("no local sunny cookie")
	}
	hc, _ := httpclient.New(httpclient.Options{Timeout: 25 * time.Second, AllowPrivate: true})
	ad := SunnyptAdapter{}
	tasks, err := ad.FetchAssessments(context.Background(), liveHC{hc}, Def{ID: "sunnypt", Schema: "tnode"}, "https://sunnypt.top", string(b), "10792")
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	t.Logf("tasks=%d", len(tasks))
}

type liveHC struct{ c *httpclient.Client }

func (a liveHC) Get(ctx context.Context, rawURL, cookieStr string, ssrf bool) (*Response, error) {
	r, err := a.c.Get(ctx, rawURL, cookieStr, ssrf)
	if err != nil {
		return nil, err
	}
	return &Response{StatusCode: r.StatusCode, Body: r.Body, FinalURL: r.FinalURL}, nil
}
func (a liveHC) Do(ctx context.Context, method, rawURL string, body []byte, headers map[string]string, cookie string, ssrf bool) (*Response, error) {
	r, err := a.c.Do(ctx, method, rawURL, body, headers, cookie, ssrf)
	if err != nil {
		return nil, err
	}
	return &Response{StatusCode: r.StatusCode, Body: r.Body, FinalURL: r.FinalURL}, nil
}
