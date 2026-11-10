// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/config"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/doctor"
)

// lazyTargets 的目标列表读失败以前被吞成 0 目标 → 体检渲染成「未配置」(skipped/info)，
// 与「真的没配」同形；现在必须报 failed/error 并带错误详情。
func TestLazyTargetsListFailureFails(t *testing.T) {
	check := lazyTargets("business.sites", "站点连通", false, func() ([]doctor.Target, error) {
		return nil, errors.New("db gone")
	})
	f := check(context.Background())
	if f.Status != doctor.StatusFailed || f.Severity != doctor.SevError {
		t.Fatalf("读库失败要报 failed/error, got %+v", f)
	}
	if !strings.Contains(f.Detail, "db gone") {
		t.Errorf("Finding 要带错误详情, got %+v", f)
	}
}

// 体检的 TMDB 探活地址必须跟业务同一份运行时设置：env 默认 + SystemConfig 热更新。
func TestTMDBProbeURLFollowsRuntimeSettings(t *testing.T) {
	db := openBareDB(t)
	if err := db.AutoMigrate(&models.SystemConfig{}); err != nil {
		t.Fatal(err)
	}
	cfgOp := oper.NewSystemConfigOper(db)
	provider := chain.NewMediaSourceProvider(cfgOp, chain.MediaSettings{TMDBAPIDomain: "api.tmdb.org"})
	mediaCh := chain.NewMediaChain(chain.Base{}, provider, nil, nil)
	d := runtimeDeps{cfg: &config.Config{TMDBAPIDomain: "api.tmdb.org"}, mediaCh: mediaCh}

	if got := tmdbProbeURL(d)(); got != "https://api.tmdb.org/3/configuration" {
		t.Fatalf("应使用生效的 TMDB 域名, got %s", got)
	}
	// 设置页热更新后的生效值优先于 env 默认。
	if err := cfgOp.Set("media.tmdb_api_domain", "tmdb.example.com"); err != nil {
		t.Fatal(err)
	}
	if got := tmdbProbeURL(d)(); got != "https://tmdb.example.com/3/configuration" {
		t.Fatalf("SystemConfig 覆盖后应改用新域名, got %s", got)
	}
}

// 正控：目标解析成功时回落 CheckTargets 既有行为（空目标照旧 skipped/info）。
func TestLazyTargetsEmptyStillSkipped(t *testing.T) {
	check := lazyTargets("business.sites", "站点连通", false, func() ([]doctor.Target, error) {
		return []doctor.Target{}, nil
	})
	f := check(context.Background())
	if f.Status != doctor.StatusSkipped {
		t.Fatalf("空目标应照旧 skipped, got %+v", f)
	}
}
