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
)

func main() {
	// 离线应急子命令：reset-admin（好学登录链路坏掉时恢复管理员）。
	if len(os.Args) > 1 && os.Args[1] == "reset-admin" {
		if err := runResetAdmin(); err != nil {
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
	// debug 环境开 debug 级别日志（便于排查站点契约/游戏响应）；release 用 info。
	logLevel := "info"
	if cfg.Env == "debug" {
		logLevel = "debug"
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

	go func() {
		if err := application.Run(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()
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
