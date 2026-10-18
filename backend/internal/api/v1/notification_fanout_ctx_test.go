package v1

import (
	"context"
	"testing"
	"time"
)

// TestNotifyFanoutContextSurvivesClientAbort 浏览器超时断开时，正在发送的渠道不能被取消。
// 缺陷本体：从前直接把 c.Request.Context() 交给 Notify 扇出，而单渠道最坏是
// 「有界 DNS 8s（notification.notifyResolveTimeout）+ HTTP 15s（各渠道 http.Client{Timeout:15s}）」，
// 前端全局 axios 只有 15s ⇒ 客户端先断 ⇒  ctx canceled ⇒
// 消息其实已发出/仍在发，账面却记成失败，用户点重试就是重复推送（同 #167/#212 那条轴）。
func TestNotifyFanoutContextSurvivesClientAbort(t *testing.T) {
	req, cancel := context.WithCancel(context.Background())
	cancel() // 模拟客户端已超时断开

	ctx, fc := notifyFanoutContext(req)
	defer fc()

	if ctx.Err() != nil {
		t.Fatalf("扇出上下文不得继承请求的取消（否则在发的渠道被掐成失败）：%v", ctx.Err())
	}
	dl, ok := ctx.Deadline()
	if !ok {
		t.Fatalf("必须有界：渠道挂住时不能无限等平台（脱离取消≠脱离超时）")
	}
	if left := time.Until(dl); left < notifyFanoutBudget-time.Second {
		t.Errorf("deadline 要给满预算，剩余 %v < %v-1s", left, notifyFanoutBudget)
	}
}

// TestNotifyFanoutBudgetCoversChannelCeilings 预算必须盖得住单渠道的两个天花板之和（8s DNS + 15s HTTP）。
// 前端 notificationApi.test 的 timeout 由 vitest 钉住 ≥30000ms（src/api/notification.test.ts），
// 两边一起才成立：服务端预算被压低而前端不动 = 继续假超时，前端被压低而服务端不动 = 界面读不到回执。
func TestNotifyFanoutBudgetCoversChannelCeilings(t *testing.T) {
	const worstChannel = 8*time.Second + 15*time.Second // 见上方注释的两处常量出处
	if notifyFanoutBudget < worstChannel {
		t.Errorf("notifyFanoutBudget=%v 小于单渠道最坏耗时 %v ⇒ 客户端会先看到超时而不是真实回执",
			notifyFanoutBudget, worstChannel)
	}
}
