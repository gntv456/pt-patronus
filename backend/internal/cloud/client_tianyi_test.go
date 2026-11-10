// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package cloud

import (
	"context"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

type fakeTYHC struct{ responses map[string]string }

func (f *fakeTYHC) Do(ctx context.Context, method, rawURL string, body []byte,
	headers map[string]string, cookieStr string, ssrf bool) (*httpclient.Response, error) {
	for frag, resp := range f.responses {
		if containsStr(rawURL, frag) {
			return &httpclient.Response{StatusCode: 200, Body: []byte(resp)}, nil
		}
	}
	return &httpclient.Response{StatusCode: 404, Body: []byte(`{"res_code":1}`)}, nil
}

func TestTianyiListAndResolve(t *testing.T) {
	hc := &fakeTYHC{responses: map[string]string{
		"parentFolderId=": `{"res_code":0,"fileListAO":{
			"folderList":[{"id":11,"name":"影视","isFolder":true}],
			"fileList":[{"fileId":22,"name":"a.mkv","size":789,"url":"https://dl.189.cn/x"}]
		}}`,
	}}
	c := &clientTianyi{drive: Drive{Token: "t"}, hc: hc}
	files, err := c.List(context.Background(), "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || !files[0].IsDir || files[1].IsDir || files[1].Size != 789 {
		t.Fatalf("bad list: %+v", files)
	}
	// 子目录解析：seg=影视 → folderList 命中 id=11（同一 mock 即可验证 resolve 逻辑）
	id, err := c.resolvePath(context.Background(), "/影视")
	if err != nil || id != "11" {
		t.Fatalf("resolve: %q %v", id, err)
	}
}

func TestTianyiResolveNotFound(t *testing.T) {
	hc := &fakeTYHC{responses: map[string]string{
		"parentFolderId=": `{"res_code":0,"fileListAO":{"folderList":[],"fileList":[]}}`,
	}}
	c := &clientTianyi{drive: Drive{Token: "t"}, hc: hc}
	if _, err := c.List(context.Background(), "/不存在"); err == nil {
		t.Fatal("missing dir must error")
	}
}

func TestTianyiTestAndQuota(t *testing.T) {
	hc := &fakeTYHC{responses: map[string]string{
		"listFiles":       `{"res_code":0,"fileListAO":{"folderList":[],"fileList":[]}}`,
		"getUserDiskInfo": `{"res_code":0,"disk":{"totalSize":1099511627776,"usedSize":4096}}`,
	}}
	c := &clientTianyi{drive: Drive{Token: "t"}, hc: hc}
	if _, err := c.Test(context.Background()); err != nil {
		t.Fatal(err)
	}
	q, err := c.Quota(context.Background())
	if err != nil || q.Used != 4096 || q.Total != 1099511627776 {
		t.Fatalf("quota: %+v %v", q, err)
	}
}

func TestTianyiDownloadURL(t *testing.T) {
	hc := &fakeTYHC{responses: map[string]string{
		"parentFolderId=": `{"res_code":0,"fileListAO":{"folderList":[],
			"fileList":[{"fileId":22,"name":"a.mkv","size":1,"url":"https://dl.189.cn/x"}]}}`,
	}}
	c := &clientTianyi{drive: Drive{Token: "t"}, hc: hc}
	u, err := c.DownloadURL(context.Background(), "/a.mkv")
	if err != nil || u != "https://dl.189.cn/x" {
		t.Fatalf("download: %q %v", u, err)
	}
	if _, err := c.DownloadURL(context.Background(), "/"); err == nil {
		t.Fatal("root download must error")
	}
}
