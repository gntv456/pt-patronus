package v1

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// #282 云盘操作出口（POST /api/v1/cloud/organize、/cloud/strm-repair、/cloud/115open/*）。
// 从前这四条把一切回成 400「请求参数有误，请检查后重试」：
//   - 整理缺的是「媒体识别没配」——要去配置，不是改参数；
//   - 网盘令牌解不开（换过主密钥）——要去重新保存凭据；不拦住的话会拿着**空令牌去写云盘**；
//   - 115 授权的发起/轮询是纯上游调用，没有任何输入可检查。
//
// 路径照生产抄（api/cloud.ts 打 /cloud/organize、/cloud/strm-repair，base=/api/v1）。
// 三条路由用例都停在「还没向云盘发请求」的那一步，测试不外呼真 115。

func cloudOpRouter(t *testing.T, driveSeed, readSeed string) (*gin.Engine, uint, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/cloud.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.CloudDrive{}, &models.StrmRule{}, &models.StrmRecord{}); err != nil {
		t.Fatal(err)
	}
	// Token 明文交给 Oper 加密（与真链路同构），所以库里密文由 driveSeed 产出。
	d, err := oper.NewCloudDriveOper(db, security.NewCrypto(driveSeed)).Create(oper.CloudDriveInput{
		Name: "115-测试盘", Type: "115", Token: "uid=1; token=abc", RootPath: "/",
	})
	if err != nil {
		t.Fatal(err)
	}
	subCh := chain.NewSubscribeChain(nil, nil, nil, oper.NewSubscriptionOper(db), nil, nil, nil, nil)
	svc, err := chain.NewCloudChain(chain.NewBase(module.New(), event.New()),
		oper.NewCloudDriveOper(db, security.NewCrypto(readSeed)), oper.NewStrmRuleOper(db), subCh, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := NewCloudHandler(svc)
	r := gin.New()
	r.POST("/api/v1/cloud/organize", h.OrganizeCloud)
	r.POST("/api/v1/cloud/strm-repair", h.RepairStrm)
	// 注入媒体识别桩，让 organize 走过「未配置」那一档、打到取盘/取令牌那一步。
	enableSearch := func() {
		svc.SetMediaSearcher(func(_ context.Context, _ string, _ int, _ string) (*chain.MediaSearchItem, error) {
			return nil, nil
		})
	}
	return r, d.ID, enableSearch
}

func doCloudOp(t *testing.T, r *gin.Engine, path, body string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	var env map[string]any
	if e := json.Unmarshal(rec.Body.Bytes(), &env); e != nil {
		t.Fatalf("%s → %d，响应不是错误信封：%q", path, rec.Code, rec.Body.String())
	}
	return rec.Code, env
}

// 媒体识别没配：503 + 「去哪儿配」，不是 400 让人检查参数。
func TestOrganizeWithoutMediaSearch(t *testing.T) {
	r, id, _ := cloudOpRouter(t, "seed-a", "seed-a")
	status, env := doCloudOp(t, r, "/api/v1/cloud/organize", fmt.Sprintf(`{"drive_id":%d,"src_dir":"/"}`, id))
	if status != http.StatusServiceUnavailable || env["code"] != errcode.MediaSearchUnconfigured.Code {
		t.Fatalf("status=%d 整包=%v，期望 503 + %q", status, env, errcode.MediaSearchUnconfigured.Code)
	}
	msg, _ := env["message"].(string)
	for _, want := range []string{"媒体源", "未移动"} {
		if !strings.Contains(msg, want) {
			t.Errorf("缺 %q：%q", want, msg)
		}
	}
	if strings.Contains(msg, "参数有误") {
		t.Errorf("这一项没有可检查的输入：%q", msg)
	}
}

// 令牌解不开（主密钥换过）：503 + 点名哪台盘 + 声明没向云盘发请求。
func TestOrganizeWithUndecryptableToken(t *testing.T) {
	r, id, enable := cloudOpRouter(t, "seed-a", "seed-b") // 密文由 seed-a 写，链用 seed-b 读
	enable()
	status, env := doCloudOp(t, r, "/api/v1/cloud/organize", fmt.Sprintf(`{"drive_id":%d,"src_dir":"/"}`, id))
	if status != http.StatusServiceUnavailable || env["code"] != errcode.CloudTokenUnreadable.Code {
		t.Fatalf("status=%d 整包=%v，期望 503 + %q", status, env, errcode.CloudTokenUnreadable.Code)
	}
	msg, _ := env["message"].(string)
	for _, want := range []string{"115-测试盘", "无法解密", "重新保存", "没有向云盘发请求"} {
		if !strings.Contains(msg, want) {
			t.Errorf("缺 %q：%q", want, msg)
		}
	}
}

// 正控：盘不存在是引用失效（404），不许和上面两档混。
func TestOrganizeDeletedDriveIs404(t *testing.T) {
	r, _, enable := cloudOpRouter(t, "seed-a", "seed-a")
	enable()
	status, env := doCloudOp(t, r, "/api/v1/cloud/organize", `{"drive_id":999999,"src_dir":"/"}`)
	if status != http.StatusNotFound || env["code"] != errcode.CloudDriveNotFound.Code {
		t.Fatalf("status=%d 整包=%v，期望 404 + %q", status, env, errcode.CloudDriveNotFound.Code)
	}
}

// 正控（这条修复的前提）：同一把主密钥下 Token 正常解出，不会被告「令牌坏了」；
// 换了密钥才是真故障。两条方向都在这里钉住，organize 的路由用例才不是空判据。
func TestCloudTokenDecryptPrecondition(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/tok.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.CloudDrive{}); err != nil {
		t.Fatal(err)
	}
	writer := oper.NewCloudDriveOper(db, security.NewCrypto("seed-a"))
	d, err := writer.Create(oper.CloudDriveInput{Name: "d", Type: "115", Token: "uid=1; token=abc"})
	if err != nil {
		t.Fatal(err)
	}
	if tok, err := writer.Token(d); err != nil || tok != "uid=1; token=abc" {
		t.Fatalf("同一把密钥应解得开：token=%q err=%v", tok, err)
	}
	if _, err := oper.NewCloudDriveOper(db, security.NewCrypto("seed-b")).Token(d); err == nil {
		t.Fatal("换过主密钥必须报错——否则接口层那一档永远不会触发")
	}
}

// snake_case 参数真的落进链层：缺 drive_id 必须是 400（这才叫参数问题），
// 而带上 drive_id 时不许再退化成「整理 0 号盘」的 404。
func TestOrganizeBindsSnakeCaseParams(t *testing.T) {
	r, id, _ := cloudOpRouter(t, "seed-a", "seed-a")
	status, env := doCloudOp(t, r, "/api/v1/cloud/organize", `{"src_dir":"/"}`)
	if status != http.StatusBadRequest || env["code"] != errcode.InvalidInput.Code {
		t.Fatalf("没带 drive_id 应 400：status=%d 整包=%v", status, env)
	}

	status, env = doCloudOp(t, r, "/api/v1/cloud/organize", fmt.Sprintf(`{"drive_id":%d,"src_dir":"/电影"}`, id))
	if env["code"] == errcode.CloudDriveNotFound.Code {
		t.Fatalf("drive_id=%d 明明存在却被当成 0 号盘（键名没绑上）：%v", id, env)
	}
	if status != http.StatusServiceUnavailable || env["code"] != errcode.MediaSearchUnconfigured.Code {
		t.Fatalf("参数绑上后应停在「媒体识别没配」这一档：status=%d 整包=%v", status, env)
	}
}

// 判据本身：四档各归各码（115 授权两半走同一条，测试不外呼）。
func TestCloudOpErrClassifiesEveryOutlet(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		code   string
		status int
	}{
		{"令牌解不开", chain.ErrCloudTokenUnreadable, errcode.CloudTokenUnreadable.Code, http.StatusServiceUnavailable},
		{"媒体识别没配", chain.ErrCloudMediaSearchUnconfigured, errcode.MediaSearchUnconfigured.Code, http.StatusServiceUnavailable},
		{"盘不存在", gorm.ErrRecordNotFound, errcode.CloudDriveNotFound.Code, http.StatusNotFound},
		{"上游失败", errors.New("dial tcp 1.1.1.1:443: i/o timeout"), errcode.CloudOpUpstreamFailed.Code, http.StatusBadGateway},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			cloudOpErr(c, tc.err)
			var env map[string]any
			if e := json.Unmarshal(rec.Body.Bytes(), &env); e != nil {
				t.Fatalf("响应不是 JSON：%q", rec.Body.String())
			}
			if rec.Code != tc.status || env["code"] != tc.code {
				t.Fatalf("status=%d code=%v，期望 %d + %q", rec.Code, env["code"], tc.status, tc.code)
			}
			if msg, _ := env["message"].(string); strings.Contains(msg, "dial tcp") {
				t.Errorf("上游原文不该进界面：%q", msg)
			}
		})
	}
	// 批量操作的兜底必须交代「可能已改动一半」。
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	cloudOpErr(c, errors.New("cloud: partial"))
	if msg := rec.Body.String(); !strings.Contains(msg, "以云盘页面为准") {
		t.Errorf("批量写操作失败要说清核对口径：%s", msg)
	}
}
