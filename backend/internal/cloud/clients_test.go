package cloud

import (
	"context"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// fakeDoer 按路径子串匹配返回预设 JSON（模拟网盘 REST 响应）。
type fakeDoer struct {
	bodies map[string]string // url 子串 → 响应体
	calls  []string          // 记录请求 url（cookie/header 断言用）
}

func (f *fakeDoer) Do(_ context.Context, method, rawURL string, body []byte,
	headers map[string]string, cookieStr string, ssrf bool) (*httpclient.Response, error) {
	f.calls = append(f.calls, rawURL)
	for sub, b := range f.bodies {
		if strings.Contains(rawURL, sub) {
			return &httpclient.Response{StatusCode: 200, Body: []byte(b)}, nil
		}
	}
	return &httpclient.Response{StatusCode: 200, Body: []byte(`{"state":true,"data":[]}`)}, nil
}

func TestClient115ListRoot(t *testing.T) {
	doer := &fakeDoer{bodies: map[string]string{
		"cid=0": `{"state":true,"data":[
			{"file_name":"电影","cid":"100"},
			{"file_name":"音乐","cid":"101"},
			{"file_name":"Inception.mkv","sha1":"abc","file_size":"1234567","fid":"9","pickcode":"pc1"}
		]}`,
	}}
	c := &client115{drive: Drive{Type: "115", Token: "UID=1; CID=2"}, hc: doer}
	files, err := c.List(context.Background(), "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("want 3 files, got %d: %+v", len(files), files)
	}
	// 前两个目录，第三个文件
	if !files[0].IsDir || files[0].Name != "电影" {
		t.Errorf("files[0] = %+v", files[0])
	}
	if files[2].IsDir || files[2].Size != 1234567 || files[2].PickCode != "pc1" {
		t.Errorf("file item wrong: %+v", files[2])
	}
	if files[2].Path != "/Inception.mkv" {
		t.Errorf("path wrong: %q", files[2].Path)
	}
	// cookie 必须随请求携带（鉴权）
	if !strings.Contains(doer.calls[0], "webapi.115.com/files") {
		t.Errorf("unexpected call: %s", doer.calls[0])
	}
}

func TestClient115ResolvePath(t *testing.T) {
	doer := &fakeDoer{bodies: map[string]string{
		"cid=0":   `{"state":true,"data":[{"file_name":"电影","cid":"100"}]}`,
		"cid=100": `{"state":true,"data":[{"file_name":"A.mkv","sha1":"x","file_size":"10","fid":"91"}]}`,
	}}
	c := &client115{drive: Drive{Type: "115"}, hc: doer}
	files, err := c.List(context.Background(), "/电影")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Name != "A.mkv" || files[0].Path != "/电影/A.mkv" {
		t.Errorf("resolve+list wrong: %+v", files)
	}
}

func TestClient115ResolvePathNotFound(t *testing.T) {
	doer := &fakeDoer{bodies: map[string]string{
		"cid=0": `{"state":true,"data":[{"file_name":"电影","cid":"100"}]}`,
	}}
	c := &client115{drive: Drive{Type: "115"}, hc: doer}
	if _, err := c.List(context.Background(), "/不存在"); err == nil {
		t.Error("want error for missing path")
	}
}

func TestClientQuarkList(t *testing.T) {
	doer := &fakeDoer{bodies: map[string]string{
		"pdir_fid=0": `{"code":0,"message":"ok","data":{"list":[
			{"fid":"f1","file_name":"剧集","file_type":0,"size":"0"},
			{"fid":"f2","file_name":"B.mkv","file_type":1,"size":"5000"}
		]}}`,
	}}
	c := &clientQuark{drive: Drive{Type: "quark", Token: "__puus=x"}, hc: doer}
	files, err := c.List(context.Background(), "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("want 2, got %d", len(files))
	}
	if !files[0].IsDir || files[0].Name != "剧集" {
		t.Errorf("dir item wrong: %+v", files[0])
	}
	if files[1].IsDir || files[1].Size != 5000 || files[1].PickCode != "f2" {
		t.Errorf("file item wrong: %+v", files[1])
	}
}

func TestClient123List(t *testing.T) {
	var seenAuth string
	doer := &fakeDoer{bodies: map[string]string{
		"parentFileId=0": `{"code":0,"message":"ok","data":{"InfoList":[
			{"FileId":11,"FileName":"电影","Type":0,"Size":"0"},
			{"FileId":22,"FileName":"C.mp4","Type":1,"Size":"8000"}
		]}}`,
	}}
	// 包一层捕获 Authorization 头
	wrap := &authCapture{inner: doer, auth: &seenAuth}
	c := &client123{drive: Drive{Type: "123", Token: "tok123"}, hc: wrap}
	files, err := c.List(context.Background(), "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || !files[0].IsDir || files[1].Size != 8000 {
		t.Errorf("list wrong: %+v", files)
	}
	if seenAuth != "Bearer tok123" {
		t.Errorf("123 should send Bearer token, got %q", seenAuth)
	}
}

// authCapture 捕获 Authorization 头（123 用 token 非 cookie）。
type authCapture struct {
	inner *fakeDoer
	auth  *string
}

func (a *authCapture) Do(ctx context.Context, method, rawURL string, body []byte,
	headers map[string]string, cookieStr string, ssrf bool) (*httpclient.Response, error) {
	if h, ok := headers["Authorization"]; ok {
		*a.auth = h
	}
	return a.inner.Do(ctx, method, rawURL, body, headers, cookieStr, ssrf)
}
