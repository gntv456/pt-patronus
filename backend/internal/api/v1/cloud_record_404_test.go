// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
)

// #246：云盘的配额与列目录从前不分原因一律回 400「请求参数有误，请检查后重试」
// （真机实测 :18103：GET /cloud/drives/999999/quota ⇒ 400 invalid_input）。
// id 是格式完全合法的 uint，真实原因是「这条云盘不存在」——把「记录没了」算成用户的错，
// 用户照着「检查后重试」点多少次都不会好。同 #107/#108/#119/#120 那一族。
//
// 这里打的是真生产通路：handler → chain.CloudChain → oper → SQLite（空库，不挂替身），
// 所以红一定红在状态码与本域码上。
func TestCloudQuotaBrowseMissingDriveIs404(t *testing.T) {
	r, _ := setupCloudServer(t)

	cases := []struct {
		name string
		path string
	}{
		{"配额 /quota", "/cloud/drives/999999/quota"},
		{"列目录 /browse", "/cloud/drives/999999/list?path=/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doCloud(t, r, http.MethodGet, tc.path, "")
			if rec.Code != http.StatusNotFound {
				t.Fatalf("%s：云盘不存在该回 404，实际 %d, body=%q（400「请求参数有误」会把「记录没了」算成用户的错）",
					tc.name, rec.Code, rec.Body.String())
			}
			var env struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
				t.Fatalf("响应不是 JSON 信封: %v, raw=%q", err, rec.Body.String())
			}
			if env.Code != "cloud_drive_not_found" {
				t.Errorf("本域码 = %q, want cloud_drive_not_found（NoRoute 也回 404，只认状态码会被骗）", env.Code)
			}
			if !strings.Contains(env.Message, "云盘") {
				t.Errorf("中文提示没点名是哪条记录没了: %q", env.Message)
			}
		})
	}
}

// #247：测试脚手架与生产路由漂移。生产只注册 `/cloud/drives/:id/list`
// （routes_monitor.go:45 `dr.GET("/:id/list", cloudH.Browse)`），而旧 setup 注册的是
// `/cloud/drives/:id/browse`——那条路径在生产里根本不存在，所以历史上打在它上面的用例
// 再怎么绿，也没有一条真正走过用户打的那条路由。这里钉住「两边一致」。
func TestCloudHarnessRoutesMatchProduction(t *testing.T) {
	harness, err := os.ReadFile("cloud_test.go")
	if err != nil {
		t.Fatalf("读不到 setupCloudServer 所在文件: %v", err)
	}
	routes, err := os.ReadFile("../routes_monitor.go")
	if err != nil {
		t.Fatalf("读不到生产路由表: %v", err)
	}
	h, r := string(harness), string(routes)

	if !strings.Contains(r, `dr.GET("/:id/list", cloudH.Browse)`) {
		t.Fatal("生产路由表里云盘列目录的注册方式变了，这条护栏需要跟着改")
	}
	if !strings.Contains(h, `r.GET("/cloud/drives/:id/list", h.Browse)`) {
		t.Errorf("测试 setup 没注册生产路径 /cloud/drives/:id/list ⇒ 用例打在用户打不到的路由上")
	}
	if strings.Contains(h, "/browse\"") {
		t.Errorf("测试 setup 里仍有 /browse 这条生产不存在的路由")
	}
	for _, path := range []string{"/quota", "/list"} {
		if !strings.Contains(h, `"/cloud/drives/:id`+path+`"`) {
			t.Errorf("setup 缺 /cloud/drives/:id%s（生产有这条）", path)
		}
	}
}

// 接线护栏：两个出口必须用同一个共用分类器，而不是各自再抄一遍归类。
// 只扫真源码并先剥 // 注释——我自己写的说明里也出现了旧写法字样，不剥就会自己造自己的假红/假绿。
func TestCloudOutletsShareTheClassifier(t *testing.T) {
	src, err := os.ReadFile("cloud.go")
	if err != nil {
		t.Fatalf("读不到被测源文件: %v", err)
	}
	var code strings.Builder
	for _, line := range strings.Split(string(src), "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		code.WriteString(line)
		code.WriteByte('\n')
	}
	got := strings.Count(code.String(), "respondRecordError(c, err, errcode.CloudDriveNotFound, errcode.InvalidInput, errcode.BadGateway)")
	if got != 2 {
		t.Errorf("云盘「读面」走共用分类器的出口 = %d, want 2（quota + browse）⇒ 又有出口自己抄归类", got)
	}
}
