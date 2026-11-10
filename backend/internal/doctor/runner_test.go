// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package doctor

import (
	"context"
	"testing"
)

func TestRunPanicIsolation(t *testing.T) {
	checks := []Check{
		func(ctx context.Context) Finding {
			return Finding{ID: "ok1", Severity: SevInfo, Status: StatusOK, Title: "good"}
		},
		func(ctx context.Context) Finding {
			panic("boom")
		},
		func(ctx context.Context) Finding {
			return Finding{ID: "warn1", Severity: SevWarn, Status: StatusDegraded, Title: "degraded"}
		},
	}
	r := Run(context.Background(), checks, nil)
	if len(r.Findings) != 3 {
		t.Fatalf("want 3 findings, got %d", len(r.Findings))
	}
	// 第二项 panic → degraded/error
	if r.Findings[1].Status != StatusDegraded || r.Findings[1].Severity != SevError {
		t.Errorf("panic not isolated: %+v", r.Findings[1])
	}
	if r.Summary.Total != 3 || r.Summary.Info != 1 || r.Summary.Warn != 1 || r.Summary.Error != 1 {
		t.Errorf("summary wrong: %+v", r.Summary)
	}
	if r.Status != ReportFailed {
		t.Errorf("want ReportFailed (error present), got %s", r.Status)
	}
}

func TestRunAllOKHealthy(t *testing.T) {
	checks := []Check{
		func(ctx context.Context) Finding { return Finding{ID: "a", Severity: SevInfo, Status: StatusOK} },
		func(ctx context.Context) Finding { return Finding{ID: "b", Severity: SevInfo, Status: StatusOK} },
	}
	r := Run(context.Background(), checks, map[string]any{"version": "x"})
	if r.Status != ReportHealthy {
		t.Errorf("want healthy, got %s", r.Status)
	}
	if r.Environment["version"] != "x" {
		t.Errorf("env not passed: %+v", r.Environment)
	}
}
