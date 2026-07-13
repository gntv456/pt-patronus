package doctor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hxpt/ptpatronus/internal/core/config"
	"github.com/hxpt/ptpatronus/internal/sysinfo"
	"gorm.io/gorm"
)

// Check 是一次诊断检查；返回 Finding（panic 由 runner 兜底转 degraded）。
type Check func(ctx context.Context) Finding

// CheckDatabase 数据库连通性（ping）。
func CheckDatabase(db *gorm.DB) Check {
	return func(ctx context.Context) Finding {
		f := Finding{ID: "database", Severity: SevError, Title: "数据库连通性"}
		if db == nil {
			f.Status, f.Severity, f.Detail = StatusSkipped, SevInfo, "db 未注入"
			return f
		}
		sqlDB, err := db.DB()
		if err != nil {
			f.Status, f.Detail, f.Recommendation = StatusFailed, err.Error(), "检查 DB_TYPE / DATABASE_URL"
			return f
		}
		if err := sqlDB.PingContext(ctx); err != nil {
			f.Status, f.Detail, f.Recommendation = StatusFailed, err.Error(), "数据库不可达：确认服务运行 / 磁盘空间 / 文件锁"
			return f
		}
		f.Status, f.Severity, f.Detail = StatusOK, SevInfo, "ping 成功"
		return f
	}
}

// CheckRuntimePaths DataDir 存在且可写。
func CheckRuntimePaths(cfg *config.Config) Check {
	return func(ctx context.Context) Finding {
		f := Finding{ID: "runtime.paths", Severity: SevWarn, Title: "运行目录可写"}
		dir := cfg.DataDir
		if dir == "" {
			dir = "./data"
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			f.Status, f.Detail, f.Recommendation = StatusFailed, fmt.Sprintf("创建 %s 失败: %v", dir, err), "检查 DATA_DIR 权限 / 挂载"
			return f
		}
		probe := filepath.Join(dir, ".doctor_probe")
		if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
			f.Status, f.Detail, f.Recommendation = StatusFailed, fmt.Sprintf("写入 %s 失败: %v", dir, err), "检查目录写权限"
			return f
		}
		os.Remove(probe)
		f.Status, f.Severity, f.Detail, f.Context = StatusOK, SevInfo, dir+" 可写", map[string]any{"data_dir": dir}
		return f
	}
}

// CheckSystemResources 内存 / 磁盘余量（gopsutil）。
func CheckSystemResources() Check {
	return func(ctx context.Context) Finding {
		f := Finding{ID: "system.resources", Severity: SevWarn, Title: "系统资源"}
		m := sysinfo.CollectSystemMetrics()
		ctxMap := map[string]any{
			"mem_usage": fmt.Sprintf("%.1f%%", m.MemUsage),
			"mem_total": m.MemTotal,
			"cpu_cores": m.CPUCores,
			"os":        m.OS,
			"hostname":  m.Hostname,
		}
		var critical []string
		for _, d := range m.Disks {
			ctxMap["disk:"+d.Device] = fmt.Sprintf("%.1f%%", d.Usage)
			if d.Usage >= 95 {
				critical = append(critical, fmt.Sprintf("%s %.1f%%", d.Device, d.Usage))
			}
		}
		if m.MemUsage >= 95 {
			critical = append(critical, fmt.Sprintf("内存 %.1f%%", m.MemUsage))
		}
		if len(critical) > 0 {
			f.Status, f.Detail, f.Recommendation = StatusDegraded, "资源紧张: "+strings.Join(critical, ", "), "清理或扩容"
			return f
		}
		f.Status, f.Severity, f.Detail, f.Context = StatusOK, SevInfo, "资源正常", ctxMap
		return f
	}
}

// CheckConfigSecurity release 模式下密钥强度。
func CheckConfigSecurity(cfg *config.Config) Check {
	return func(ctx context.Context) Finding {
		f := Finding{ID: "config.security", Severity: SevWarn, Title: "密钥强度"}
		if !cfg.IsRelease() {
			f.Status, f.Severity, f.Detail = StatusOK, SevInfo, "debug 模式（跳过强度校验）"
			return f
		}
		if len(cfg.SecretKey) < 32 {
			f.Status, f.Detail, f.Recommendation = StatusFailed, "SECRET_KEY < 32 字节", "轮换为强随机密钥"
			return f
		}
		f.Status, f.Severity, f.Detail = StatusOK, SevInfo, "SECRET_KEY 强度合格"
		return f
	}
}

// CheckSafeMode 报告安全模式状态。
func CheckSafeMode(cfg *config.Config) Check {
	return func(ctx context.Context) Finding {
		f := Finding{ID: "startup.safe_mode", Severity: SevInfo, Title: "安全模式", Context: map[string]any{"safe_mode": cfg.SafeMode}}
		if cfg.SafeMode {
			f.Status, f.Detail = StatusOK, "SAFE_MODE 已启用（插件 / 调度 / 监控未激活）"
		} else {
			f.Status, f.Detail = StatusOK, "SAFE_MODE 未启用（正常运行）"
		}
		return f
	}
}
