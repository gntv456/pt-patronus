package plugins

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestFFprobeErrorReasons(t *testing.T) {
	// 1) 宿主机没装 ffmpeg：给可操作提示，而不是英文 os 错
	notFound := &exec.Error{Name: "ffprobe", Err: exec.ErrNotFound}
	msg := ffprobeError(notFound, "").Error()
	if !strings.Contains(msg, "未找到 ffprobe") || !strings.Contains(msg, "ffmpeg") {
		t.Errorf("要点名缺 ffmpeg 并给出可操作动作, got %q", msg)
	}
	if strings.Contains(msg, "executable file not found") {
		t.Errorf("英文原文不该进界面, got %q", msg)
	}

	// 2) 这次探测失败：`ffprobe -v error` 的诊断走 stderr，必须带回来
	if m := ffprobeError(errors.New("exit status 1"), "No such file or directory").Error(); !strings.Contains(m, "No such file") {
		t.Errorf("stderr 原因要透出, got %q", m)
	}
	// 3) 没有 stderr 时也不能留「ffprobe: xxx: 」这种空原因尾巴
	if m := ffprobeError(errors.New("exit status 1"), "   ").Error(); strings.HasSuffix(m, ": ") {
		t.Errorf("空原因文案复现, got %q", m)
	}
}

// TestProbeDurationWithoutFFprobe 走真出口：把 PATH 清空后 probeDuration
// 必须回中文「未找到 ffprobe」，而不是旧代码那句英文 exit/lookup 原文。
func TestProbeDurationWithoutFFprobe(t *testing.T) {
	t.Setenv("PATH", "")
	_, err := probeDuration(context.Background(), "anything.mp4")
	if err == nil {
		t.Skip("本机仍能启动 ffprobe（PATH 清理未生效），跳过")
	}
	if strings.Contains(err.Error(), "executable file not found") {
		t.Errorf("英文原文泄漏给用户, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "ffprobe") {
		t.Errorf("错误里要点名 ffprobe, got %q", err.Error())
	}
}
