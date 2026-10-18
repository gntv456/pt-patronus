package chain

// #386 SaveSetting 把「查不到这条设置」与「查不动这张表」压成同一个 row.ID==0。
//
// keepseed_setting.go 的形状：
//
//	func (c *KeepSeedChain) Setting(id uint) (models.CrossSeedSetting, error) {
//	    err := c.db.Where("downloader_id = ?", id).First(&s).Error
//	    if errors.Is(err, gorm.ErrRecordNotFound) {   // ← 分得清
//	        return 默认值, nil
//	    }
//	    return s, err
//	}
//
//	func (c *KeepSeedChain) SaveSetting(s models.CrossSeedSetting) error {
//	    var row models.CrossSeedSetting
//	    c.db.Where("downloader_id = ?", s.DownloaderID).First(&row)   // ← 错误整个丢掉
//	    if row.ID == 0 {
//	        c.db.Create(&s)      // ← 于是 DB 读故障被当成「没有这条设置」→ 走插入
//	    }
//	    ...
//	}
//
// 同一个文件、两个函数、两种态度：Setting 分得清「不存在」与「真故障」，SaveSetting 丢掉了。
//
// 后果：当 DB 只是**读**故障（连接池打满、SQLite 的 database is locked 在读路径、
// 只读副本滞后、gorm 的 prepared statement 失效…）而写路径仍通时，
// SaveSetting 会往表里**再插一行**同一个 downloader_id。此后：
//   - Setting 用 First 取到的是哪一行不确定（读故障修好之后）；
//   - 界面「设置」页保存/读取开始飘；
//   - ListAutoReseedSettings 会拿到重复项，cron 对同一台重复触发。
// 而用户的直觉解释永远是「保存失败了」，根本想不到是重复行。
//
// 这与 #345/#342 治的「存在性与真故障分不开」同族，只是后果从「说错话」升级成「写坏数据」。

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// failFirstOnce 让下一次 First 查询返回 readErr，模拟「DB 读不动但还能写」。
func failFirstOnce(t *testing.T, db *gorm.DB, readErr error) {
	t.Helper()
	name := "fail_first_once"
	err := db.Callback().Query().Before("gorm:query").Register(name, func(d *gorm.DB) {
		_ = d.AddError(readErr)
	})
	if err != nil {
		t.Fatalf("注册查询回调失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove(name) })
}

// TestSaveSettingDoesNotInsertOnReadFailure 读故障时必须报错，不许当成「没有这条设置」去插入。
func TestSaveSettingDoesNotInsertOnReadFailure(t *testing.T) {
	db := newReseedDB(t)
	ch := &KeepSeedChain{db: db}

	// 先正常存一条，建立「这台已有设置」的事实。
	if err := ch.SaveSetting(models.CrossSeedSetting{DownloaderID: 7, SavePath: "/data/seed", PauseAfterAdd: true}); err != nil {
		t.Fatalf("预置设置失败: %v", err)
	}
	var before int64
	if err := db.Model(&models.CrossSeedSetting{}).Where("downloader_id = ?", 7).Count(&before).Error; err != nil {
		t.Fatal(err)
	}

	// 现在让查询读不动，但写仍可用。
	failFirstOnce(t, db, errors.New("sql: database is locked"))

	err := ch.SaveSetting(models.CrossSeedSetting{DownloaderID: 7, SavePath: "/data/other", PauseAfterAdd: false})
	if err == nil {
		t.Fatalf("查询读不动却返回 nil 错误 —— 旧形状会把它当「没有这条设置」再插一行")
	}
	if !strings.Contains(err.Error(), "database is locked") {
		t.Errorf("原因必须带出来（用户/运维靠它判断是 DB 问题还是自己填错），got %q", err.Error())
	}

	// 撤销故障后核对行数：绝不能多出一行。
	_ = db.Callback().Query().Remove("fail_first_once")
	var after int64
	if e := db.Model(&models.CrossSeedSetting{}).Where("downloader_id = ?", 7).Count(&after).Error; e != nil {
		t.Fatal(e)
	}
	if after != before {
		t.Errorf("行数从 %d 变成 %d —— 读故障被当成「不存在」导致重复插入", before, after)
	}
}

// TestSaveSettingUpsertStillWorks 正常路径不许回归：真没有记录时要能插入，有记录时要能改。
func TestSaveSettingUpsertStillWorks(t *testing.T) {
	db := newReseedDB(t)
	ch := &KeepSeedChain{db: db}

	// 首次：不存在 → 插入。
	if err := ch.SaveSetting(models.CrossSeedSetting{DownloaderID: 9, SavePath: "/a", PauseAfterAdd: true}); err != nil {
		t.Fatalf("首次保存失败: %v", err)
	}
	// 二次：已存在 → 更新，不能再插。
	if err := ch.SaveSetting(models.CrossSeedSetting{DownloaderID: 9, SavePath: "/b", PauseAfterAdd: false}); err != nil {
		t.Fatalf("二次保存失败: %v", err)
	}
	var n int64
	if err := db.Model(&models.CrossSeedSetting{}).Where("downloader_id = ?", 9).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("同一 downloader_id 应只有 1 行，实测 %d 行（upsert 坏了）", n)
	}
	got, err := ch.Setting(9)
	if err != nil {
		t.Fatal(err)
	}
	if got.SavePath != "/b" || got.PauseAfterAdd {
		t.Errorf("更新没落库: %+v", got)
	}
}

// TestSaveSettingSeparateReadFailureFromNotFound 接线护栏：SaveSetting 必须自己判
// ErrRecordNotFound，而不是靠「row.ID==0」顺带区分——那是本条洞的根源。
func TestSaveSettingSeparateReadFailureFromNotFound(t *testing.T) {
	src, err := readKeepseedSettingSource()
	if err != nil {
		t.Fatalf("读不到 keepseed_setting.go: %v", err)
	}
	code := stripKeepseedSettingComments(src)
	// 按语义判，不钉写法：First(&row) 的返回值必须被接住并判 ErrRecordNotFound。
	// （第一版钉的是 `First(&row)` 这行裸调的死形状，结果自己按 findErr := ….Error()
	//  写完就被误判成「还在裸调」——判据比修法窄会逼后来人写更别扭的代码。）
	// 逐行扫 SaveSetting 的函数体：凡出现 First(&row) 的行，该行必须**接住返回值**
	// （形如 `xxx := ....First(&row).Error`）。行尾就是 First(&row) ⇒ 错误被丢弃。
	body := keepseedFuncBody(code, "func (c *KeepSeedChain) SaveSetting(")
	found := false
	for _, l := range strings.Split(body, "\n") {
		if !strings.Contains(l, "First(&row)") {
			continue
		}
		found = true
		if !strings.Contains(l, ".Error") {
			t.Errorf("SaveSetting 仍在裸调 First(&row)（该行没有接住错误）：DB 读故障会被当成「没有这条设置」而插入重复行 —— %q", strings.TrimSpace(l))
		}
	}
	if !found {
		t.Error("判据没有落点：SaveSetting 里找不到 First(&row)")
	}
	if !strings.Contains(body, "ErrRecordNotFound") {
		t.Error("SaveSetting 必须显式判 ErrRecordNotFound（与同文件的 Setting 同一口径）")
	}
	if !strings.Contains(body, "return ") || !strings.Contains(body, "findErr") && !strings.Contains(body, "readErr") {
		t.Error("查不动时必须把错误返回出去，不许顺着 row.ID==0 走进插入分支")
	}
}

// readKeepseedSettingSource 读被测源文件（接线判据用）。
func readKeepseedSettingSource() (string, error) {
	b, err := os.ReadFile("keepseed_setting.go")
	return string(b), err
}

// stripKeepseedSettingComments 去掉整行注释，免得判据匹配到自己写的旧形状说明。
func stripKeepseedSettingComments(s string) string {
	out := make([]string, 0, 128)
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

// keepseedFuncBody 按大括号配平取函数体（不靠行号/缩进猜，函数改行数不失配）。
func keepseedFuncBody(src, marker string) string {
	at := strings.Index(src, marker)
	if at < 0 {
		return ""
	}
	start := strings.Index(src[at:], "{")
	if start < 0 {
		return ""
	}
	start += at
	depth := 0
	for i := start; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[start : i+1]
			}
		}
	}
	return src[start:]
}
