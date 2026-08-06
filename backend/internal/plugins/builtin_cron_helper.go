package plugins

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

type cronHelperPlugin struct{}

func NewCronHelperPlugin() Plugin { return cronHelperPlugin{} }

func (cronHelperPlugin) Manifest() Manifest {
	return Manifest{
		ID:          "cron-helper",
		Name:        "Cron 助手",
		Version:     "1.0.0",
		Author:      "PTPatronus",
		Description: "校验 Cron 表达式并计算未来执行时间，配置定时任务前先预览结果。",
		Runtime:     RuntimeBuiltin,
		Builtin:     true,
		Permissions: []string{"action:execute"},
		Actions: []ActionSpec{
			{
				Name:        "next_runs",
				Label:       "计算执行时间",
				Description: "按标准 5 段 Cron 表达式计算接下来几次执行时间。",
				InputSchema: []ConfigField{
					{Key: "expression", Label: "Cron 表达式", Type: "text", Required: true, Default: "0 3 * * *"},
					{Key: "count", Label: "次数", Type: "number", Default: 5},
					{Key: "timezone", Label: "时区", Type: "text", Default: "Asia/Shanghai"},
				},
			},
		},
	}
}

func (cronHelperPlugin) DefaultConfig() map[string]any { return map[string]any{} }

func (cronHelperPlugin) HandleEvent(context.Context, Runtime, Event) error { return nil }

func (cronHelperPlugin) ExecuteAction(ctx context.Context, rt Runtime, action string, input map[string]any) (map[string]any, error) {
	_ = ctx
	if action != "next_runs" {
		return nil, fmt.Errorf("unknown action %s", action)
	}
	expr := strings.TrimSpace(pluginString(input["expression"]))
	if expr == "" {
		return nil, errors.New("Cron 表达式不能为空")
	}
	schedule, err := cron.ParseStandard(expr)
	if err != nil {
		return nil, err
	}
	locName := strings.TrimSpace(pluginString(input["timezone"]))
	if locName == "" {
		locName = "Asia/Shanghai"
	}
	loc, err := time.LoadLocation(locName)
	if err != nil {
		return nil, err
	}
	count := pluginInt(input["count"], 5)
	if count <= 0 {
		count = 5
	}
	if count > 20 {
		count = 20
	}
	next := make([]string, 0, count)
	t := time.Now().In(loc)
	for i := 0; i < count; i++ {
		t = schedule.Next(t).In(loc)
		next = append(next, t.Format("2006-01-02 15:04:05 MST"))
	}
	rt.Log(logInfo, action, "cron next runs calculated", map[string]any{"expression": expr, "count": count})
	return map[string]any{"expression": expr, "timezone": locName, "next_runs": next}, nil
}
