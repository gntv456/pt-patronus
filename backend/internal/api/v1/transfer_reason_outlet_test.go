package v1

// #336 手动整理/重做（POST /transfer/organize、/transfer/redo/:id）的原因没进响应体。
//
// 链层在 #324 已经把「这颗种子不在该下载器」写成可操作的中文（chain/transfer.go 里
// `下载器（编号 N）里没有这颗种子…请在「下载器」页…`），Redo 甚至用 %w 特意保住了
// ErrCannotConnect 想让 errtext 翻成「去『下载器』页测试连接」——但两个出口都写成
//
//	errcode.Respond(c, errcode.NotFound.WithErr(err))
//
// WithErr 只做 ne.Err = err，一个字都不动 Message；Respond 于是把 message/error 两个
// 字段都写成 NotFound.Message =「请求的资源不存在」。链层那份中文只进 slog.Debug，
// 默认 INFO 级别连日志都看不到。
//
// 于是界面永远只弹「请求的资源不存在」：用户既不知道该换哪台下载器，也不知道这颗种子
// 是被删了还是被做种完成后清理了。#324 那条注释写的「经 handler 的 userErrText 弹成
// toast」从未发生——这条路径走的是 errcode 硬失败面，从来没经过 userErrText。
// 旧测试只做源码扫描（transfer_manual_reason_test.go 钉字符串），中文改对了它照样绿，
// 出口照样丢——所以本测试钉的是「原因进响应体」，不是「源码里有这句中文」。

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader/manager"
	"gorm.io/gorm"
)

// newFakeQB 起一台假 qB：登录/版本都正常，maindata 回空种子表。
// 目的是让 ListTorrents 成功返回空列表——这样「种子不在该下载器」这条出口
// 是被确定性地走到的，而不是靠下载器连不上（那会停在上一层，测不到本出口）。
func newFakeQB(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/app/version":
			_, _ = w.Write([]byte("v5.0.0"))
		case "/api/v2/sync/maindata":
			_, _ = w.Write([]byte(`{"rid":1,"full_update":true,"torrents":{},"server_state":{},"tags":[]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// setupTransferReasonServer 与 setupTransferServer 同构，但装了真实的 dlMgr：
// 上游那个把 dlMgr 传 nil，只够跑入参校验那条 4xx 路径，跑不到「找种子」这一步。
// 这里配一台假 qB（登录 + 版本 + 空 maindata），让 ListTorrents 成功返回空列表，
// 从而确定性地走进 resolveAndProcessResult 的「没找到」出口。
func setupTransferReasonServer(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/transfer-reason.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.TransferHistory{}, &models.SystemConfig{}, &models.Downloader{}, &models.DownloadMediaLink{}); err != nil {
		t.Fatal(err)
	}

	srv := newFakeQB(t)
	crypto := security.NewCrypto("tr-reason-seed")
	dlOp := oper.NewDownloaderOper(db, crypto)
	if err := db.Create(&models.Downloader{Name: "假 qB", Type: "qb", URL: srv}).Error; err != nil {
		t.Fatal(err)
	}

	ch := chain.NewTransferChain(chain.NewBase(module.New(), event.New()), &chain.MediaChain{},
		manager.NewManager(dlOp), dlOp,
		oper.NewTransferHistoryOper(db), oper.NewDownloadMediaLinkOper(db),
		oper.NewSystemConfigOper(db), nil, nil, nil)
	h := NewTransferHandler(ch)
	r := gin.New()
	r.POST("/transfer/organize", h.Organize)
	r.POST("/transfer/redo/:id", h.Redo)
	return r, db
}

// TestTransfer_OrganizeReasonReachesResponse 「手动整理：种子不在该下载器」的原因
// 必须进响应体的 message/error，而不是被压成「请求的资源不存在」。
func TestTransfer_OrganizeReasonReachesResponse(t *testing.T) {
	r, _ := setupTransferReasonServer(t)
	rec := doTr(t, r, http.MethodPost, "/transfer/organize", `{"downloader_id":1,"hash":"deadbeef"}`)
	body := rec.Body.String()

	// 通用兜底话术不许出现：它正是本次要消灭的那个「钉死的猜测」。
	if strings.Contains(body, "请求的资源不存在") {
		t.Fatalf("原因被压成通用话术了（链层那段中文没进响应体），got %s", body)
	}
	// 可操作的下一步：用户得知道去「下载器」页。
	if !strings.Contains(body, "下载器") {
		t.Errorf("响应体没给可操作的下一步，got %s", body)
	}
	// 那颗种子本身也要带出来，否则用户无从判断是哪条。
	if !strings.Contains(body, "deadbeef") {
		t.Errorf("响应体没带出种子 hash，got %s", body)
	}
}

// TestTransfer_RedoReasonReachesResponse 同上，但走 Redo：它的原因更讲究
// （区分「压根没问到下载器」与「种子真不在了」），更不该被压成一句 404。
func TestTransfer_RedoReasonReachesResponse(t *testing.T) {
	r, db := setupTransferReasonServer(t)
	db.Create(&models.TransferHistory{Status: "success", DownloadHash: "cafebabe"})
	rec := doTr(t, r, http.MethodPost, "/transfer/redo/1", `{}`)
	body := rec.Body.String()

	if strings.Contains(body, "请求的资源不存在") {
		t.Fatalf("原因被压成通用话术了（Redo 那份区分过的中文没进响应体），got %s", body)
	}
	if !strings.Contains(body, "下载器") {
		t.Errorf("响应体没给可操作的下一步，got %s", body)
	}
}

// TestTransfer_DeleteHistoryNotFoundStillHasOwnCode 反向对照：历史不存在仍要 404 +
// transfer_history_not_found。别把「让原因进响应体」改成「一律 200 + 大杂烩」。
func TestTransfer_DeleteHistoryNotFoundStillHasOwnCode(t *testing.T) {
	r, db := setupTransferReasonServer(t)
	db.Create(&models.TransferHistory{Status: "success"})
	rec := doTr(t, r, http.MethodDelete, "/transfer/history/9999", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("删不存在的历史应 404，got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestTransfer_ReasonOutletsShareTheClassifier 接线护栏：organize/redo 两个出口必须
// 走 respondReasonErr，不许各自抄一份 WithErr —— 那种写法正是 #324 改完中文仍丢原因的原因。
func TestTransfer_ReasonOutletsShareTheClassifier(t *testing.T) {
	src, err := os.ReadFile("transfer.go")
	if err != nil {
		t.Fatalf("读不到被测源文件: %v", err)
	}
	code := stripTransferCodeComments(string(src))
	if n := strings.Count(code, "respondReasonErr(c, err,"); n != 2 {
		t.Errorf("走统一判据的出口 = %d, want 2（organize + redo）⇒ 又有出口自己抄归类", n)
	}
	// 反向断言：这两个出口不许再出现 WithErr（那会把链层原因整段丢掉）。
	for _, fn := range []string{"ManualOrganize", "Redo(c *gin.Context"} {
		at := strings.Index(code, fn)
		if at < 0 {
			t.Fatalf("找不到 %s（判据没有落点）", fn)
		}
		tail := code[at:]
		if cut := strings.Index(tail[1:], "\nfunc "); cut > 0 {
			tail = tail[:1+cut]
		}
		if strings.Contains(tail, "errcode.NotFound.WithErr(err)") {
			t.Errorf("%s 的出口又变回 WithErr：链层原因会被丢进日志只回通用话术", fn)
		}
	}
}

// stripTransferCodeComments 去掉整行注释，免得把判据本身当成被测代码。
func stripTransferCodeComments(s string) string {
	out := make([]string, 0, 128)
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}
