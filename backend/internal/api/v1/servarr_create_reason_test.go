// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

// #340 Servarr 兼容层的「添加电影/剧集」失败出口把原因压成三个字「创建失败」。
//
// 冷门域的典型形状（同 #5）：这个兼容层此前零测试覆盖，而出口写的是
//
//	if err != nil || created == nil {
//	    errcode.Respond(c, errcode.ServarrCreateFailed)   // 连 err 都没带
//	    return
//	}
//
// 而 ServarrCreateFailed 的默认文案就是「创建失败」（codes.go:224，全仓最差的一条：
// 没有对象、没有下一步）。于是用户从 Sonarr/Radarr 点「添加」失败后，看到的是
// 「创建失败」——既不知道是数据库写不进去、还是这条已存在、还是外键约束，
// 更不知道该去哪处理。链层 subCh.Create 的 err 被整个丢弃，
// 且这里连 WithErr 都没挂 ⇒ 日志里也查不到。
//
// 另有一处：`err != nil || created == nil` 把「链层报错」与「链层返回 nil 且无错」
// 两种形状合并成一个出口，而后者根本没有 err 可带。

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// setupServarrCreateServer 与 setupServarrServer 同构，但把写路径（AddMovie/AddSeries）挂上。
func setupServarrCreateServer(t *testing.T, apiKey string) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/servarr-create.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.Subscription{}, &models.SubscriptionHit{}, &models.Downloader{}); err != nil {
		t.Fatal(err)
	}
	subCh := chain.NewSubscribeChain(nil, nil, nil, oper.NewSubscriptionOper(db), nil, nil, nil, nil)
	h := NewServarrHandler(subCh, nil, nil, apiKey)
	r := gin.New()
	r.POST("/api/v3/movie", h.AddMovie)
	r.POST("/api/v3/series", h.AddSeries)
	return r, db
}

func postArr(t *testing.T, r *gin.Engine, path, apikey, body string) *httptest.ResponseRecorder {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if apikey != "" {
		req.Header.Set("X-Api-Key", apikey)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestServarrCreateFailedTextIsActionable 码的默认文案本身就不合格：全仓其它失败码
// 都带上下一步（「下载器暂时连不上：请在『下载器』页测试连接」之类），这里是光秃秃的
// 「创建失败」。即便链层这次没带原因，用户也不能只看到这三个字。
func TestServarrCreateFailedTextIsActionable(t *testing.T) {
	msg := errcode.ServarrCreateFailed.Message
	assertActionable(t, "ServarrCreateFailed", msg,
		[]string{"订阅"}, // 说清是「订阅」这个对象（Radarr/Sonarr 用户认得）
		[]string{"站点", "填", "选", "日志", "查看", "刷新", "换", "检查", "手动"}, // 至少给一条可走的下一步
	)
}

// TestServarrCreateFailuresCarryReason 写面失败必须把原因带进响应体：
// 链层 subCh.Create 的 err 被丢弃、且没挂 WithErr ⇒ 日志与界面都查不到。
func TestServarrCreateFailuresCarryReason(t *testing.T) {
	code := stripCommentsForTest(mustReadSource(t, "servarr.go"))

	// 两个添加出口都必须走带原因的判据，不许裸 Respond(码) —— 那等于把原因扔掉。
	// 数的是「没有 .With 后缀的 ServeFailed」：WithDetail/WithErr 才挂得住原因。
	if n := strings.Count(code, "errcode.ServarrCreateFailed)"); n != 0 {
		t.Errorf("servarr.go 里还有 %d 处裸 Respond(errcode.ServarrCreateFailed)：链层原因会被丢掉", n)
	}
	// 也必须有 WithErr/WithDetail 之一挂上原因，否则日志里查不到。
	if !strings.Contains(code, "ServarrCreateFailed.With") {
		t.Error("失败出口必须用 WithErr/WithDetail 挂上底层原因（日志定位用）")
	}
	// `err != nil || created == nil` 把两种形状合并，后者没有 err 可带。
	if strings.Contains(code, "err != nil || created == nil") {
		t.Error("不许把「链层报错」与「返回 nil 且无错」压成一个出口（后者根本没有原因可带）")
	}
}

// TestServarrCreateStillRejectsBadInput 反向对照：入参校验不能被「带原因」改坏。
func TestServarrCreateStillRejectsBadInput(t *testing.T) {
	r, _ := setupServarrCreateServer(t, "k")
	// 缺 tmdbid → 仍应是 400（不是 500 也不是 200）
	rec := postArr(t, r, "/api/v3/movie", "k", `{"title":"x"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("缺 tmdbid 应 400，got %d: %s", rec.Code, rec.Body.String())
	}
	// 缺 tvdbid/tmdbid → 仍应是 400
	rec = postArr(t, r, "/api/v3/series", "k", `{"title":"x"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("剧集缺 source id 应 400，got %d: %s", rec.Code, rec.Body.String())
	}
}

// assertActionable 断言失败文案说清了对象并给出至少一条下一步。
func assertActionable(t *testing.T, what, msg string, wantObjects, wantNext []string) {
	t.Helper()
	hitObj := false
	for _, w := range wantObjects {
		if strings.Contains(msg, w) {
			hitObj = true
		}
	}
	if !hitObj {
		t.Errorf("%s 的文案没说清是什么对象：%q", what, msg)
	}
	hitNext := false
	for _, w := range wantNext {
		if strings.Contains(msg, w) {
			hitNext = true
		}
	}
	if !hitNext {
		t.Errorf("%s 的文案没给可走的下一步：%q", what, msg)
	}
}

// mustReadSource 读同目录的被测源文件（源码级判据用；行为级判据走真 HTTP）。
func mustReadSource(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("读不到 %s：%v", name, err)
	}
	return string(b)
}
