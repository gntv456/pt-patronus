// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// longptMedalServer 构造覆盖六十四卦全部端点的 mock。
func longptMedalServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "pluginsMedalInfo/getList"):
			_, _ = w.Write([]byte(`{"code":0,"data":[
				{"medalId":1,"parentMedalId":0,"medalType":"taiji","medalName":"太极勋章","medalLevel":1,"owned":true},
				{"medalId":10,"parentMedalId":1,"medalType":"liangyi","medalName":"两仪·阳","medalLevel":2,"owned":false},
				{"medalId":1010,"parentMedalId":10,"medalType":"sixiang","medalName":"四象","medalLevel":3,"owned":false},
				{"medalId":111111,"parentMedalId":1010,"medalType":"bagua","medalName":"八卦","medalLevel":4,"owned":false}]}`))
		case strings.HasSuffix(r.URL.Path, "getUserMedalShardInfo"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"yangYao":5,"yinYao":3}}`))
		case strings.HasSuffix(r.URL.Path, "mergeUserMedal"):
			_, _ = w.Write([]byte(`{"code":0,"msg":"合成成功"}`))
		case strings.HasSuffix(r.URL.Path, "mergeUserBigMedal"):
			_, _ = w.Write([]byte(`{"code":0,"msg":"大勋章合成成功"}`))
		case strings.HasSuffix(r.URL.Path, "convertYinYang"):
			_, _ = w.Write([]byte(`{"code":0,"msg":"转换成功"}`))
		default:
			w.WriteHeader(404)
		}
	}))
}

// TestLongPTMedalTree 勋章树 + 卦象消耗计算（medalId 数 1/0）+ 大勋章 cost。
func TestLongPTMedalTree(t *testing.T) {
	srv := longptMedalServer()
	defer srv.Close()

	nodes, err := LongPTMedalGame{}.MedalTree(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 4 {
		t.Fatalf("nodes len = %d", len(nodes))
	}
	byID := map[string]MedalNode{}
	for _, n := range nodes {
		byID[n.MedalID] = n
	}
	// "10" → 1 阳 1 阴
	if n := byID["10"]; n.NeedYang != 1 || n.NeedYin != 1 || n.BigCost != 0 {
		t.Errorf("liangyi 10 = %+v", n)
	}
	// "1010" → 2 阳 2 阴
	if n := byID["1010"]; n.NeedYang != 2 || n.NeedYin != 2 {
		t.Errorf("sixiang 1010 = %+v", n)
	}
	// "111111" → 6 阳 0 阴；level4 大勋章 cost=200000
	if n := byID["111111"]; n.NeedYang != 6 || n.NeedYin != 0 || n.BigCost != 200000 {
		t.Errorf("bagua 111111 = %+v", n)
	}
	if nodes[0].Name != "太极勋章" || !nodes[0].Owned {
		t.Errorf("taiji = %+v", nodes[0])
	}
}

// TestLongPTMedalShards 碎片余额。
func TestLongPTMedalShards(t *testing.T) {
	srv := longptMedalServer()
	defer srv.Close()

	st, err := LongPTMedalGame{}.MedalShards(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if st.YangYao != 5 || st.YinYao != 3 {
		t.Errorf("shards = %+v", st)
	}
}

// TestLongPTComposeMedal 碎片合成（mergeUserMedal 传 medalId 数字形式）。
func TestLongPTComposeMedal(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = w.Write([]byte(`{"code":0,"msg":"合成成功"}`))
	}))
	defer srv.Close()

	res, err := LongPTMedalGame{}.ComposeMedal(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"}, "1010")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.Message != "合成成功" {
		t.Errorf("compose = %+v", res)
	}
	// medalId 必须是数字形式（无引号），服务端按数字校验
	if !strings.Contains(gotBody, `"medalId":1010`) {
		t.Errorf("body = %q, want medalId as number", gotBody)
	}
}

// TestLongPTComposeBigMedal 大勋章合成。
func TestLongPTComposeBigMedal(t *testing.T) {
	srv := longptMedalServer()
	defer srv.Close()

	res, err := LongPTMedalGame{}.ComposeBigMedal(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"}, "111111")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Errorf("compose big = %+v", res)
	}
}

// TestLongPTConvertShard 阴阳转换 query 透传。
func TestLongPTConvertShard(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"code":0,"msg":"转换成功"}`))
	}))
	defer srv.Close()

	res, err := LongPTMedalGame{}.ConvertShard(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"}, -1)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Errorf("convert = %+v", res)
	}
	if gotQuery != "yiYang=-1" {
		t.Errorf("query = %q, want yiYang=-1", gotQuery)
	}
}

// TestLongPTComposeFail 合成失败（code!=0）→ Success:false + msg。
func TestLongPTComposeFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"msg":"阳爻不足"}`))
	}))
	defer srv.Close()

	res, _ := LongPTMedalGame{}.ComposeMedal(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"}, "1010")
	if res.Success {
		t.Error("expected fail")
	}
	if !strings.Contains(res.Message, "阳爻") {
		t.Errorf("msg = %q", res.Message)
	}
}
