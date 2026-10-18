package chain

import (
	"context"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// seedStat 写一行站点快照（uploaded/downloaded 都是累计值）。
func seedStat(t *testing.T, db *gorm.DB, siteID uint, at time.Time, up, down int64) {
	t.Helper()
	if err := db.Create(&models.SiteStat{SiteID: siteID, SnapAt: at, Uploaded: up, Downloaded: down}).Error; err != nil {
		t.Fatalf("seed stat: %v", err)
	}
}

func workbenchDelta(t *testing.T, db *gorm.DB) (int64, int64) {
	t.Helper()
	c := &DashboardChain{db: db}
	w, err := c.Workbench(context.Background(), nil)
	if err != nil {
		t.Fatalf("Workbench: %v", err)
	}
	if len(w.Errors) != 0 {
		t.Fatalf("健康库不应有读取错误：%v", w.Errors)
	}
	return w.Summary.UploadedDelta, w.Summary.DownloadedDelta
}

// TestWorkbenchTodayDeltaUsesLatestRowPerSite 回归：今日增量原先对一天内的每一行求和。
// uploaded 是累计量，4h cron 一天写 6 行 → 今天被算 6 遍；只要今天的行数少于昨天，
// 差值就是大负数，前端 max(0,…) 钳位后「今日上传」恒显 0 B——用户明明涨了流量却看到 0。
func TestWorkbenchTodayDeltaUsesLatestRowPerSite(t *testing.T) {
	db := newWorkbenchDB(t)
	var sites []models.Site
	if err := db.Order("id ASC").Find(&sites).Error; err != nil {
		t.Fatal(err)
	}
	if len(sites) != 2 {
		t.Fatalf("需要 2 个站点，got %d", len(sites))
	}
	a, b := sites[0].ID, sites[1].ID

	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	ydayStart := dayStart.AddDate(0, 0, -1)

	// 昨天：a 站最后一行 80、b 站 5（昨天各站最新量 = 85）
	seedStat(t, db, a, ydayStart.Add(time.Hour), 60, 600)
	seedStat(t, db, a, ydayStart.Add(2*time.Hour), 80, 700)
	seedStat(t, db, b, ydayStart.Add(time.Hour), 5, 50)
	// 今天：a 站 cron 写了 3 行（100/200/300），b 站写了 2 行（10/20）
	seedStat(t, db, a, dayStart.Add(time.Hour), 100, 800)
	seedStat(t, db, a, dayStart.Add(2*time.Hour), 200, 900)
	seedStat(t, db, a, dayStart.Add(3*time.Hour), 300, 1000)
	seedStat(t, db, b, dayStart.Add(time.Hour), 10, 100)
	seedStat(t, db, b, dayStart.Add(4*time.Hour), 20, 120)

	// 期望：今天各站最新行 300+20=320，昨天 80+5=85 → 增量 235；下载 (1000+120)-(700+50)=370
	up, down := workbenchDelta(t, db)
	if up != 235 {
		t.Fatalf("今日上传增量应按各站当天最后一行相减，got %d（想要 235；把一天每行都加起来是 630-145=485 这类乱数）", up)
	}
	if down != 370 {
		t.Fatalf("今日下载增量同理，got %d（想要 370）", down)
	}
}

// TestWorkbenchTodayDeltaIgnoresFutureDay 时间戳比今天更晚的行（时钟/时区偏差、从旧备份回写过）
// 不得顶替昨天：只认正好落在 today / yesterday 的行。
// 旧写法是「凡不是今天的行就算昨天」，LIMIT 2 + day DESC 下第一行正是那条未来的行，
// 于是今天的量减去一条更晚的累计量，差值变负、前端钳位成 0 B。
func TestWorkbenchTodayDeltaIgnoresFutureDay(t *testing.T) {
	db := newWorkbenchDB(t)
	var sites []models.Site
	if err := db.Order("id ASC").Find(&sites).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	seedStat(t, db, sites[0].ID, dayStart.Add(time.Hour), 300, 20)                     // 今天
	seedStat(t, db, sites[0].ID, dayStart.AddDate(0, 0, 1).Add(time.Hour), 5000, 5000) // 明天（时钟偏差）

	up, down := workbenchDelta(t, db)
	if up != 300 {
		t.Fatalf("未来的行不得当昨天基线，got up=%d（想要 300；当成昨天是 300-5000=-4700）", up)
	}
	if down != 20 {
		t.Fatalf("同上，got down=%d（想要 20）", down)
	}
}

// TestWorkbenchTodayDeltaIgnoresOlderThanYesterday 只有今天 + 前天的行时，前天不得顶替昨天：
// 否则今天的累计量会减去前天的累计量，把两天的量当成一天。
func TestWorkbenchTodayDeltaIgnoresOlderThanYesterday(t *testing.T) {
	db := newWorkbenchDB(t)
	var sites []models.Site
	if err := db.Order("id ASC").Find(&sites).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	seedStat(t, db, sites[0].ID, dayStart.AddDate(0, 0, -2), 1000, 1000)                // 前天
	seedStat(t, db, sites[0].ID, dayStart.AddDate(0, 0, -2).Add(time.Hour), 1200, 1100) // 前天更晚一行
	seedStat(t, db, sites[0].ID, dayStart.Add(time.Hour), 300, 20)                      // 今天唯一一行

	// 没有昨天的行 → 昨天基线为 0，增量取今天最新累计量（产品口径：首日无对比=当日总量）
	up, down := workbenchDelta(t, db)
	if up != 300 {
		t.Fatalf("前天的行不得当成昨天，got up=%d（想要 300；错把 1200 当昨天会得到负数）", up)
	}
	if down != 20 {
		t.Fatalf("同上，got down=%d（想要 20）", down)
	}
}
