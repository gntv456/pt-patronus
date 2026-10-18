package chain

import (
	"context"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/errtext"
	"github.com/hxpt/ptpatronus/internal/site"
)

// 站免/公告页的两种「什么都没有」必须分开：
//   - 站点定义没开这个 capability（228 个内置定义里约 75 个是这样）→ 从没去抓过；
//   - 抓到了，当前确实没有站免/公告 → 站点现状。
// 后端以前两者都只给空数组，前端 Freeleech.vue 于是把每一行空结果都标成
// 「该站点未开启站免检测」——对接了线的站也被说成没开检测。

func flRowByName(t *testing.T, rows []FreeleechResult, name string) *FreeleechResult {
	t.Helper()
	for i := range rows {
		if rows[i].SiteName == name {
			return &rows[i]
		}
	}
	t.Fatalf("结果里没有站 %q: %+v", name, rows)
	return nil
}

func annRowByName(t *testing.T, rows []AnnouncementResult, name string) *AnnouncementResult {
	t.Helper()
	for i := range rows {
		if rows[i].SiteName == name {
			return &rows[i]
		}
	}
	t.Fatalf("结果里没有站 %q: %+v", name, rows)
	return nil
}

func siteByName(t *testing.T, siteOp *oper.SiteOper, name string) *models.Site {
	t.Helper()
	sites, err := siteOp.ListActive()
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	for i := range sites {
		if sites[i].Name == name {
			return &sites[i]
		}
	}
	t.Fatalf("站点清单里没有 %q", name)
	return nil
}

// TestFreeleechMarksUnwiredSite 未接线的站要标 Unsupported，且两条出口（回源 + 缓存命中）同口径。
func TestFreeleechMarksUnwiredSite(t *testing.T) {
	ch, siteOp := newSearchChain(t, &fakeFLAdapter{
		fakeAdapter: &fakeAdapter{schema: "nexusphp"},
		notices:     []site.SiteFreeleech{{Title: "FREE"}},
	})
	createActive(t, siteOp, "nexus", "nexusphp")
	createActive(t, siteOp, "unit", "unit3d")

	for _, force := range []bool{true, false} {
		rows, err := ch.FreeleechAll(context.Background(), force)
		if err != nil {
			t.Fatal(err)
		}
		if u := flRowByName(t, rows, "unit"); !u.Unsupported {
			t.Errorf("force=%v 未接线站必须标 Unsupported，got %+v", force, u)
		} else if u.Error != "" {
			t.Errorf("未接线不是故障，不该写进 Error，got %q", u.Error)
		} else if len(u.Notices) != 0 {
			t.Errorf("未接线行的 notices 必须是空数组，got %+v", u.Notices)
		}
		if n := flRowByName(t, rows, "nexus"); n.Unsupported || len(n.Notices) != 1 {
			t.Errorf("已接线站不该受影响，got %+v", n)
		}
	}
}

// TestFreeleechWiredButEmptyIsNotUnsupported 反向对照：接了线、这一轮真的没有促销，
// 不能被新字段一并打成「未适配」（否则修复只是把误标换了个方向）。
func TestFreeleechWiredButEmptyIsNotUnsupported(t *testing.T) {
	ch, siteOp := newSearchChain(t, &fakeFLAdapter{fakeAdapter: &fakeAdapter{schema: "nexusphp"}})
	createActive(t, siteOp, "nexus", "nexusphp")
	rows, err := ch.FreeleechAll(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	r := flRowByName(t, rows, "nexus")
	if r.Unsupported {
		t.Errorf("已接线但当前无促销的站不该标 Unsupported，got %+v", r)
	}
	if r.Notices == nil {
		t.Error("notices 必须归一成空数组")
	}
}

// TestSyncFreeleechRowConsistentWithAggregate 单站刷新面（/freeleech/sync/:id）与聚合面同口径：
// 未接线 → Unsupported；抓取失败 → 走中文翻译口而不是把 "context deadline exceeded" 弹给用户。
func TestSyncFreeleechRowConsistentWithAggregate(t *testing.T) {
	dead := context.DeadlineExceeded
	ch, siteOp := newSearchChain(t, &fakeFLAdapter{
		fakeAdapter: &fakeAdapter{schema: "nexusphp"},
		err:         dead,
	})
	createActive(t, siteOp, "nexus", "nexusphp")
	createActive(t, siteOp, "unit", "unit3d")
	wired, unwired := siteByName(t, siteOp, "nexus"), siteByName(t, siteOp, "unit")

	if n := ch.SyncFreeleech(context.Background(), wired); n.Unsupported || n.Error != errtext.Timeout {
		t.Errorf("已接线站失败要给中文原因，got %+v want %q", n, errtext.Timeout)
	}
	if u := ch.SyncFreeleech(context.Background(), unwired); !u.Unsupported || u.Error != "" {
		t.Errorf("单站刷新面也要标未接线、且不能记成故障，got %+v", u)
	}
}

// TestAnnouncementMarksUnwiredSite 公告聚合两条出口的未接线判定。
func TestAnnouncementMarksUnwiredSite(t *testing.T) {
	ch, siteOp := newSearchChain(t, &fakeAnnAdapter{
		fakeAdapter: &fakeAdapter{schema: "nexusphp"},
		ann:         &site.SiteAnnouncement{Items: []site.AnnouncementItem{{Title: "A"}}},
	})
	createActive(t, siteOp, "nexus", "nexusphp")
	createActive(t, siteOp, "unit", "unit3d")

	for _, force := range []bool{true, false} {
		rows, err := ch.AnnouncementsAll(context.Background(), force)
		if err != nil {
			t.Fatal(err)
		}
		if u := annRowByName(t, rows, "unit"); !u.Unsupported || u.Error != "" {
			t.Errorf("force=%v 公告未接线站要标 Unsupported 且不记故障，got %+v", force, u)
		}
		if n := annRowByName(t, rows, "nexus"); n.Unsupported || len(n.Items) != 1 {
			t.Errorf("已接线公告站不该受影响，got %+v", n)
		}
	}
}

// TestAnnouncementCacheHitMarksUnwired 公告缓存命中出口（ann != nil 才走这条）。
// 正常流程里未接线的站不会缓存到非 nil 公告（抓取被 capability 守卫短路），
// 这条是纵深防御：用户把某站定义改成关闭公告、而旧缓存还在时，行不能退化。
func TestAnnouncementCacheHitMarksUnwired(t *testing.T) {
	ch, siteOp := newSearchChain(t, &fakeAnnAdapter{fakeAdapter: &fakeAdapter{schema: "nexusphp"}})
	createActive(t, siteOp, "unit", "unit3d")
	s := siteByName(t, siteOp, "unit")
	ch.annMu.Lock()
	ch.annCache[s.ID] = annCacheEntry{ann: &site.SiteAnnouncement{
		SiteID: s.ID, SiteName: s.Name, FetchedAt: time.Now(),
		Items: []site.AnnouncementItem{{Title: "旧缓存"}},
	}, at: time.Now()}
	ch.annMu.Unlock()

	rows, err := ch.AnnouncementsAll(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	r := annRowByName(t, rows, "unit")
	if !r.Unsupported {
		t.Errorf("缓存命中出口也要标 Unsupported，got %+v", r)
	}
}

// TestSyncAnnouncementRowConsistentWithAggregate 公告单站刷新面的中文原因 + 未接线判定。
func TestSyncAnnouncementRowConsistentWithAggregate(t *testing.T) {
	ch, siteOp := newSearchChain(t, &fakeAnnAdapter{
		fakeAdapter: &fakeAdapter{schema: "nexusphp"},
		err:         context.DeadlineExceeded,
	})
	createActive(t, siteOp, "nexus", "nexusphp")
	createActive(t, siteOp, "unit", "unit3d")
	if n := ch.SyncAnnouncement(context.Background(), siteByName(t, siteOp, "nexus")); n.Unsupported || n.Error != errtext.Timeout {
		t.Errorf("已接线公告站失败要给中文原因，got %+v want %q", n, errtext.Timeout)
	}
	u := ch.SyncAnnouncement(context.Background(), siteByName(t, siteOp, "unit"))
	if !u.Unsupported || u.Error != "" {
		t.Errorf("公告单站刷新面也要标未接线，got %+v", u)
	}
	if u.Items == nil {
		t.Error("items 必须归一成空数组")
	}
	if u.FetchedAt.IsZero() {
		t.Error("未接线的公告行也应带抓取时间（前端按它排序）")
	}
}
