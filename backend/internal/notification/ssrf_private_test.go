package notification

import (
	"context"
	"strings"
	"testing"
)

// TestNotifyURLPrivateLANGated 内网/容器网段的通知服务：默认拒，开了产品开关才放行。
// 缺陷本体：站点抓取认 ALLOW_PRIVATE_SITES（app.go 建 client 时传 AllowPrivate），
// 而通知链抄的第二份判定不看这个开关 ⇒ 同机 NAS 上的 ntfy、compose 里的兄弟服务
// 被永久拦死，用户按提示设了开关也救不回来。
func TestNotifyURLPrivateLANGated(t *testing.T) {
	t.Cleanup(func() { SetAllowPrivateTargets(false) })
	ctx := context.Background()

	if err := assertNotifyURL(ctx, "ntfy", "http://192.168.1.20:8000/topic"); err == nil {
		t.Fatalf("默认应当拦内网地址（SSRF 守卫的本意）")
	}
	SetAllowPrivateTargets(true)
	if err := assertNotifyURL(ctx, "ntfy", "http://192.168.1.20:8000/topic"); err != nil {
		t.Errorf("ALLOW_PRIVATE_SITES=true 时内网 ntfy 必须放行，got %v", err)
	}
	if err := assertNotifyURL(ctx, "ntfy", "http://172.17.0.3:8000/topic"); err != nil {
		t.Errorf("compose 容器网段同样要放行，got %v", err)
	}
	// 开关不放链路本地/云元数据：那与「内网跑 ntfy」无关，是凭据外泄的口。
	if err := assertNotifyURL(ctx, "ntfy", "http://169.254.169.254/latest/meta-data"); err == nil {
		t.Errorf("云元数据地址即使在开关打开后也必须恒拒")
	}
}

// TestNotifyURLErrorsAreActionableChinese 拦下时的句子要能指路（#109/#126 口径）：
// 从前是 `blocked non-public/non-loopback address`，面板原样显示，用户既不知道
// 是自己填的 URL 有问题，也不知道有 ALLOW_PRIVATE_SITES 这个开关。
func TestNotifyURLErrorsAreActionableChinese(t *testing.T) {
	t.Cleanup(func() { SetAllowPrivateTargets(false) })
	ctx := context.Background()

	err := assertNotifyURL(ctx, "gotify", "http://10.0.0.5:8080/message")
	if err == nil {
		t.Fatal("内网地址应被拦")
	}
	msg := err.Error()
	for _, want := range []string{"通知地址", "SSRF", "ALLOW_PRIVATE_SITES", "gotify"} {
		if !strings.Contains(msg, want) {
			t.Errorf("拦下时要点名渠道并给出下一步，缺 %q，got %q", want, msg)
		}
	}
	if strings.Contains(msg, "blocked non-public") {
		t.Errorf("英文原文不该再出现在用户面上，got %q", msg)
	}

	if err := assertNotifyURL(ctx, "bark", "ftp://bark.example.com/key"); err == nil || !strings.Contains(err.Error(), "协议只支持 http/https") {
		t.Errorf("scheme 错误要给中文可操作提示，got %v", err)
	}
}

// TestNotifyURLLoopbackStillAllowedWithoutSwitch 反向对照：127.0.0.1 是本机通知容器的常规填法，
// 关着开关也不能拦（防我把「放行内网」写成「一律放行」或把 loopback 一起挡掉）。
func TestNotifyURLLoopbackStillAllowedWithoutSwitch(t *testing.T) {
	t.Cleanup(func() { SetAllowPrivateTargets(false) })
	SetAllowPrivateTargets(false)
	if err := assertNotifyURL(context.Background(), "apprise", "http://127.0.0.1:8000/notify/x"); err != nil {
		t.Errorf("loopback 必须始终放行，got %v", err)
	}
}
