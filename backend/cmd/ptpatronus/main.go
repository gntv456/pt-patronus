// Command ptpatronus is the PT management backend entry point.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hxpt/ptpatronus/internal/app"
	"github.com/hxpt/ptpatronus/internal/core/config"
	applog "github.com/hxpt/ptpatronus/internal/core/log"
	"github.com/hxpt/ptpatronus/internal/version"
	"github.com/hxpt/ptpatronus/internal/core/safego"
)

func main() {
	// 离线应急子命令：reset-admin（好学登录链路坏掉时恢复管理员）。
	// --password 变体：给实例主人设本地密码（好学站点关闭后的主登录入口，#<issue>）。
	if len(os.Args) > 1 && os.Args[1] == "reset-admin" {
		if err := runResetAdmin(os.Args[2:]); err != nil {
			slog.Error("reset-admin failed", "err", err)
			os.Exit(1)
		}
		return
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config invalid, refusing to start", "err", err)
		os.Exit(1)
	}
	// 日志级别：LOG_LEVEL env 优先（运维排障可临时调 debug，无需改代码重启镜像构建）；
	// 未设置时 debug 环境开 debug 级别（便于排查站点契约/游戏响应），release 用 info。
	logLevel := "info"
	if cfg.Env == "debug" {
		logLevel = "debug"
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		switch v {
		case "debug", "info", "warn", "error":
			logLevel = v
		default:
			slog.Warn("invalid LOG_LEVEL, fallback to default", "got", v, "allowed", "debug|info|warn|error")
		}
	}
	applog.Init(cfg.Env, logLevel)
	slog.Info("starting", "app", cfg.AppName, "version", version.Version, "env", cfg.Env, "addr", cfg.HTTPAddr)

	// Ensure the data directory exists before opening SQLite.
	if cfg.DataDir != "" {
		if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
			slog.Error("create data dir failed", "err", err)
			os.Exit(1)
		}
	}

	application, err := app.New(cfg)
	if err != nil {
		slog.Error("init failed", "err", err)
		os.Exit(1)
	}

	safego.Go("main:57", func() {
		if err := application.Run(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	})
	slog.Info("server listening", "addr", cfg.HTTPAddr)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := application.Shutdown(ctx); err != nil {
		slog.Error("forced shutdown", "err", err)
	}
	slog.Info("bye")
}
