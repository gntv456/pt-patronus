package v1

// #342 「按 id 取站点」失败一律报 SiteNotFound，15 处出口各抄一份（#341 的放大版）。
//
// SiteOper.Get 底层是 db.First，两种完全不同的失败被压成同一句「站点不存在」：
//   - 库里真没这条记录  → 该刷新列表（404 正确）
//   - DB 真故障（磁盘满/连接断/锁）→ 也是 404「站点不存在」
// 用户在消息/站务/勋章/站点搜索里点任何一个，数据库一挂就看到「站点不存在」，
// 于是去站点列表里翻——那页也是同一个故障，于是更困惑。
//
// 形状在 15 处重复（assessment/freeleech/harem/medalwall/message/site_search/site…），
// 每处各抄一遍，与 filter_rule.go 的 respondFilterErr、input_error.go 的
// respondRecordError 是同一种病：判据没有单一来源，于是改的时候必然漏。
//
// 修法：respondSiteLookup —— 「查站点」这一动作的三段归类，各出口统一走它。

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// TestSiteLookupHasSingleClassifier 必须有统一判据；没有就等于 15 处继续各抄一份。
func TestSiteLookupHasSingleClassifier(t *testing.T) {
	src, err := os.ReadFile("input_error.go")
	if err != nil {
		t.Fatalf("读不到 input_error.go：%v", err)
	}
	code := stripCommentsForTest(string(src))
	if !strings.Contains(code, "func respondSiteLookup(") {
		t.Fatal("缺 respondSiteLookup：15 处「查站点失败」各抄一份判据，改的时候必漏")
	}
	// 判据本身必须三段齐全：record not found → 404；其余内部错误 → 500 且带原因。
	for _, want := range []string{"gorm.ErrRecordNotFound", "errcode.SiteNotFound", "WithErr"} {
		if !strings.Contains(code, want) {
			t.Errorf("respondSiteLookup 缺 %s（判据不完整就不是单一来源）", want)
		}
	}
}

// TestSiteLookupOutletsUseIt 15 处出口必须走统一判据，不许再裸 Respond(SiteNotFound)。
func TestSiteLookupOutletsUseIt(t *testing.T) {
	files := []string{"assessment.go", "freeleech.go", "harem.go", "medalwall.go", "message.go", "site_search.go", "site.go"}
	// 逐个文件统计「裸 Respond(SiteNotFound)」；有就点名到行。
	offenders := []string{}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("读不到 %s：%v", f, err)
		}
		code := stripCommentsForTest(string(src))
		lines := strings.Split(code, "\n")
		for i, L := range lines {
			// 形如：  errcode.Respond(c, errcode.SiteNotFound)  ← 站点查找失败那一处
			if !strings.Contains(L, "errcode.SiteNotFound)") {
				continue
			}
			// 站点「查不到」那一档必须改走 respondSiteLookup / respondRecordError；
			// 但入参校验里的 SiteNotFound 属正常用法（那不是查库失败）。
			prev := ""
			if i > 0 {
				prev = strings.TrimSpace(lines[i-1])
			}
			if prev == "if err != nil {" {
				offenders = append(offenders, f+":"+strconv.Itoa(i+1))
			}
		}
	}
	if len(offenders) > 0 {
		t.Errorf("这些「查站点失败」出口还裸 Respond(SiteNotFound)：%s\n"+
			"（DB 故障会被说成「站点不存在」；应统一走 respondSiteLookup）", strings.Join(offenders, ", "))
	}
}

// TestSiteLookupKeeps404ForMissing 反向对照：真·查不到仍要 404 + site_not_found。
func TestSiteLookupKeeps404ForMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/site-lookup.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.Site{}); err != nil {
		t.Fatal(err)
	}
	op := oper.NewSiteOper(db, security.NewCrypto("site-lookup-seed"))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(http.MethodGet, "/api/v1/sites/999999", nil)
	if _, err := op.Get(999999); err != nil {
		respondSiteLookup(c, err, op)
	} else {
		t.Fatal("前提不成立：999999 号站点不该存在")
	}
	if w.Code != http.StatusNotFound {
		t.Fatalf("查不到站点应 404，got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "site_not_found") {
		t.Errorf("应回本域码 got %s", w.Body.String())
	}
}

// TestSiteLookupCodesStayDistinct 专属码与兜底码不得同文案（换了码却还是同一句话＝没修）。
func TestSiteLookupCodesStayDistinct(t *testing.T) {
	if errcode.SiteNotFound.Message == errcode.Internal.Message {
		t.Errorf("SiteNotFound 与 Internal 文案相同：%q", errcode.SiteNotFound.Message)
	}
}
