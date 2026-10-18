package automation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/errtext"
	"github.com/hxpt/ptpatronus/internal/notification"
	"github.com/hxpt/ptpatronus/internal/site"
	"gorm.io/gorm"
)

// fakeHRAdapter 实现 site.HRCapable：按站点 baseURL 决定返回危险条目 / 报错。
type fakeHRAdapter struct {
	schema   string
	torrents []site.HRTorrent
	err      error
	failHost string // baseURL 含此串时只报错（让同批里既有答话的站也有失败的站）
}

func (f *fakeHRAdapter) Name() string   { return "fake-hr" }
func (f *fakeHRAdapter) Priority() int  { return 100 }
func (f *fakeHRAdapter) Schema() string { return f.schema }
func (f *fakeHRAdapter) Search(context.Context, *site.SiteMetadata, string, string, string, string, int) ([]site.Torrent, error) {
	return nil, nil
}
func (f *fakeHRAdapter) FetchUserInfo(context.Context, *site.SiteMetadata, string, string) (*site.UserInfo, error) {
	return &site.UserInfo{}, nil
}
func (f *fakeHRAdapter) FetchHR(_ context.Context, _ *site.SiteMetadata, baseURL, _ string) ([]site.HRTorrent, error) {
	if f.failHost != "" && !strings.Contains(baseURL, f.failHost) {
		return f.torrents, nil // 点名只让某一站失败，其余照常答话
	}
	return f.torrents, f.err
}

// hrCaptureChannel 收全部通知（要断言「同一天只自曝一次」，单条 last 不够）。
type hrCaptureChannel struct{ msgs []notification.Message }

func (c *hrCaptureChannel) Name() string { return "hr-capture" }
func (c *hrCaptureChannel) Send(_ context.Context, msg notification.Message) error {
	c.msgs = append(c.msgs, msg)
	return nil
}

// newHRService 造一个只跑 H&R 的 automation.Service；defs 里的站用 hrsite 定义（hr.enabled: true），
// plain 里的站用默认定义（内置 228 个定义大多没开 hr.enabled，这正是漏检的常态）。
func newHRService(t *testing.T, plain, defs []string, adapter *fakeHRAdapter) (*Service, *hrCaptureChannel) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/hr.db"), &gorm.Config{})
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
	siteOp := oper.NewSiteOper(db, security.NewCrypto("hr-test-seed"))
	for _, name := range plain {
		if _, err := siteOp.Create(oper.SiteInput{
			Name: name, Platform: "nexusphp", BaseURL: "https://" + name + ".example", IsActive: true,
		}); err != nil {
			t.Fatal(err)
		}
	}
	reg := site.NewRegistry()
	reg.AddDefinition(&site.SiteMetadata{ID: "hrsite", Name: "HR 站", Schema: "nexusphp", HR: site.HRDef{Enabled: true}})
	for _, name := range defs {
		if _, err := siteOp.Create(oper.SiteInput{
			Name: name, Platform: "nexusphp", Definition: "hrsite", BaseURL: "https://" + name + ".example", IsActive: true,
		}); err != nil {
			t.Fatal(err)
		}
	}
	mm := module.New()
	if adapter != nil {
		mm.Register(adapter)
	}
	siteCh := chain.NewSiteChain(chain.NewBase(mm, event.New()), siteOp, reg)
	capCh := &hrCaptureChannel{}
	nc := chain.NewNotificationChain(chain.NewBase(module.New(), event.New()), notification.NewManager(capCh))
	return &Service{siteCh: siteCh, siteOp: siteOp, notif: nc, rootCtx: context.Background()}, capCh
}

// TestRunHRCheckAllUnwiredSelfExposes 一个站都没接线时不能计「干净」后静默：
// 必须自曝「没有一个站点被真正检测」并教用户开 hr.enabled，且同一天第二次跑不再重复刷屏。
func TestRunHRCheckAllUnwiredSelfExposes(t *testing.T) {
	s, capCh := newHRService(t, []string{"站A", "站B"}, nil, &fakeHRAdapter{schema: "nexusphp"})
	s.runHRCheck()
	if len(capCh.msgs) != 1 {
		t.Fatalf("未接线常态应自曝 1 条，got %d: %+v", len(capCh.msgs), capCh.msgs)
	}
	m := capCh.msgs[0]
	if m.Level != "warning" || !strings.Contains(m.Body, "没有一个站点被真正检测") ||
		!strings.Contains(m.Body, "hr.enabled") {
		t.Fatalf("自曝文案要含漏检与可操作提示，got %+v", m)
	}
	// 小时级任务：同一天第二轮回跳通知不能再来一条。
	s.runHRCheck()
	if len(capCh.msgs) != 1 {
		t.Fatalf("同日第二轮不该重复自曝，got %d 条", len(capCh.msgs))
	}
}

// TestRunHRCheckSurfacesFetchFailures 接线站抓取失败（过去只留 Debug 然后当成干净）必须出声，
// 且原因走中文翻译口而不是英文原文。
func TestRunHRCheckSurfacesFetchFailures(t *testing.T) {
	s, capCh := newHRService(t, nil, []string{"站A"}, &fakeHRAdapter{schema: "nexusphp", err: context.DeadlineExceeded})
	s.runHRCheck()
	if len(capCh.msgs) != 1 {
		t.Fatalf("读取失败要自曝 1 条，got %+v", capCh.msgs)
	}
	body := capCh.msgs[0].Body
	if !strings.Contains(body, "1 站读取失败") {
		t.Fatalf("账面要含失败站数，got %q", body)
	}
	if !strings.Contains(body, errtext.Timeout) || strings.Contains(body, "context deadline exceeded") {
		t.Fatalf("原因要中文、不留英文原文，got %q", body)
	}
}

// TestRunHRCheckStillAlertsDangerous 有危险种子时照常告警；接线的站都答了话就不该追加漏检噪音
// （未接线的站由各自界面标注，不进 H&R 告警正文）。
func TestRunHRCheckStillAlertsDangerous(t *testing.T) {
	s, capCh := newHRService(t, []string{"未接线站"}, []string{"危险站"}, &fakeHRAdapter{
		schema:   "nexusphp",
		torrents: []site.HRTorrent{{Title: "某种未达标", Status: "未达标", DeadlineLeft: "3 天"}},
	})
	s.runHRCheck()
	if len(capCh.msgs) != 1 {
		t.Fatalf("危险种子应只发 1 条告警，got %+v", capCh.msgs)
	}
	body := capCh.msgs[0].Body
	if !strings.Contains(body, "某种未达标") {
		t.Fatalf("告警正文要含种子，got %q", body)
	}
	if strings.Contains(body, "另：") {
		t.Fatalf("已检测的站答了话就不该有漏检附注，got %q", body)
	}
}

// TestRunHRCheckAppendsGapToDangerAlert 危险告警里也要带上「另有 N 站没答话」，
// 否则用户看到 1 条告警会以为其余站都被查过。
func TestRunHRCheckAppendsGapToDangerAlert(t *testing.T) {
	s, capCh := newHRService(t, nil, []string{"危险站", "坏站"}, &fakeHRAdapter{
		schema:   "nexusphp",
		torrents: []site.HRTorrent{{Title: "某种未达标", Status: "未达标"}},
		err:      errors.New("boom"),
		failHost: "坏站",
	})
	s.runHRCheck()
	if len(capCh.msgs) != 1 {
		t.Fatalf("应发 1 条危险告警，got %+v", capCh.msgs)
	}
	body := capCh.msgs[0].Body
	if !strings.Contains(body, "某种未达标") || !strings.Contains(body, "1 站读取失败") {
		t.Fatalf("危险告警要同时带漏检账面，got %q", body)
	}
}
