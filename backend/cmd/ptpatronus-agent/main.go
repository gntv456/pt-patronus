// Command ptpatronus-agent 是 PTPatronus 监控 agent（独立轻量二进制，跑在被监控机）。
// 通过 WS 长连接主动上报本机指标到 dashboard，替代 SSH 拉取。
//
// 用法：
//
//	DASHBOARD_URL=ws://dashboard:8088 AGENT_ID=1 AGENT_SECRET=xxx ./ptpatronus-agent
//	REPORT_INTERVAL=5  # 可选，上报间隔秒（默认 5，最小 3）
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/hxpt/ptpatronus/internal/agent"
)

func main() {
	cfg, ok := agent.LoadConfig()
	if !ok {
		slog.Error("missing required env: DASHBOARD_URL / AGENT_SECRET / AGENT_ID")
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	slog.Info("ptpatronus agent starting", "server", cfg.ServerURL, "agent_id", cfg.AgentID, "interval", cfg.ReportInterval)
	agent.Run(ctx, cfg)
}
