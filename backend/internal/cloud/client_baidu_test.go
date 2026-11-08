package cloud

import (
	"context"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// fakeBDHC 按路径子串分发 mock。
type fakeBDHC struct{ responses map[string]string }

func (f *fakeBDHC) Do(ctx context.Context, method, rawURL string, body []byte,
	headers map[string]string, cookieStr string, ssrf bool) (*httpclient.Response, error) {
	for frag, resp := range f.responses {
		if containsStr(rawURL, frag) {
			return &httpclient.Response{StatusCode: 200, Body: []byte(resp)}, nil
		}
	}
	return &httpclient.Response{StatusCode: 404, Body: []byte(`{"errno":2}`)}, nil
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestBaiduList(t *testing.T) {
	hc := &fakeBDHC{responses: map[string]string{
		"method=list": `{"errno":0,"list":[
			{"fs_id":111,"path":"/影视","server_filename":"影视","isdir":1},
			{"fs_id":222,"path":"/影视/a.mkv","server_filename":"a.mkv","isdir":0,"size":4567}
		]}`,
	}}
	c := &clientBaidu{drive: Drive{Token: "t"}, hc: hc}
	files, err := c.List(context.Background(), "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || !files[0].IsDir || files[1].IsDir || files[1].Size != 4567 {
		t.Fatalf("bad list: %+v", files)
	}
	if files[1].PickCode != "222" {
		t.Fatalf("fs_id not captured: %s", files[1].PickCode)
	}
}

func TestBaiduListErrno(t *testing.T) {
	hc := &fakeBDHC{responses: map[string]string{
		"method=list": `{"errno":-6,"list":[]}`,
	}}
	c := &clientBaidu{drive: Drive{Token: "expired"}, hc: hc}
	if _, err := c.List(context.Background(), "/"); err == nil {
		t.Fatal("errno -6 must error")
	}
}

func TestBaiduTestAndQuota(t *testing.T) {
	hc := &fakeBDHC{responses: map[string]string{
		"method=uinfo": `{"errno":0,"baidu_name":"someone","total":214748364800,"used":1024}`,
	}}
	c := &clientBaidu{drive: Drive{Token: "t"}, hc: hc}
	info, err := c.Test(context.Background())
	if err != nil || info == "" {
		t.Fatalf("test: %q %v", info, err)
	}
	q, err := c.Quota(context.Background())
	if err != nil || q.Used != 1024 || q.Total != 214748364800 {
		t.Fatalf("quota: %+v %v", q, err)
	}
}

func TestBaiduDownloadURL(t *testing.T) {
	hc := &fakeBDHC{responses: map[string]string{
		"method=list":           `{"errno":0,"list":[{"fs_id":222,"path":"/影视/a.mkv","server_filename":"a.mkv","isdir":0,"size":1}]}`,
		"method=locatedownload": `{"errno":0,"list":[{"dlink":"https://d.pcs.baidu.com/file/xxx"}]}`,
	}}
	c := &clientBaidu{drive: Drive{Token: "t"}, hc: hc}
	u, err := c.DownloadURL(context.Background(), "/影视/a.mkv")
	if err != nil {
		t.Fatal(err)
	}
	if u != "https://d.pcs.baidu.com/file/xxx" {
		t.Fatalf("bad dlink: %s", u)
	}
}

func TestBaiduDownloadURLMissing(t *testing.T) {
	hc := &fakeBDHC{responses: map[string]string{
		"method=list": `{"errno":0,"list":[]}`,
	}}
	c := &clientBaidu{drive: Drive{Token: "t"}, hc: hc}
	if _, err := c.DownloadURL(context.Background(), "/nope.mkv"); err == nil {
		t.Fatal("missing file must error")
	}
}
