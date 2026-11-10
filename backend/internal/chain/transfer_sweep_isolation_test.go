// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/transfer"
)

// Sweep 种子循环的 per-item 隔离：单个种子在整理流程里 panic（本测试用
// links/engine 未装配的链条复现真实 nil 解引用）不能带走整轮扫盘——
// 此前 panic 会逃出 Sweep，同轮其余种子/下载器全漏跑。
func TestTransferSweepIsolatesPerTorrentPanic(t *testing.T) {
	const (
		hashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		hashB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		hashC = "cccccccccccccccccccccccccccccccccccccccc"
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/sync/maindata":
			_, _ = w.Write([]byte(`{
				"rid":1,"full_update":true,
				"torrents":{
					"` + hashA + `":{"name":"Torrent A","progress":1,"state":"uploading","completion_on":1700000000,"save_path":"/dl","category":"pt","tags":"ptpsrc|mteam|123|movie"},
					"` + hashB + `":{"name":"Torrent B","progress":1,"state":"uploading","completion_on":1700000000,"save_path":"/dl","category":"pt","tags":""},
					"` + hashC + `":{"name":"Torrent C","progress":0.5,"state":"downloading","save_path":"/dl","category":"pt"}
				},
				"server_state":{}
			}`))
		case "/api/v2/torrents/files":
			_, _ = w.Write([]byte(`[]`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	db := newSweepDB(t)
	if err := db.AutoMigrate(&models.TransferHistory{}); err != nil {
		t.Fatal(err)
	}
	if err := transfer.Save(oper.NewSystemConfigOper(db), transfer.Config{Enabled: true, Categories: []string{"pt"}}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Downloader{Name: "qb1", Type: "qb", URL: srv.URL}).Error; err != nil {
		t.Fatal(err)
	}
	ch := newSweepChain(db)
	ch.hist = oper.NewTransferHistoryOper(db)

	sum := ch.Sweep(context.Background())
	if sum.Candidates != 2 {
		t.Fatalf("两个 completed 种子应计入候选，got %+v", sum)
	}
	if len(sum.Failed) != 2 {
		t.Fatalf("两个崩溃种子都要就地记账（panic 不得逃逸打断循环），got %v", sum.Failed)
	}
	for _, want := range []string{"Torrent A", "Torrent B"} {
		found := false
		for _, f := range sum.Failed {
			if strings.Contains(f, want) && strings.Contains(f, "panic:") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("失败清单缺少 %s 的 panic 记录：%v", want, sum.Failed)
		}
	}
}
