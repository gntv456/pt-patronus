package v1

// #367 Status 的第二次 List()（补 site_name）失败被静默吞掉，站名整列为空，
// 而前端拿这个字段做**集合成员判断**——于是真实告警被误判为「已经报过了」。
//
// Status 的形状：
//
//	list, err := h.oper.ListConnectivity()      ← 失败会 500（对的）
//	names := map[uint]string{}
//	if sites, err := h.oper.List(); err == nil { ← 失败被静默丢弃
//		for _, s := range sites { names[s.ID] = s.Name }
//	}
//	…
//	"site_name": names[r.SiteID],   ← 查不到就是空串
//
// SiteConnectivity（models/site.go:67）根本没有站名字段，站名必须靠这次 List 关联。
// 所以这次 List 一失败，**整列 site_name 变空**，而 HTTP 仍是 200 —— 典型的假绿。
//
// 后果不止「界面上少了站名」，前端还有两处拿它做成员判断：
//
//	useDashboardActions.ts:53
//	  cookieAlerts.value.includes(status.site_name)
//	useSiteMatrixData.ts:139
//	  props.cookieAlerts.includes(row.site_name)
//
// 空站名一旦进到 cookieAlerts 的判定里，所有「站名为空」的未联通项都会被
// 当成「已经有 Cookie 告警了」而 continue 掉 —— 真告警被吞，界面显示一切正常。
//
// 判据：真的关掉数据库让第二次 List 失败，看 site_name 是否为空且状态码仍 200。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// TestSiteStatusNameLookupFailureIsVisible 第二次 List() 失败不许静默丢站名。
func TestSiteStatusNameLookupFailureIsVisible(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/status.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Site{}, &models.SiteConnectivity{}); err != nil {
		t.Fatal(err)
	}
	so := oper.NewSiteOper(db, security.NewCrypto("k"))
	s, err := so.Create(oper.SiteInput{Name: "馒头", Platform: "nexusphp", BaseURL: "https://x.test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.SiteConnectivity{
		SiteID: s.ID, Status: "disconnected", ErrorMsg: "连接超时", CheckedAt: time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}

	r := gin.New()
	h := &SiteHandler{oper: so}
	r.GET("/sites/status", h.Status)

	// 正常读：site_name 必须有值（正控，先证明这条路径本来是好的）。
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sites/status", nil))
	var env struct {
		Statuses []struct {
			SiteID   uint   `json:"site_id"`
			SiteName string `json:"site_name"`
			Error    string `json:"error"`
		} `json:"statuses"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("响应不是 JSON：%q", rec.Body.String())
	}
	if len(env.Statuses) != 1 || env.Statuses[0].SiteName != "馒头" {
		t.Fatalf("正控：正常读取该带出站名，got %+v", env.Statuses)
	}

	// 现在关掉连接，让后续查询失败（模拟连接池断/库不可用）。
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}

	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/sites/status", nil))

	// #367 关键：ListConnectivity 也会失败 ⇒ 500（这条是对的，界面知道整份数据读不到）。
	// 但若实现改成「先读缓存、再单独补名」，补名失败就必须显式报出来，
	// 不能回 200 + 一列空站名（前端会拿空串做成员判断，把真告警吞掉）。
	if rec2.Code == http.StatusOK {
		var env2 struct {
			Statuses []struct {
				SiteName string `json:"site_name"`
			} `json:"statuses"`
		}
		_ = json.Unmarshal(rec2.Body.Bytes(), &env2)
		for _, st := range env2.Statuses {
			if st.SiteName == "" {
				t.Fatalf("库读不到时回 200 + 空站名（旧形状：前端拿空串做成员判断会吞掉真告警）：%q",
					rec2.Body.String())
			}
		}
	}
}

// TestSiteStatusNeverEmitsEmptySiteName 无论什么路径，站名列都不许整列为空却仍报 200。
func TestSiteStatusNeverEmitsEmptySiteName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/status2.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Site{}, &models.SiteConnectivity{}); err != nil {
		t.Fatal(err)
	}
	// 关连接，否则 t.TempDir() 清理时会因文件锁失败（另一条测试已经踩过一次）。
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	so := oper.NewSiteOper(db, security.NewCrypto("k"))
	s, err := so.Create(oper.SiteInput{Name: "备用", Platform: "nexusphp", BaseURL: "https://y.test"})
	if err != nil {
		t.Fatal(err)
	}
	// 造一条真实的孤儿行：先写联通缓存，再把站点硬删掉。
	// 站点硬删（oper/site.go:280 `db.Delete(&models.Site{}, id)`）是无级联的，
	// 不会连带删 site_connectivity —— 于是 Status 里这条就永远查不到站名。
	// 这条路径不需要任何异常：正常使用中「删掉一个站点」就会留下它。
	if err := db.Create(&models.SiteConnectivity{
		SiteID: s.ID, Status: "disconnected", ErrorMsg: "已删除站点的残留检测", CheckedAt: time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := so.Delete(s.ID); err != nil {
		t.Fatalf("删站点失败：%v", err)
	}
	if err := db.Create(&models.SiteConnectivity{
		SiteID: 4242, Status: "disconnected", ErrorMsg: "另一个孤儿", CheckedAt: time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	_ = s

	r := gin.New()
	h := &SiteHandler{oper: so}
	r.GET("/sites/status", h.Status)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sites/status", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("正控：本例应 200，got %d：%s", rec.Code, rec.Body.String())
	}
	var env struct {
		Statuses []struct {
			SiteID   uint   `json:"site_id"`
			SiteName string `json:"site_name"`
		} `json:"statuses"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	for _, st := range env.Statuses {
		if st.SiteName == "" {
			t.Errorf("site_id=%d 的 site_name 是空串：前端两处拿它做 includes() 判断，"+
				"空串会让真告警被当成「已报过」而吞掉", st.SiteID)
		}
	}
}

// TestSiteStatusNameLookupMustNotBeSilentlyDropped 接线护栏：补站名那次 List() 失败必须
// 显式报错，不许 `if err == nil` 静默跳过。
//
// 之所以要单列一条：上面两条行为测试只覆盖了「孤儿行」与「整体读不到」，
// 把 List 换回 `if err == nil` 时它们**依然全绿**（孤儿行走 siteDisplayName 兜底，
// 整体读不到时 ListConnectivity 先失败已 500）—— 分类对了、失败分支没接上。
func TestSiteStatusNameLookupMustNotBeSilentlyDropped(t *testing.T) {
	src, err := os.ReadFile("site.go")
	if err != nil {
		t.Fatalf("读不到 site.go：%v", err)
	}
	// 必须剥掉整行注释：本函数上方就引用了旧形状的字面文本（`if sites, err := h.oper.List(); err == nil {…}`），
	// 不剥就会把判据本身当成被测代码 —— 源码扫描式判据最常见的一类自伤。
	code := stripWholeLineComments(string(src))
	at := strings.Index(code, "func (h *SiteHandler) Status(")
	if at < 0 {
		t.Fatal("找不到 Status（判据没有落点）")
	}
	tail := code[at:]
	if n := strings.Index(tail[1:], "\nfunc "); n > 0 {
		tail = tail[:1+n]
	}
	if strings.Contains(tail, "if sites, err := h.oper.List(); err == nil") {
		t.Error("Status 不许静默吞掉补站名那次 List 的失败（旧形状：回 200 + 空站名列，" +
			"前端拿空串做 includes() 判断会把真告警吞掉")
	}
	if !strings.Contains(tail, "siteDisplayName(names, r.SiteID)") {
		t.Error("孤儿行（站点已硬删、缓存仍在）必须用 siteDisplayName 兜底，绝不吐空串")
	}
}

// stripWholeLineComments 去掉整行注释（trim 后以 // 开头的行），避免判据匹配到自己写的说明。
func stripWholeLineComments(s string) string {
	out := make([]string, 0, 128)
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}
