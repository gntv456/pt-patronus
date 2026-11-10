// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package filemanager

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// argsContain 判断 rclone 参数切片（含 --config 前缀）中是否出现某个子命令/路径串。
func argsContain(args []string, s string) bool {
	for _, a := range args {
		if a == s {
			return true
		}
	}
	return false
}

// newTestRclone 构造一个不触发 LookPath（binary 留空→"rclone"）的 Rclone，仅用于单元测试。
func newTestRclone(t *testing.T, rootDir string) *Rclone {
	t.Helper()
	r, err := NewRclone("", "myremote", "", rootDir)
	if err != nil {
		t.Fatalf("NewRclone: %v", err)
	}
	return r
}

func TestRclone_NewValidates(t *testing.T) {
	// 空 remote 应报错
	if _, err := NewRclone("", "  ", "", "/Media"); err == nil {
		t.Fatal("空 remote 应报错")
	}
	// remote 带非法字符应报错
	for _, bad := range []string{"my:remote", "my/remote", `my\remote`} {
		if _, err := NewRclone("", bad, "", ""); err == nil {
			t.Fatalf("remote %q 应报错", bad)
		}
	}
	// 尾随冒号应被裁掉
	r, err := NewRclone("", "myremote:", "", "")
	if err != nil {
		t.Fatalf("尾随冒号应容忍: %v", err)
	}
	if r.remote != "myremote" {
		t.Errorf("remote = %q, want myremote", r.remote)
	}
	// binary 为空时默认 rclone 且不 LookPath（无需真实二进制）
	if r.binary != "rclone" {
		t.Errorf("binary = %q, want rclone", r.binary)
	}
}

func TestRclone_Resolve(t *testing.T) {
	r := newTestRclone(t, "/Media")
	cases := []struct{ in, want string }{
		{"/", "myremote:/Media"},
		{"/Movies", "myremote:/Media/Movies"},
		{"/Movies/2024/a.mkv", "myremote:/Media/Movies/2024/a.mkv"},
		{"../../../etc/passwd", "myremote:/Media/etc/passwd"}, // ../ 被钳制回 root
		{`\..\..\etc`, "myremote:/Media/etc"},                 // 反斜杠变体同样钳制
		{"/a/../../etc", "myremote:/Media/etc"},               // 多级回溯被钳
		{"", "myremote:/Media"},                               // 空路径落到根
	}
	for _, c := range cases {
		if got := r.resolve(c.in); got != c.want {
			t.Errorf("resolve(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// rootDir 为空：以 / 为边界，整 remote 可浏览
	r2 := newTestRclone(t, "")
	if got := r2.resolve("/foo/bar"); got != "myremote:/foo/bar" {
		t.Errorf("empty root resolve = %q, want myremote:/foo/bar", got)
	}
	if got := r2.resolve("../../../etc"); got != "myremote:/etc" {
		t.Errorf("empty root escape clamp = %q, want myremote:/etc", got)
	}
}

func TestRclone_List(t *testing.T) {
	r := newTestRclone(t, "/Media")
	r.execFunc = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if !argsContain(args, "lsjson") {
			t.Errorf("List 应调用 lsjson, args=%v", args)
		}
		// 校验远端目标路径
		if !argsContain(args, "myremote:/Media") {
			t.Errorf("List 目标应为 myremote:/Media, args=%v", args)
		}
		return []byte(`[
		  {"Name":"movie.mkv","IsDir":false,"Size":1024,"ModTime":"2023-01-02T15:04:05Z"},
		  {"Name":"sub","IsDir":true,"Size":0,"ModTime":"2023-01-02T15:04:06.123456789Z"}
		]`), nil
	}
	entries, err := r.List(context.Background(), "/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries len = %d, want 2", len(entries))
	}
	// 第一项文件
	if entries[0].Name != "movie.mkv" || entries[0].IsDir || entries[0].Size != 1024 {
		t.Errorf("entry0 = %+v", entries[0])
	}
	if entries[0].Path != "/Media/movie.mkv" {
		t.Errorf("entry0.Path = %q, want /Media/movie.mkv", entries[0].Path)
	}
	if entries[0].ModTime.Year() != 2023 {
		t.Errorf("entry0.ModTime = %v", entries[0].ModTime)
	}
	// 第二项目录 + 小数秒 ModTime 解析
	if !entries[1].IsDir || entries[1].Name != "sub" {
		t.Errorf("entry1 = %+v", entries[1])
	}
	if entries[1].Path != "/Media/sub" {
		t.Errorf("entry1.Path = %q, want /Media/sub", entries[1].Path)
	}
	if entries[1].ModTime.IsZero() {
		t.Errorf("entry1.ModTime 小数秒解析失败")
	}
}

func TestRclone_Mkdir(t *testing.T) {
	r := newTestRclone(t, "/Media")
	var captured []string
	r.execFunc = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		captured = args
		return nil, nil
	}
	if err := r.Mkdir(context.Background(), "/new/dir"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if !argsContain(captured, "mkdir") {
		t.Errorf("Mkdir 应调用 mkdir, args=%v", captured)
	}
	if !argsContain(captured, "myremote:/Media/new/dir") {
		t.Errorf("Mkdir 目标应为 myremote:/Media/new/dir, args=%v", captured)
	}
	// --config 注入
	if !argsContain(captured, "--config") {
		t.Errorf("Mkdir 应注入 --config, args=%v", captured)
	}
}

func TestRclone_Upload(t *testing.T) {
	r := newTestRclone(t, "/Media")
	var captured []string
	r.execFunc = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		captured = args
		return nil, nil
	}
	if err := r.Upload(context.Background(), "/up/a.txt", strings.NewReader("hello")); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if !argsContain(captured, "copyto") {
		t.Errorf("Upload 应调用 copyto, args=%v", captured)
	}
	if !argsContain(captured, "myremote:/Media/up/a.txt") {
		t.Errorf("Upload 远端目标应为 myremote:/Media/up/a.txt, args=%v", captured)
	}
	// 本地源应是真实存在的临时文件（run 之前 io.Copy 已写入）
	// copyto 的参数序：--config conf copyto <localSrc> <remoteDst>
	localSrc := ""
	for i, a := range captured {
		if a == "copyto" && i+1 < len(captured) {
			localSrc = captured[i+1]
			break
		}
	}
	if localSrc == "" {
		t.Fatalf("未找到 copyto 本地源, args=%v", captured)
	}
	if _, err := os.Stat(localSrc); !os.IsNotExist(err) {
		t.Errorf("Upload 后本地临时文件应已删除: %s (stat err=%v)", localSrc, err)
	}
}

func TestRclone_Download(t *testing.T) {
	r := newTestRclone(t, "/Media")
	r.execFunc = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if !argsContain(args, "copyto") {
			t.Errorf("Download 应调用 copyto, args=%v", args)
		}
		if !argsContain(args, "myremote:/Media/dl.txt") {
			t.Errorf("Download 远端源应为 myremote:/Media/dl.txt, args=%v", args)
		}
		// copyto <remote> <localTmp>：最后一个参数是本地目标，桩写入内容供后续 os.Open 读取
		localDst := args[len(args)-1]
		if err := os.WriteFile(localDst, []byte("downloaded-bytes"), 0o644); err != nil {
			t.Fatalf("stub write local dst: %v", err)
		}
		return nil, nil
	}
	rc, err := r.Download(context.Background(), "/dl.txt")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	// 记录临时文件路径，Close 后应被删除
	tmpPath := rc.(*tempFileReadCloser).path
	buf := make([]byte, 32)
	n, _ := rc.Read(buf)
	if string(buf[:n]) != "downloaded-bytes" {
		t.Errorf("download content = %q", string(buf[:n]))
	}
	if err := rc.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("Download Close 后临时文件应已删除: %s (stat err=%v)", tmpPath, err)
	}
}

// TestRclone_Delete 验证：目录→purge，文件→deletefile，不存在→幂等 nil。
func TestRclone_Delete(t *testing.T) {
	ctx := context.Background()

	// 目录分支
	t.Run("dir_uses_purge", func(t *testing.T) {
		r := newTestRclone(t, "/Media")
		var ops []string
		r.execFunc = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			if argsContain(args, "lsjson") {
				// 父目录探测返回子项，命中 mydir 且 IsDir=true
				return []byte(`[{"Name":"mydir","IsDir":true},{"Name":"other","IsDir":false}]`), nil
			}
			ops = append(ops, args...)
			return nil, nil
		}
		if err := r.Delete(ctx, "/mydir"); err != nil {
			t.Fatalf("Delete dir: %v", err)
		}
		if !argsContain(ops, "purge") {
			t.Errorf("目录应调用 purge, ops=%v", ops)
		}
		if argsContain(ops, "deletefile") {
			t.Errorf("目录不应调用 deletefile, ops=%v", ops)
		}
		if !argsContain(ops, "myremote:/Media/mydir") {
			t.Errorf("purge 目标应为 myremote:/Media/mydir, ops=%v", ops)
		}
	})

	// 文件分支
	t.Run("file_uses_deletefile", func(t *testing.T) {
		r := newTestRclone(t, "/Media")
		var ops []string
		r.execFunc = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			if argsContain(args, "lsjson") {
				return []byte(`[{"Name":"myfile","IsDir":false}]`), nil
			}
			ops = append(ops, args...)
			return nil, nil
		}
		if err := r.Delete(ctx, "/myfile"); err != nil {
			t.Fatalf("Delete file: %v", err)
		}
		if !argsContain(ops, "deletefile") {
			t.Errorf("文件应调用 deletefile, ops=%v", ops)
		}
		if argsContain(ops, "purge") {
			t.Errorf("文件不应调用 purge, ops=%v", ops)
		}
	})

	// 不存在 → 报 fs.ErrNotExist（旧行为返回 nil，界面把没删成的行报成「已删除」）
	t.Run("missing_is_reported", func(t *testing.T) {
		r := newTestRclone(t, "/Media")
		var ops []string
		r.execFunc = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			if argsContain(args, "lsjson") {
				// 父目录列表不含目标
				return []byte(`[{"Name":"sibling","IsDir":false}]`), nil
			}
			ops = append(ops, args...)
			return nil, nil
		}
		err := r.Delete(ctx, "/ghost")
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("Delete missing err=%v，应判为 fs.ErrNotExist（handler 靠它归类成 404 中文提示）", err)
		}
		if len(ops) != 0 {
			t.Errorf("不存在的路径不应触发删除操作, ops=%v", ops)
		}
	})
}

// TestRclone_RunTempConf 验证 run 写出的临时 conf 文件内容即 confContent，并在调用后删除。
func TestRclone_RunTempConf(t *testing.T) {
	r, err := NewRclone("", "myremote", "[myremote]\ntype = drive\n", "/Media")
	if err != nil {
		t.Fatalf("NewRclone: %v", err)
	}
	var seenConfPath string
	var seenContent string
	r.execFunc = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		// args = --config <path> <subcmd...>
		for i, a := range args {
			if a == "--config" && i+1 < len(args) {
				seenConfPath = args[i+1]
				break
			}
		}
		if seenConfPath != "" {
			b, _ := os.ReadFile(seenConfPath)
			seenContent = string(b)
		}
		return []byte("ok"), nil
	}
	out, err := r.run(context.Background(), "lsjson", "myremote:/Media")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if string(out) != "ok" {
		t.Errorf("run stdout = %q", string(out))
	}
	if seenContent != "[myremote]\ntype = drive\n" {
		t.Errorf("临时 conf 内容 = %q", seenContent)
	}
	// run 返回后临时文件应已删除
	if _, err := os.Stat(seenConfPath); !os.IsNotExist(err) {
		t.Errorf("临时 conf 应已删除: %s (stat err=%v)", seenConfPath, err)
	}
}

// TestRclone_DefaultExecMissingBinary 验证默认 execFunc 在二进制缺失时返回含 stderr 的错误，
// 而非 panic（确保未覆盖 execFunc 时的错误路径可读）。
func TestRclone_DefaultExecMissingBinary(t *testing.T) {
	dir := t.TempDir()
	fakeBin := filepath.Join(dir, "definitely-not-a-real-rclone-binary")
	// defaultRcloneExec 直接走 exec，二进制不存在 → 错误
	_, err := defaultRcloneExec(context.Background(), fakeBin, "--version")
	if err == nil {
		t.Fatal("缺失二进制应返回错误")
	}
	if !strings.Contains(err.Error(), "definitely-not-a-real-rclone-binary") && !strings.Contains(err.Error(), "executable file not found") {
		// exec 的错误信息在不同平台措辞不同，只要非空即可
		t.Logf("err = %v", err)
	}
}
