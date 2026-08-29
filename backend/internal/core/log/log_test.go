package log

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

// TestRingBufferClear 验证清空：Recent 归零，且清空后 Push 仍正常（不越界、计数正确）。
func TestRingBufferClear(t *testing.T) {
	r := &RingBuffer{cap: 4, buf: make([]LogEntry, 0, 4)}
	r.Push(LogEntry{Level: "INFO", Message: "a"})
	r.Push(LogEntry{Level: "INFO", Message: "b"})
	if got := r.Recent(0, slog.LevelInfo); len(got) != 2 {
		t.Fatalf("before clear: want 2, got %d", len(got))
	}

	r.Clear()
	if got := r.Recent(0, slog.LevelInfo); len(got) != 0 {
		t.Fatalf("after clear: want 0, got %d", len(got))
	}

	// 清空后仍可正常 Push
	r.Push(LogEntry{Level: "INFO", Message: "c"})
	got := r.Recent(0, slog.LevelInfo)
	if len(got) != 1 || got[0].Message != "c" {
		t.Fatalf("after clear+push: want [c], got %+v", got)
	}
}

// TestInitReleaseJSON 验证 release 环境走 JSONHandler（stdout 机器可读）：
// 输出能被 json.Unmarshal 解析且含 level/msg 字段；debug 环境仍为 TextHandler
// （非 JSON 形态）。Init 会替换全局默认 logger，测试后无法还原——本包内
// 后续用例不依赖默认 handler，可接受。
func TestInitReleaseJSON(t *testing.T) {
	restore := captureStdout(t)

	Init("release", "info")
	slog.Info("hello_json", "k", "v")
	out := restore()

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("release output should be JSON, got %q: %v", out, err)
	}
	if parsed["msg"] != "hello_json" {
		t.Fatalf("want msg=hello_json, got %v", parsed["msg"])
	}
	if parsed["level"] != "INFO" {
		t.Fatalf("want level=INFO, got %v", parsed["level"])
	}
}

func TestInitDebugText(t *testing.T) {
	restore := captureStdout(t)

	Init("debug", "debug")
	slog.Debug("hello_text", "k", "v")
	out := restore()

	if !strings.Contains(out, "hello_text") {
		t.Fatalf("debug output should contain message, got %q", out)
	}
	// TextHandler 形态（time=... level=...）；不应是纯 JSON 一行对象
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Fatalf("debug env should use TextHandler, got JSON: %q", out)
	}
}

// captureStdout 重定向进程 stdout，返回恢复函数；恢复时取回累积输出。
func captureStdout(t *testing.T) func() string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	ch := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		ch <- string(b)
	}()
	return func() string {
		w.Close()
		os.Stdout = old
		return <-ch
	}
}

// ── 日志轮转（LOG_FILE + lumberjack） ────────────────────────────

// TestChooseOutputStdoutByDefault LOG_FILE 空 → stdout（默认容器友好）。
func TestChooseOutputStdoutByDefault(t *testing.T) {
	t.Setenv("LOG_FILE", "")
	if w := chooseOutput("release"); w != io.Writer(os.Stdout) {
		t.Fatalf("默认应 stdout，got %T", w)
	}
}

// TestInitWithLogFileWritesFile Init 级端到端：LOG_FILE 指定后日志落文件（JSON/文本皆直写）。
// 注：Init 替换全局默认 logger 且持有 lumberjack 句柄（进程生命周期语义，测试无法 Close），
// 因此用独立临时文件且不依赖 TempDir 自动清理（句柄占用会令 RemoveAll 失败）。
func TestInitWithLogFileWritesFile(t *testing.T) {
	// 不用 t.TempDir：Init 持有 lumberjack 句柄至进程结束，Windows 下 RemoveAll 会失败。
	// 用固定临时路径，残留一个探针文件属可接受噪音（CI/Linux 无此问题）。
	dir := filepath.Join(os.TempDir(), "ptp-logtest-init")
	_ = os.MkdirAll(dir, 0o755)
	logFile := filepath.Join(dir, "init-probe.log")
	_ = os.Remove(logFile)
	t.Setenv("LOG_FILE", logFile)

	logger := Init("debug", "info")
	logger.Info("rotation-probe-line")
	if _, err := os.Stat(logFile); err != nil {
		t.Fatalf("日志文件应被创建: %v", err)
	}
	// 写入内容包含探针串（TextHandler 直写即落盘）
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "rotation-probe-line") {
		t.Fatalf("日志应落盘并含探针: %q", string(data))
	}
}

// TestLumberjackRotateBehavior 直接验证轮转参数组装：小文件阈值下写超限产生备份。
func TestLumberjackRotateBehavior(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "rotate.log")
	t.Setenv("LOG_FILE", logFile)
	t.Setenv("LOG_MAX_SIZE_MB", "1")
	t.Setenv("LOG_MAX_BACKUPS", "2")

	// chooseOutput 产出的 writer 即 lumberjack 实例（配置正确性 + 轮转行为二合一）
	w := chooseOutput("release")
	lj, ok := w.(*lumberjack.Logger)
	if !ok {
		t.Fatalf("应返回 lumberjack writer，got %T", w)
	}
	if lj.Filename != logFile || lj.MaxSize != 1 || lj.MaxBackups != 2 {
		t.Fatalf("轮转参数组装错误: %+v", lj)
	}

	// 写 >1MB 触发一次轮转（JSON 行每条 ~100B，写 12000 条）
	line := strings.Repeat("x", 100)
	for i := 0; i < 12000; i++ {
		if _, err := w.Write([]byte(line + "\n")); err != nil {
			t.Fatalf("写入失败: %v", err)
		}
	}
	_ = lj.Close()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	// lumberjack 默认备份命名 rotate-<UTC时间戳>.log（非 .1 序号形态）
	hasBackup := false
	for _, e := range entries {
		n := e.Name()
		if n != "rotate.log" && strings.HasPrefix(n, "rotate-") && strings.HasSuffix(n, ".log") {
			hasBackup = true
		}
	}
	if !hasBackup {
		t.Fatalf("超过 MaxSize 应产生轮转备份，目录内容: %v", entries)
	}
	// 轮转后主日志文件仍存在且被重新创建
	if _, err := os.Stat(logFile); err != nil {
		t.Fatalf("轮转后主文件应重建: %v", err)
	}
}

// TestGetenvIntDefault 环境变量整数解析（非法回默认、零值透传）。
func TestGetenvIntDefault(t *testing.T) {
	t.Setenv("LOG_TEST_NUM", "42")
	if got := getenvIntDefault("LOG_TEST_NUM", 7); got != 42 {
		t.Fatalf("合法值应 42，got %d", got)
	}
	t.Setenv("LOG_TEST_NUM", "abc")
	if got := getenvIntDefault("LOG_TEST_NUM", 7); got != 7 {
		t.Fatalf("非法值应回默认 7，got %d", got)
	}
	if got := getenvIntDefault("LOG_TEST_MISSING", 7); got != 7 {
		t.Fatalf("缺失应回默认 7，got %d", got)
	}
	t.Setenv("LOG_TEST_NUM", "-3")
	if got := getenvIntDefault("LOG_TEST_NUM", 7); got != 7 {
		t.Fatalf("负值应回默认 7（防御），got %d", got)
	}
}
