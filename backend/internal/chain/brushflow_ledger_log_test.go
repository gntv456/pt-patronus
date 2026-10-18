package chain

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestNoteTaskWrite 台账字段写失败的留痕本身要能点名「哪个任务、哪个字段、为什么」。
// 这些字段都有界面读者（BrushFlow.vue:308 的 ⚠ last_error、Web/移动的上次执行时间与累计数），
// 从前八个写点全是 `_ = c.op.…`：写不进去界面就停在上一次的值上，且日志里查不到原因。
func TestNoteTaskWrite(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	c := &BrushFlowChain{}
	c.noteTaskWrite(7, "last_error(select)", errors.New("database is locked"))
	logged := buf.String()
	for _, want := range []string{"刷流任务台账写入失败", "last_error(select)", "database is locked", "task=7"} {
		if !strings.Contains(logged, want) {
			t.Errorf("日志里要点名 %q, got %q", want, logged)
		}
	}

	// 正控：写成功（err==nil）不该产生任何噪音。
	buf.Reset()
	c.noteTaskWrite(7, "last_select_at", nil)
	if strings.TrimSpace(buf.String()) != "" {
		t.Errorf("成功路径不该打日志, got %q", buf.String())
	}
}

// TestNoDiscardedTaskLedgerWrites 结构护栏（诚实标注：这是源码层守卫，不证明运行时日志出现，
// 只保证没有人再把这几个台账写退回 `_ =` 丢弃）。
func TestNoDiscardedTaskLedgerWrites(t *testing.T) {
	pat := regexp.MustCompile(`(?m)^\s*_ = c\.(op|itemOp)\.[A-Za-z]+\(`)
	for _, file := range []string{"brushflow.go", "rss.go"} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("读 %s: %v", file, err)
		}
		if hits := pat.FindAllString(string(raw), -1); len(hits) > 0 {
			t.Errorf("%s 又出现了丢弃台账写入的用例：%v", file, hits)
		}
	}
}
