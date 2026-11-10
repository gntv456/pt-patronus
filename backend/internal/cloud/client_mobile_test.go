// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package cloud

import (
	"context"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

type fakeCMHC struct{ responses map[string]string }

func (f *fakeCMHC) Do(ctx context.Context, method, rawURL string, body []byte,
	headers map[string]string, cookieStr string, ssrf bool) (*httpclient.Response, error) {
	for frag, resp := range f.responses {
		if containsStr(rawURL, frag) {
			return &httpclient.Response{StatusCode: 200, Body: []byte(resp)}, nil
		}
	}
	return &httpclient.Response{StatusCode: 404, Body: []byte(`{"code":404}`)}, nil
}

func TestMobileListAndResolve(t *testing.T) {
	hc := &fakeCMHC{responses: map[string]string{
		"queryContentList": `{"code":0,"data":{"list":[
			{"id":"f1","isCatalog":true,"name":"影视"},
			{"id":"f2","isCatalog":false,"name":"a.mkv","size":"10240"}
		]}}`,
	}}
	c := &clientMobile{drive: Drive{Token: "t"}, hc: hc}
	files, err := c.List(context.Background(), "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || !files[0].IsDir || files[1].IsDir || files[1].Size != 10240 {
		t.Fatalf("bad list: %+v", files)
	}
	id, err := c.resolvePath(context.Background(), "/影视")
	if err != nil || id != "f1" {
		t.Fatalf("resolve: %q %v", id, err)
	}
}

func TestMobileListErrCode(t *testing.T) {
	hc := &fakeCMHC{responses: map[string]string{
		"queryContentList": `{"code":10001,"message":"token expired"}`,
	}}
	c := &clientMobile{drive: Drive{Token: "expired"}, hc: hc}
	if _, err := c.List(context.Background(), "/"); err == nil {
		t.Fatal("code!=0 must error")
	}
}

func TestMobileQuota(t *testing.T) {
	hc := &fakeCMHC{responses: map[string]string{
		"queryContentList":  `{"code":0,"data":{"list":[]}}`,
		"queryUserDiskInfo": `{"code":0,"data":{"totalCapacity":"536870912000","usedCapacity":"2048"}}`,
	}}
	c := &clientMobile{drive: Drive{Token: "t"}, hc: hc}
	q, err := c.Quota(context.Background())
	if err != nil || q.Used != 2048 || q.Total != 536870912000 {
		t.Fatalf("quota: %+v %v", q, err)
	}
	if _, err := c.Test(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestMobileDownloadURL(t *testing.T) {
	hc := &fakeCMHC{responses: map[string]string{
		"queryContentList":   `{"code":0,"data":{"list":[{"id":"f2","isCatalog":false,"name":"a.mkv","size":"1"}]}}`,
		"getFileDownloadUrl": `{"code":0,"data":{"downloadUrl":"https://dl.caiyun.cn/x"}}`,
	}}
	c := &clientMobile{drive: Drive{Token: "t"}, hc: hc}
	u, err := c.DownloadURL(context.Background(), "/a.mkv")
	if err != nil || u != "https://dl.caiyun.cn/x" {
		t.Fatalf("download: %q %v", u, err)
	}
	if _, err := c.DownloadURL(context.Background(), "/"); err == nil {
		t.Fatal("root download must error")
	}
}
