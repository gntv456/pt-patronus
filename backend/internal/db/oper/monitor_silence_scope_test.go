package oper

import (
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
)

// TestActiveSilenceUntilScope 静默继承的查询口径必须在 DB 层钉住：
// 静默记在某一条告警上，而抖动会新建行——查询要能跨「已恢复的旧行」找到仍未过期的静默，
// 又不能把它扩散到别的指标或别的设备（否则「静默这台盘的 CPU」会变成整台设备静音）。
func TestActiveSilenceUntilScope(t *testing.T) {
	db := newMonitorDB(t)
	op := NewMonitorOper(db, security.NewCrypto("silence-scope-seed"))
	target, err := op.Create(TargetInput{Name: "nas-1", Type: "nas", Protocol: "local", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	other, err := op.Create(TargetInput{Name: "nas-2", Type: "nas", Protocol: "local", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	row := &models.MonitorAlert{TargetID: target.ID, Metric: "offline", Level: "danger", Message: "掉线", CreatedAt: now}
	if err := op.CreateAlert(row); err != nil {
		t.Fatal(err)
	}
	until := now.Add(30 * time.Minute)
	if err := op.SilenceAlert(row.ID, until, "me"); err != nil {
		t.Fatal(err)
	}
	// 旧行已恢复（静默就挂在这条恢复行上），再触发会产生新行：查询必须仍然找得到静默。
	if err := op.ResolveOpenAlerts(target.ID, "offline"); err != nil {
		t.Fatal(err)
	}
	if err := op.CreateAlert(&models.MonitorAlert{TargetID: target.ID, Metric: "offline", Level: "danger", Message: "又掉线", CreatedAt: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}

	got, err := op.ActiveSilenceUntil(target.ID, "offline", now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("未过期的静默必须跨「已恢复的旧行」被查出来，否则抖动一次就绕过静默")
	}
	if !got.After(now.Add(25 * time.Minute)) {
		t.Errorf("继承到的静默截止时间 = %v，期望接近 %v", *got, until)
	}

	// 反证一：静默窗口过后不得再造出静默（否则「静默」永久化，告警再也不响）。
	if got, err := op.ActiveSilenceUntil(target.ID, "offline", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	} else if got != nil {
		t.Errorf("静默已过期应返回 nil，got %v", *got)
	}
	// 反证二：作用域不能外溢到别的指标或别的设备。
	if got, err := op.ActiveSilenceUntil(target.ID, "cpu_usage", now); err != nil {
		t.Fatal(err)
	} else if got != nil {
		t.Errorf("静默 offline 不该影响 cpu_usage，got %v", *got)
	}
	if got, err := op.ActiveSilenceUntil(other.ID, "offline", now); err != nil {
		t.Fatal(err)
	} else if got != nil {
		t.Errorf("静默不该跨设备生效，got %v", *got)
	}
}
