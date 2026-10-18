package app

import (
	"context"
	"errors"
	"strings"
	"testing"

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
