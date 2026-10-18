package errtext

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// TestDownloaderOfflineBecomesActionable 下载器连不上在各面（测试连接、辅种、查漏）都以
// 「cannot connect to downloader: Post "http://…" dial tcp … refused」的形式回来。
// 从前这段英文原文要么被翻成「请求参数有误」（指错方向），要么原样弹给用户；
// 现在统一落到能照着做的中文提示，同时不泄露 Go 的拨号细节。
func TestDownloaderOfflineBecomesActionable(t *testing.T) {
	err := fmt.Errorf(
		"%w: Post %q: dial tcp 127.0.0.1:9: connectex: No connection could be made because the target machine actively refused it.",
		downloader.ErrCannotConnect, "http://127.0.0.1:9/api/v2/auth/login")
	got := Text(err)
	if got != DownloaderOffline {
		t.Fatalf("Text = %q, want 映射到 DownloaderOffline 提示", got)
	}
	for _, leak := range []string{"dial tcp", "connectex", "cannot connect", "/api/v2/auth/login"} {
		if strings.Contains(got, leak) {
			t.Errorf("提示不得带出 Go 拨号原文（%q）：%q", leak, got)
		}
	}
	// 提示必须给出下一步：去哪儿检查。
	if !strings.Contains(got, "下载器") {
		t.Errorf("Text = %q, want 指明去「下载器」页处理", got)
	}
}

// TestOtherDownloaderTextStillPassesThrough 划界：只有本仓库自己声明的哨兵才翻译，
// 客户端自己回的错误页/业务消息是用户唯一的线索，不能加工。
func TestOtherDownloaderTextStillPassesThrough(t *testing.T) {
	err := errors.New(`qbittorrent: "Forbidden."`)
	if got := Text(err); got != err.Error() {
		t.Errorf("Text = %q, want 原样透传 %q", got, err.Error())
	}
}
