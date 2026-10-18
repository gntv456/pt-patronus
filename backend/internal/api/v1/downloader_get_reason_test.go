package v1

// #372 Downloader 的 Get 是全仓最后一处「按 id 查不到就说不存在」的手写出口。
//
// 同一个文件里 Update（downloader.go:108）早就走了 respondRecordError，
// Delete/Enable/Disable 等也走了 —— 只剩 Get 这一处还手抄：
//
//	if err != nil {
//		errcode.Respond(c, errcode.DownloaderNotFound.WithErr(err))  // ← 404「下载器不存在」
//	}
//
// 而 DownloaderOper.Get 底层是 db.First（oper/downloader.go:106），它把两种
// 完全不同的失败原样返回：
//
//	库里没这条记录 → 该 404「请刷新下载器列表后重新选择」
//	DB 真故障（磁盘满/连接断/锁/表锁）→ 该 500，说清是服务端读不到
//
// 现在一律 404「下载器不存在」：数据库一挂，用户在下载器详情页看到的也是
// 「下载器不存在」，于是跑去下载器列表页刷新——那页同样故障，而用户被告知
// 的方向（这条记录被删了）从头到尾是错的。这正是 #342（15 处站点查询）与
// #345/#343（工作流/Webhook 查询）治的同一族，全仓只剩这一处。

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"gorm.io/gorm"
)

func classifyDownloaderGet(t *testing.T, err error) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	respondRecordError(c, err, errcode.DownloaderNotFound, errcode.DownloaderInvalidInput, errcode.Internal)
	var env map[string]any
	if e := json.Unmarshal(rec.Body.Bytes(), &env); e != nil {
		t.Fatalf("响应不是 JSON：%q", rec.Body.String())
	}
	return rec.Code, env
}

// TestDownloaderGetSeparatesNotFoundFromDBFailure 记录不存在与 DB 故障必须分开。
func TestDownloaderGetSeparatesNotFoundFromDBFailure(t *testing.T) {
	// 记录不存在：404 + 下载器专属码（该刷新列表，这是唯一该说「不存在」的情况）。
	status, env := classifyDownloaderGet(t, gorm.ErrRecordNotFound)
	if status != http.StatusNotFound || env["code"] != errcode.DownloaderNotFound.Code {
		t.Fatalf("记录不存在应 404 + %q，got %d + %v", errcode.DownloaderNotFound.Code, status, env["code"])
	}

	// DB 真故障：500 + Internal。说成 404 会让用户跑去刷新列表，那页同样故障。
	dbDown := errors.New("sql: database is closed")
	status2, env2 := classifyDownloaderGet(t, dbDown)
	if status2 != http.StatusInternalServerError || env2["code"] != errcode.Internal.Code {
		t.Fatalf("DB 故障应 500 + internal，got %d + %v（旧形状：被说成「下载器不存在」，方向全错）",
			status2, env2["code"])
	}
	msg, _ := env2["message"].(string)
	if strings.Contains(msg, "下载器不存在") {
		t.Errorf("DB 故障不许说成「下载器不存在」：%q", msg)
	}
}

// TestDownloaderGetOutletUsesClassifier 接线护栏：Get 必须走分类器。
//
// 只驱动 respondRecordError 是不够的 —— 把 Get 的出口换回手抄的
// DownloaderNotFound.WithErr(err)，上面那条依然全绿（本轮第三次踩「出口没接」）。
func TestDownloaderGetOutletUsesClassifier(t *testing.T) {
	src, err := os.ReadFile("downloader.go")
	if err != nil {
		t.Fatalf("读不到 downloader.go：%v", err)
	}
	var lines []string
	for _, l := range strings.Split(string(src), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		lines = append(lines, l)
	}
	code := strings.Join(lines, "\n")
	at := strings.Index(code, "func (h *DownloaderHandler) Get(")
	if at < 0 {
		t.Fatal("找不到 Get（判据没有落点）")
	}
	tail := code[at:]
	if n := strings.Index(tail[1:], "\nfunc "); n > 0 {
		tail = tail[:1+n]
	}
	if strings.Contains(tail, "errcode.DownloaderNotFound.WithErr(err)") {
		t.Error("Get 不许把 DB 故障说成「下载器不存在」（全仓最后一处手写查询出口）")
	}
	if !strings.Contains(tail, "respondRecordError(c, err, errcode.DownloaderNotFound") {
		t.Error("Get 必须走 respondRecordError（与同文件 Update/Delete 同一判据）")
	}
}
