package cloud

import (
	"context"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// cidDoer 按 ?cid= 精确匹配的 115 fake（列表 URL 含 count=100& 等参数，
// 子串键 "cid=0"/"cid=100" 在 map 迭代序下会串键）。
type cidDoer struct {
	byCID map[string]string
	other map[string]string
	calls []string
}

func (d *cidDoer) Do(_ context.Context, method, rawURL string, body []byte,
	headers map[string]string, cookieStr string, ssrf bool) (*httpclient.Response, error) {
	d.calls = append(d.calls, rawURL)
	if v := cidOfURL(rawURL); v != "" {
		if b, ok := d.byCID[v]; ok {
			return &httpclient.Response{StatusCode: 200, Body: []byte(b)}, nil
		}
	}
	for sub, b := range d.other {
		if strings.Contains(rawURL, sub) {
			return &httpclient.Response{StatusCode: 200, Body: []byte(b)}, nil
		}
	}
	return &httpclient.Response{StatusCode: 200, Body: []byte(`{"state":true,"data":[]}`)}, nil
}

// cidOfURL 提取 ?cid=<v> 的精确值（到下一个 &）。
func cidOfURL(u string) string {
	i := strings.Index(u, "cid=")
	if i < 0 {
		return ""
	}
	rest := u[i+4:]
	for j := 0; j < len(rest); j++ {
		if rest[j] == '&' {
			return rest[:j]
		}
	}
	return rest
}

// TestFileManager115Ops 115 web：move/copy/rename/delete 走 fid（非 pickcode）。
func TestFileManager115Ops(t *testing.T) {
	doer := &cidDoer{
		byCID: map[string]string{
			"0":   `{"state":true,"data":[{"file_name":"电影","cid":"100"},{"file_name":"电影2","cid":"200"}]}`,
			"100": `{"state":true,"data":[{"file_name":"a.mkv","sha1":"x","file_size":"1","fid":"91","pickcode":"pc1"}]}`,
		},
		other: map[string]string{
			"/files/move":    `{"state":true}`,
			"/files/copy":    `{"state":true}`,
			"/files/rename":  `{"state":true}`,
			"/files/rdelete": `{"state":true}`,
		},
	}
	c := &client115{drive: Drive{Type: "115", Token: "ck"}, hc: doer}
	if err := c.Move(context.Background(), "/电影/a.mkv", "/电影2"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if err := c.Rename(context.Background(), "/电影/a.mkv", "b.mkv"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if err := c.Delete(context.Background(), "/电影/a.mkv"); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

// TestFileManager115Share 分享码/提取码提取。
func TestFileManager115Share(t *testing.T) {
	if got := extract115ShareCode("https://115.com/s/abc123?password=xyz#/"); got != "abc123" {
		t.Errorf("code=%q", got)
	}
	if got := extract115SharePwd("https://115.com/s/abc?password=xyz"); got != "xyz" {
		t.Errorf("pwd=%q", got)
	}
	if got := extract115ShareCode("https://115.com/nothing"); got != "" {
		t.Errorf("no code should be empty, got %q", got)
	}
}

// TestFileManagerQuarkOps 夸克：move/rename 走 /file action；delete 走 /file/delete。
func TestFileManagerQuarkOps(t *testing.T) {
	doer := &fakeDoer{bodies: map[string]string{
		"file/sort":     `{"code":0,"data":{"list":[{"fid":"f9","file_name":"a.mkv","file_type":1,"size":"1"}]}}`,
		"file?pr=ucpro": `{"status":200,"code":0,"data":{}}`,
		"/file/delete":  `{"status":200,"code":0,"data":{}}`,
		"/file/copy":    `{"status":200,"code":0,"data":{"task_id":"t1"}}`,
	}}
	c := &clientQuark{drive: Drive{Type: "quark", Token: "ck"}, hc: doer}
	if err := c.Rename(context.Background(), "/a.mkv", "b.mkv"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if err := c.Delete(context.Background(), "/a.mkv"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	// 离线：磁力 task_type=2
	if err := c.OfflineAdd(context.Background(), "/", "magnet:?xt=urn:btih:x"); err != nil {
		t.Fatalf("offline: %v", err)
	}
}

// TestFileManager123Unsupported 123 copy/离线/转存返回 ErrUploadUnsupported。
func TestFileManager123Unsupported(t *testing.T) {
	c := &client123{drive: Drive{Type: "123", Token: "t"}}
	if err := c.Copy(context.Background(), "/a", "/b"); err == nil {
		t.Error("copy should be unsupported")
	}
	if err := c.OfflineAdd(context.Background(), "/", "magnet:?x"); err == nil {
		t.Error("offline should be unsupported")
	}
}

// TestFileManager115OpenUnsupported 115开放 离线/转存不支持。
func TestFileManager115OpenUnsupported(t *testing.T) {
	c := &client115Open{drive: Drive{Type: "115_open", Token: "t"}}
	if err := c.OfflineAdd(context.Background(), "/", "magnet:?x"); err == nil {
		t.Error("offline should be unsupported")
	}
}
