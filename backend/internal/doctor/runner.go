package doctor

import (
	"context"
	"fmt"
	"time"
)

// Run 执行所有检查（per-check recover 隔离 panic），汇总 Report。
// env 透传到 Report.Environment（version / safe_mode / release 等由 handler 填）。
func Run(ctx context.Context, checks []Check, env map[string]any) *Report {
	r := &Report{
		GeneratedAt: time.Now(),
		Environment: env,
		Findings:    make([]Finding, 0, len(checks)),
	}
	worst := SevInfo
	for _, c := range checks {
		var f Finding
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					f = Finding{
						ID: "doctor.panic", Severity: SevError, Status: StatusDegraded,
						Title: "检查异常", Detail: fmt.Sprintf("panic: %v", rec),
						Recommendation: "查看日志定位该检查项",
					}
				}
			}()
			f = c(ctx)
		}()
		if f.ID == "" {
			f.ID = "unknown"
		}
		r.Findings = append(r.Findings, f)
		r.Summary.Total++
		switch f.Severity {
		case SevInfo:
			r.Summary.Info++
		case SevWarn:
			r.Summary.Warn++
		case SevError:
			r.Summary.Error++
		}
		if f.Status == StatusFixed {
			r.Summary.Fixed++
		}
		if rank(f.Severity) > rank(worst) {
			worst = f.Severity
		}
	}
	switch worst {
	case SevError:
		r.Status = ReportFailed
	case SevWarn:
		r.Status = ReportDegraded
	default:
		r.Status = ReportHealthy
	}
	r.computeScore()
	return r
}

func rank(s Severity) int {
	switch s {
	case SevInfo:
		return 0
	case SevWarn:
		return 1
	case SevError:
		return 2
	}
	return 0
}
