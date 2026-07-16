package assessment

import (
	"strings"
	"testing"
	"time"
)

// TestParseNexusPHPAssessment 单行完整指标（Phase 1）+ header + 状态。
func TestParseNexusPHPAssessment(t *testing.T) {
	text := `用户资料
考核
名称：新手考核 时间：2026-01-01 00:00 ~ 2026-12-31 23:59
指标1：做种积分增量, 要求：500 , 当前：10331, 结果：完成！
指标2：上传量, 要求：100GB , 当前：120GB, 结果：通过
指标3：分享率, 要求：1.5 , 当前：1.2, 结果：未达标`
	tasks := parseNexusPHPAssessment(text)
	if len(tasks) != 1 {
		t.Fatalf("tasks: got %d want 1", len(tasks))
	}
	tk := tasks[0]
	if tk.Name != "新手考核" {
		t.Errorf("name: got %q want 新手考核", tk.Name)
	}
	if len(tk.Metrics) != 3 {
		t.Fatalf("metrics: got %d want 3", len(tk.Metrics))
	}
	// 上传量 100GB / 120GB → 单位 GB，已达标
	var up Metric
	for _, m := range tk.Metrics {
		if m.Label == "上传量" {
			up = m
		}
	}
	if up.Unit != "GB" {
		t.Errorf("上传量 unit: got %q want GB", up.Unit)
	}
	if up.Required != 100 || up.Current != 120 {
		t.Errorf("上传量 values: req=%v cur=%v want 100/120", up.Required, up.Current)
	}
	if !up.IsCompleted {
		t.Error("上传量 should be met")
	}
	// 分享率未达标 → 非 allMet → ACTIVE（endTime 未过期）
	if tk.Status != StatusActive {
		t.Errorf("status: got %q want ACTIVE (one metric unmet, not expired)", tk.Status)
	}
	if tk.EndTime == 0 {
		t.Error("endTime should parse to non-zero")
	}
}

// TestParseNexusPHPAssessment_SplitLines 拆行指标（Phase 2 逐行回退）。
func TestParseNexusPHPAssessment_SplitLines(t *testing.T) {
	text := "考核\n名称：长期考核 时间：2025-01-01 00:00 ~ 2026-12-31 23:59\n" +
		"指标1：做种积分增量\n要求：500\n当前：100\n结果：未达标"
	tasks := parseNexusPHPAssessment(text)
	if len(tasks) != 1 {
		t.Fatalf("tasks: got %d want 1", len(tasks))
	}
	if len(tasks[0].Metrics) != 1 {
		t.Fatalf("metrics: got %d want 1 (Phase 2 line-by-line)", len(tasks[0].Metrics))
	}
	m := tasks[0].Metrics[0]
	if m.Label != "做种积分增量" {
		t.Errorf("label: got %q", m.Label)
	}
	if m.Required != 500 || m.Current != 100 {
		t.Errorf("values: req=%v cur=%v", m.Required, m.Current)
	}
}

// TestNormMetricValue 单位归一（size→GB、time→小时）。
func TestNormMetricValue(t *testing.T) {
	cases := []struct {
		s, name string
		want    float64
	}{
		{"100GB", "上传量", 100},
		{"1TB", "上传量", 1024},
		{"512MB", "下载量", 0.5},
		{"72H", "做种时间", 72},
		{"3D", "做种时间", 72},
		{"1.5", "分享率", 1.5},
	}
	for _, c := range cases {
		if got := normMetricValue(c.s, c.name); got != c.want {
			t.Errorf("normMetricValue(%q,%q) = %v want %v", c.s, c.name, got, c.want)
		}
	}
}

// TestMTeamExamineMetrics 馒头拼写错误字段 uploaed/downloaed/bonuses。
func TestMTeamExamineMetrics(t *testing.T) {
	item := map[string]any{
		"uploadTarget":   float64(100 * 1073741824), // 100GB
		"uploaed":        float64(120 * 1073741824), // 拼写错误，优先
		"uploaded":       float64(999 * 1073741824), // 应被忽略
		"downloadTarget": float64(50 * 1073741824),
		"downloaed":      float64(60 * 1073741824),
		"bonusTarget":    float64(1000),
		"bonuses":        float64(1500), // 复数，优先
	}
	metrics := mTeamExamineMetrics(item)
	byLabel := map[string]Metric{}
	for _, m := range metrics {
		byLabel[m.Label] = m
	}
	if up := byLabel["上传量"]; up.Current != 120 || up.Required != 100 || !up.IsCompleted {
		t.Errorf("上传量: cur=%v req=%v met=%v want 120/100/true (uploaed typo priority)", up.Current, up.Required, up.IsCompleted)
	}
	if bn := byLabel["魔力值"]; bn.Current != 1500 || bn.Required != 1000 || !bn.IsCompleted {
		t.Errorf("魔力值: cur=%v req=%v want 1500/1000 (bonuses plural)", bn.Current, bn.Required)
	}
}

func TestMTeamExamineAcceptsNumericCode(t *testing.T) {
	body := []byte(`{"code":0,"data":[{"name":"numeric-code","uploadTarget":"1073741824","uploaded":"1073741824"}]}`)
	tasks := parseMTeamExamine(body)
	if len(tasks) != 1 || tasks[0].Name != "numeric-code" {
		t.Fatalf("tasks=%+v", tasks)
	}
}

// TestMTeamExamineTimeString 馒头考核时间为 "2006-01-02 15:04:05" 字符串（同 freeleech 端点），
// 必须经 ParseInLocation 解析；旧实现走数字拼接会把 "2026-01-02 03:04:05" 拼成
// 20260102030405ms（≈公元 2600 年），导致前端考核截止时间显示错乱。
func TestMTeamExamineTimeString(t *testing.T) {
	body := []byte(`{"code":"0","data":[{
		"name":"保种考核",
		"uploadTarget": 549755813888,
		"uploaed":      322122547200,
		"startTime": "2026-01-01 00:00:00",
		"endTime":   "2026-12-31 23:59:59"
	}]}`)
	tasks := parseMTeamExamine(body)
	if len(tasks) != 1 {
		t.Fatalf("tasks: got %d want 1", len(tasks))
	}
	tk := tasks[0]
	if tk.Name != "保种考核" {
		t.Errorf("name: got %q want 保种考核", tk.Name)
	}
	if tk.StartTime == 0 || tk.EndTime == 0 {
		t.Fatalf("time must parse non-zero: start=%d end=%d", tk.StartTime, tk.EndTime)
	}
	// 反解年份须落在 2025~2027（含时区 ±14h 偏移），证明走的是日期解析而非数字拼接
	start := time.UnixMilli(tk.StartTime)
	end := time.UnixMilli(tk.EndTime)
	for _, tt := range []time.Time{start, end} {
		if y := tt.Year(); y < 2025 || y > 2027 {
			t.Errorf("year %d out of [2025,2027] — string date likely got digit-concatenated into a bogus epoch", y)
		}
	}
	if !end.After(start) {
		t.Errorf("end %v should be after start %v", end, start)
	}
}

// TestMTeamExamineTimeNumeric 纯数字时间（epoch s/ms）仍能正确解析，不被字符串分支误伤。
func TestMTeamExamineTimeNumeric(t *testing.T) {
	body := []byte(`{"code":"0","data":[{
		"name":"新手考核",
		"uploadTarget": 549755813888,
		"uploaed":      549755813888,
		"startTime": 1735689600,
		"endTime":   1798761599
	}]}`)
	tasks := parseMTeamExamine(body)
	if len(tasks) != 1 {
		t.Fatalf("tasks: got %d want 1", len(tasks))
	}
	tk := tasks[0]
	if tk.StartTime == 0 || tk.EndTime == 0 {
		t.Fatalf("numeric time must parse: start=%d end=%d", tk.StartTime, tk.EndTime)
	}
	start := time.UnixMilli(tk.StartTime)
	if y := start.Year(); y < 2024 || y > 2026 {
		t.Errorf("start year %d want ~2025 (epoch s 1735689600 = 2025-01-01 UTC)", y)
	}
}

// TestMTeamExamineRelativeTime 馒头常以描述串给倒计时（"距離考核結束還有4 天"）而非绝对日期：
// 此时不应跳过任务，EndTime 应由倒计时派生（now + 4 天）。
func TestMTeamExamineRelativeTime(t *testing.T) {
	body := []byte(`{"code":"0","data":[{
		"name":"新手考核",
		"uploadTarget": 549755813888,
		"uploaed":      1073741824,
		"description":"距離考核結束還有4 天，考核期間不享受所有魔力加成。"
	}]}`)
	tasks := parseMTeamExamine(body)
	if len(tasks) != 1 {
		t.Fatalf("tasks: got %d want 1 (no date fields → must still produce task from countdown)", len(tasks))
	}
	tk := tasks[0]
	if tk.EndTime == 0 {
		t.Fatal("EndTime must be derived from countdown text")
	}
	diff := time.Duration(tk.EndTime-time.Now().UnixMilli()) * time.Millisecond
	if diff < 3*24*time.Hour || diff > 5*24*time.Hour {
		t.Errorf("EndTime should be ~4 days out (from 還有4 天), got %v", diff)
	}
	// 上传量 1GB < 目标 512GB → 未达标 → 非 COMPLETED
	if tk.Status == StatusCompleted {
		t.Error("status should not be COMPLETED (upload 1GB < target 512GB)")
	}
	// 说明串应被捕获（含倒计时 + "不享受所有魔力加成"）
	if !strings.Contains(tk.Description, "加成") {
		t.Errorf("description should contain 加成 note, got %q", tk.Description)
	}
}

// TestMTeamExamineNoMetrics 仅有倒计时、无任何指标字段：不跳过，状态 ACTIVE，EndTime 来自倒计时。
func TestMTeamExamineNoMetrics(t *testing.T) {
	body := []byte(`{"code":"0","data":[{
		"name":"保种考核",
		"notice":"距离考核结束还有 7 天"
	}]}`)
	tasks := parseMTeamExamine(body)
	if len(tasks) != 1 {
		t.Fatalf("tasks: got %d want 1 (countdown-only item must not be skipped)", len(tasks))
	}
	tk := tasks[0]
	if tk.Status != StatusActive {
		t.Errorf("status: got %q want ACTIVE (no metrics → not auto-completed)", tk.Status)
	}
	if tk.EndTime == 0 {
		t.Fatal("EndTime must derive from countdown")
	}
	diff := time.Duration(tk.EndTime-time.Now().UnixMilli()) * time.Millisecond
	if diff < 6*24*time.Hour || diff > 8*24*time.Hour {
		t.Errorf("EndTime should be ~7 days out, got %v", diff)
	}
}
