// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package cloud

import (
	"context"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// fakeODHC mock Graph 响应：按 URL 子串分发（首个命中生效）。
type fakeODHC struct {
	responses map[string]string
	calls     []string
}

func (f *fakeODHC) Do(ctx context.Context, method, rawURL string, body []byte,
	headers map[string]string, cookieStr string, ssrf bool) (*httpclient.Response, error) {
	f.calls = append(f.calls, rawURL)
	for frag, resp := range f.responses {
		if strings.Contains(rawURL, frag) {
			return &httpclient.Response{StatusCode: 200, Body: []byte(resp)}, nil
		}
	}
	return &httpclient.Response{StatusCode: 404, Body: []byte(`{"error":{"message":"not found"}}`)}, nil
}

func TestOneDriveList(t *testing.T) {
	hc := &fakeODHC{responses: map[string]string{
		"/me/drive/root/children": `{"value":[
			{"id":"1","name":"Movies","folder":{}},
			{"id":"2","name":"a.mkv","size":123,"file":{}}
		]}`,
	}}
	c := &clientOneDrive{drive: Drive{Token: "t"}, hc: hc}
	files, err := c.List(context.Background(), "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || !files[0].IsDir || files[1].IsDir || files[1].Size != 123 {
		t.Fatalf("bad list: %+v", files)
	}
	if files[1].Path != "/a.mkv" {
		t.Fatalf("bad path join: %s", files[1].Path)
	}
}

func TestOneDriveListPathNotFound(t *testing.T) {
	hc := &fakeODHC{}
	c := &clientOneDrive{drive: Drive{Token: "t"}, hc: hc}
	if _, err := c.List(context.Background(), "/nope"); err == nil {
		t.Fatal("expect 404 error")
	}
}

func TestOneDriveTestAndQuota(t *testing.T) {
	hc := &fakeODHC{responses: map[string]string{
		"/me/drive": `{"owner":{"user":{"displayName":"someone@example.com"}},"quota":{"used":100,"total":1000}}`,
	}}
	c := &clientOneDrive{drive: Drive{Token: "t"}, hc: hc}
	info, err := c.Test(context.Background())
	if err != nil || info == "" {
		t.Fatalf("test: %q %v", info, err)
	}
	q, err := c.Quota(context.Background())
	if err != nil || q.Used != 100 || q.Total != 1000 {
		t.Fatalf("quota: %+v %v", q, err)
	}
}

func TestOneDriveDownloadURL(t *testing.T) {
	hc := &fakeODHC{responses: map[string]string{
		"/me/drive/root:/Movies/a.mkv": `{"id":"2","name":"a.mkv","file":{},"@microsoft.graph.downloadUrl":"https://pre.signed/url"}`,
	}}
	c := &clientOneDrive{drive: Drive{Token: "t"}, hc: hc}
	u, err := c.DownloadURL(context.Background(), "/Movies/a.mkv")
	if err != nil {
		t.Fatal(err)
	}
	if u != "https://pre.signed/url" {
		t.Fatalf("bad url: %s", u)
	}
	if _, err := c.DownloadURL(context.Background(), "/"); err == nil {
		t.Fatal("root download should error")
	}
}
