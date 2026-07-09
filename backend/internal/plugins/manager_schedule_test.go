package plugins

import (
	"context"
	"testing"
)

// fakeScheduler 记录 AddFunc/Remove 调用，供调度逻辑单测断言（不真跑 cron）。
type fakeScheduler struct {
	jobs map[string]string // name → spec
}

func (f *fakeScheduler) AddFunc(name, spec string, _ func()) error {
	if f.jobs == nil {
		f.jobs = map[string]string{}
	}
	f.jobs[name] = spec
	return nil
}

func (f *fakeScheduler) Remove(name string) {
	delete(f.jobs, name)
}

func (f *fakeScheduler) Start() {}

func (f *fakeScheduler) Stop() context.Context { return context.Background() }

func TestEffectiveCron(t *testing.T) {
	// 1) 无 CronConfigKey → 用 Cron
	if got := effectiveCron(ScheduleSpec{Cron: "0 6 * * *"}, nil); got != "0 6 * * *" {
		t.Fatalf("no config key: got %q", got)
	}
	// 2) CronConfigKey 指向非空配置 → 用配置值覆盖
	cfg := map[string]any{"cron_update": "*/2 * * * *"}
	if got := effectiveCron(ScheduleSpec{Cron: "0 6 * * *", CronConfigKey: "cron_update"}, cfg); got != "*/2 * * * *" {
		t.Fatalf("config override: got %q", got)
	}
	// 3) CronConfigKey 指向空串 → 停用（返回空）
	cfg2 := map[string]any{"cron_update": ""}
	if got := effectiveCron(ScheduleSpec{Cron: "0 6 * * *", CronConfigKey: "cron_update"}, cfg2); got != "" {
		t.Fatalf("empty config should disable: got %q", got)
	}
	// 4) 配置缺键 → 视为空 → 停用
	if got := effectiveCron(ScheduleSpec{Cron: "0 6 * * *", CronConfigKey: "missing"}, map[string]any{}); got != "" {
		t.Fatalf("missing config key should disable: got %q", got)
	}
	// 5) 空白串 trim 后空 → 停用
	if got := effectiveCron(ScheduleSpec{Cron: "0 6 * * *", CronConfigKey: "k"}, map[string]any{"k": "   "}); got != "" {
		t.Fatalf("whitespace should disable: got %q", got)
	}
}

// TestPluginScheduleRegistration 用 cloudflare-subscribe（带 daily_update 调度 + cron_update 配置键）
// 验证 refreshSchedules 在 enable/config 变更时的注册/覆盖/停用语义。
func TestPluginScheduleRegistration(t *testing.T) {
	db := newPluginTestDB(t)
	m := NewManager(db, nil, nil, nil, nil)
	fake := &fakeScheduler{}
	m.sched = fake // 注入假调度器（绕过 Start 里的 scheduler.New）

	const job = "plugin:cloudflare-subscribe:daily_update"

	// 未启用 → 不登记任何 job
	m.refreshSchedules("cloudflare-subscribe")
	if len(fake.jobs) != 0 {
		t.Fatalf("disabled plugin should register no jobs, got %v", fake.jobs)
	}

	// 启用（默认配置 cron_update="0 6 * * *"）→ 登记一条默认 cron
	if err := m.SetEnabled("cloudflare-subscribe", true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if got := fake.jobs[job]; got != "0 6 * * *" {
		t.Fatalf("after enable expected default cron %q, got %q (all=%v)", "0 6 * * *", got, fake.jobs)
	}

	// 配置覆盖 cron → 旧 job 被替换成新 spec
	if _, err := m.SaveConfig("cloudflare-subscribe", map[string]any{"cron_update": "*/2 * * * *"}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if got := fake.jobs[job]; got != "*/2 * * * *" {
		t.Fatalf("after config override expected %q, got %q", "*/2 * * * *", got)
	}

	// 清空 cron 配置 → 该条调度停用
	if _, err := m.SaveConfig("cloudflare-subscribe", map[string]any{"cron_update": ""}); err != nil {
		t.Fatalf("clear cron config: %v", err)
	}
	if _, ok := fake.jobs[job]; ok {
		t.Fatalf("empty cron should disable schedule, jobs=%v", fake.jobs)
	}

	// 重新设回 cron → 恢复登记
	if _, err := m.SaveConfig("cloudflare-subscribe", map[string]any{"cron_update": "0 6 * * *"}); err != nil {
		t.Fatalf("restore cron: %v", err)
	}
	if got := fake.jobs[job]; got != "0 6 * * *" {
		t.Fatalf("restore expected default cron, got %q", got)
	}

	// 禁用插件 → 清理 job
	if err := m.SetEnabled("cloudflare-subscribe", false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if _, ok := fake.jobs[job]; ok {
		t.Fatalf("disabled plugin should have no jobs, jobs=%v", fake.jobs)
	}
}

// TestPluginWithoutScheduleNoOp 确保无 Schedule 声明的插件（如 json-toolbox）不触发任何调度。
func TestPluginWithoutScheduleNoOp(t *testing.T) {
	db := newPluginTestDB(t)
	m := NewManager(db, nil, nil, nil, nil)
	fake := &fakeScheduler{}
	m.sched = fake
	if err := m.SetEnabled("json-toolbox", true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if len(fake.jobs) != 0 {
		t.Fatalf("plugin without schedule should register nothing, got %v", fake.jobs)
	}
}
