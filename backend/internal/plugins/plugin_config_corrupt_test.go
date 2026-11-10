// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// 插件「已保存的设置」读不出时的账面。旧行为把两种截然不同的事合成同一个静默分支：
// 坏 JSON 悄悄退回默认配置继续跑（插件拿着空密码动作），定时/事件出口则当成
// 「用户没启用」整轮跳过——界面、日志、通知三处都看不出插件其实已经不干活了。

func setRawPluginConfig(t *testing.T, db *gorm.DB, id, raw string) {
	t.Helper()
	if err := db.Model(&models.PluginState{}).Where("id = ?", id).
		Update("config_json", raw).Error; err != nil {
		t.Fatalf("写 config_json=%q 失败: %v", raw, err)
	}
}

func corruptConfigManager(t *testing.T) (*Manager, *gorm.DB, *fakeScheduler) {
	t.Helper()
	db := newPluginTestDB(t)
	m := NewManager(db, nil, nil, nil, nil)
	mustRegister(t, m, NewCloudflareSubscribePlugin())
	fake := &fakeScheduler{}
	m.sched = fake
	return m, db, fake
}

const corruptReason = "无法解析"

func TestCorruptPluginConfigIsRejectedNotDefaulted(t *testing.T) {
	const id = "cloudflare-subscribe"
	m, db, _ := corruptConfigManager(t)
	if err := m.SetEnabled(id, true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	setRawPluginConfig(t, db, id, `{"cron_update": `)

	p, err := m.plugin(id)
	if err != nil {
		t.Fatalf("plugin: %v", err)
	}
	st, cfg, err := m.stateAndConfig(p)
	if err == nil {
		t.Fatalf("坏配置应被拒绝；旧行为是交回默认配置继续跑 cfg=%v st=%+v", cfg, st)
	}
	// 出口分类靠哨兵，不靠字符串相似度：接口层据此选择 409 + 原样透传这句话。
	if !errors.Is(err, ErrPluginConfigUnreadable) {
		t.Fatalf("错误必须能被 errors.Is 判为 ErrPluginConfigUnreadable（否则接口层又只能报参数有误）, got %v", err)
	}
	if !strings.Contains(err.Error(), corruptReason) {
		t.Fatalf("人话句子要留在 Error() 里（WithDetail 取的就是它）, got %v", err)
	}
	if !strings.Contains(err.Error(), corruptReason) {
		t.Fatalf("原因要点名「%s」并给出可恢复动作, got=%v", corruptReason, err)
	}
	if !strings.Contains(err.Error(), "重新保存") {
		t.Errorf("要把用户能做的事写进原因（去设置页重新保存）, got=%v", err)
	}
	if cfg != nil {
		t.Errorf("拒绝时不该交回一份配置继续用, got=%v", cfg)
	}

	// 正控：合法的部分配置照旧与默认值合并，别把正常路径一起掐掉。
	if _, err := m.SaveConfig(id, map[string]any{"cron_update": "*/5 * * * *"}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	_, merged, err := m.stateAndConfig(p)
	if err != nil {
		t.Fatalf("合法配置不该报错: %v", err)
	}
	if got, _ := merged["cron_update"].(string); got != "*/5 * * * *" {
		t.Fatalf("配置应被合并使用, got %q", got)
	}
}

func TestCorruptPluginConfigLeavesTrailOnScheduleOutlet(t *testing.T) {
	const id = "cloudflare-subscribe"
	m, db, fake := corruptConfigManager(t)
	if err := m.SetEnabled(id, true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	setRawPluginConfig(t, db, id, "不是 JSON")

	// 出口一：定时任务注册。不注册是对的（配置不可信就不能按默认 cron 跑），
	// 但必须在插件日志里说得出原因，否则用户只看到「这个插件从来不执行」。
	m.refreshSchedules(id)
	if len(fake.jobs) != 0 {
		t.Errorf("坏配置时不该注册任何 job, got=%v", fake.jobs)
	}
	logs, err := m.Logs(id, 20)
	if err != nil {
		t.Fatalf("读插件日志: %v", err)
	}
	var found bool
	for _, l := range logs {
		if strings.Contains(l.Message, corruptReason) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("调度出口要把读失败留在插件日志里（面板渲染它）, logs=%+v", logs)
	}
}

func TestCorruptPluginConfigMarksEventFailureOnEventOutlet(t *testing.T) {
	// 出口二：事件投递用 event-audit（它声明了 plugin.test 事件）。
	db := newPluginTestDB(t)
	m := NewManager(db, nil, nil, nil, nil)
	const id = "event-audit"
	mustRegister(t, m, NewEventAuditPlugin())
	if err := m.SetEnabled(id, true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	setRawPluginConfig(t, db, id, "不是 JSON")

	m.handleEvent(context.Background(), id, Event{Type: "plugin.test"})
	var st models.PluginState
	if err := db.First(&st, "id = ?", id).Error; err != nil {
		t.Fatalf("读 state: %v", err)
	}
	if !strings.Contains(st.LastError, corruptReason) {
		t.Fatalf("事件出口读失败必须写进 last_error（插件面板渲染它）, got %q", st.LastError)
	}

	// 正控：配置合法时同一条事件不该被记成读失败。
	if _, err := m.SaveConfig(id, map[string]any{}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	m.handleEvent(context.Background(), id, Event{Type: "plugin.test"})
	var after models.PluginState
	if err := db.First(&after, "id = ?", id).Error; err != nil {
		t.Fatalf("复读 state: %v", err)
	}
	if strings.Contains(after.LastError, corruptReason) {
		t.Errorf("合法配置被当成读失败: last_error=%q", after.LastError)
	}
}

// 读侧的整批风险：List 若因第一个坏配置就 return err，界面上会显示「没有任何插件」，
// 其余正常插件一起消失——修「配置坏掉被当成没配」不能顺手造成新的整批假空。
func TestPluginListSurvivesOneCorruptConfig(t *testing.T) {
	db := newPluginTestDB(t)
	m := NewManager(db, nil, nil, nil, nil)
	mustRegister(t, m, NewCloudflareSubscribePlugin())
	mustRegister(t, m, NewEventAuditPlugin())
	if err := m.SetEnabled("cloudflare-subscribe", true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	setRawPluginConfig(t, db, "cloudflare-subscribe", "不是 JSON")

	views, err := m.List()
	if err != nil {
		t.Fatalf("一个坏配置不该让整批列表失败: %v", err)
	}
	var bad, ok int
	for _, v := range views {
		if v.ID == "cloudflare-subscribe" {
			bad++
			if !strings.Contains(v.LastError, corruptReason) {
				t.Errorf("坏配置那行要带原因（面板渲染 last_error）, got %q", v.LastError)
			}
			if v.Config["cron_update"] == nil {
				t.Errorf("降级行仍要渲染得出设置表单（用默认值占位）, config=%v", v.Config)
			}
			continue
		}
		ok++
		if strings.Contains(v.LastError, corruptReason) {
			t.Errorf("正常插件被连坐记成读失败: %s last_error=%q", v.ID, v.LastError)
		}
	}
	if bad != 1 || ok < 1 {
		t.Fatalf("应正好 1 行坏配置 + 至少 1 行正常, got bad=%d ok=%d total=%d", bad, ok, len(views))
	}
}
