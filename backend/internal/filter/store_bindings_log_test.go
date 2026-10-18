package filter

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

func newBindingStore(t *testing.T) (*Store, *gorm.DB) {
	t.Helper()
	// 必须带 `file:` 前缀：`xxx?mode=memory&cache=shared` 少了 scheme 时，驱动把它当**磁盘路径**，
	// 于是「命名内存库」变成在源码目录里落一个 SQLite 文件——第二次跑会读到上一轮的残留行，
	// 用例的隔离前提就没了（同仓正确写法见 oper/testdb_test.go 与 automation/mail_seen_log_test.go）。
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+fmt.Sprintf("_%d", len(t.Name()))+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.SystemConfig{}); err != nil {
		t.Fatal(err)
	}
	return NewStore(oper.NewSystemConfigOper(db)), db
}

func blockConfigsUpdates(t *testing.T, db *gorm.DB, on bool) {
	t.Helper()
	table := db.NamingStrategy.TableName("SystemConfig")
	var err error
	if on {
		err = db.Exec(`CREATE TRIGGER block_cfg_update BEFORE UPDATE ON ` + table +
			` BEGIN SELECT RAISE(ABORT, '模拟绑定清理失败'); END`).Error
	} else {
		err = db.Exec(`DROP TRIGGER block_cfg_update`).Error
	}
	if err != nil {
		t.Fatalf("切换写阻断失败：%v", err)
	}
}

// TestCleanupGroupFromBindingsLogsFailure 删除规则组时，组表本身是检查过写结果的，
// 但从三个全局绑定里摘掉这个组名的那一步从前是 `_ = s.SetBindings(key, out)`：
// 写不进去就留下**悬空绑定**——那条搜索/订阅/洗版筛选再也找不到规则组，
// 表现为「筛选悄悄不生效」，而用户看到组已经不在列表里，什么都查不到头绪。
//
// 这里刻意不把失败上抛成「删除失败」：组已经删了，重试删除会先撞上「规则组不存在」，
// 悬空绑定反而永远清不掉。所以判据是「不改删除结果，但必须出声」。
func TestCleanupGroupFromBindingsLogsFailure(t *testing.T) {
	st, db := newBindingStore(t)
	if err := st.SetBindings(KeySearchFilterRuleGroups, []string{"待删组", "留着组"}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	blockConfigsUpdates(t, db, true)
	st.cleanupGroupFromBindings("待删组")
	logged := buf.String()
	for _, want := range []string{"规则组绑定清理失败", "静默不生效", "SearchFilterRuleGroups", "待删组", "模拟绑定清理失败"} {
		if !strings.Contains(logged, want) {
			t.Errorf("日志里要点名 %q, got %q", want, logged)
		}
	}
	// 悬空绑定确实还在（这就是必须留线索的那个状态；本轮不改删除结果）
	names, err := st.GetBindings(KeySearchFilterRuleGroups)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(names, ","), "待删组") {
		t.Logf("绑定仍含已删除的组名（悬空引用）：%v", names)
	}

	// 正控：写得进去时不该有失败日志，且组名真的被摘掉、别的组名保留。
	buf.Reset()
	blockConfigsUpdates(t, db, false)
	st.cleanupGroupFromBindings("待删组")
	if strings.Contains(buf.String(), "清理失败") {
		t.Errorf("正常清理不该报失败, got %q", buf.String())
	}
	after, err := st.GetBindings(KeySearchFilterRuleGroups)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || after[0] != "留着组" {
		t.Errorf("应只摘掉目标组名, got %v", after)
	}
}
