package v1

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/plugins"
)

// 插件动作失败从前一律 400「请求参数有误，请检查后重试」：未启用、本机没装、
// 设置读不出这三种没有一种能靠改输入框解决，真机实测连原因文本都被抹掉。
func TestPluginActionErrorClassification(t *testing.T) {
	// 状态码也钉住：404=去刷新列表，409=去插件设置页重新保存，二者都不该是 400。
	wantStatus := map[string]int{
		"plugin_disabled":          409,
		"plugin_not_installed":     404,
		"plugin_config_unreadable": 409,
		"invalid_input":            400,
	}

	cases := []struct {
		name string
		err  error
		want string
	}{
		{"未启用", plugins.ErrPluginDisabled, "plugin_disabled"},
		{"未注册", fmt.Errorf("%w: plugin nope not found", plugins.ErrPluginNotRegistered), "plugin_not_installed"},
		{"已卸载", plugins.ErrPluginNotInstalled, "plugin_not_installed"},
		// 动作名写错：改输入有用，留 400，但要带哨兵（链层 21 个出口都改成经 unknownActionErr）
		{"动作不存在", fmt.Errorf("%w：「ping」不在该插件支持的动作里，可在插件面板查看它支持的动作", plugins.ErrUnknownAction), "invalid_input"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := pluginActionError(tc.err)
			if got == nil || got.Code != tc.want {
				t.Fatalf("归类错了：got code=%v want %s (err=%v)", got, tc.want, tc.err)
			}
			if wantStatus[tc.want] != 0 && got.HTTP != wantStatus[tc.want] {
				t.Errorf("%s 的状态码应是 %d，got %d", tc.want, wantStatus[tc.want], got.HTTP)
			}
		})
	}

	// 界面必须拿到动作名与下一步，不能还是那句泛化文案（WithDetail 直送链层自产中文）。
	ua := pluginActionError(fmt.Errorf("%w：「ping」不在该插件支持的动作里，可在插件面板查看它支持的动作", plugins.ErrUnknownAction))
	if !strings.Contains(ua.Message, "ping") || !strings.Contains(ua.Message, "插件面板") {
		t.Errorf("未知动作的界面文案丢了动作名或下一步: %q", ua.Message)
	}

	// #287 收尾：兜底按出口分开。插件自己跑挂（无哨兵的运行时错误）在 Action 上是
	// 502 并指向插件日志；同一个错误在 SaveConfig 上仍按「配置值不合插件心意」走 400。
	crash := errors.New("exit status 2: Traceback (most recent call last): boom")
	got := pluginActionErrorWith(crash, errcode.PluginActionFailed)
	if got.Code != "plugin_action_failed" || got.HTTP != 502 {
		t.Fatalf("插件运行时失败在 Action 出口应归 plugin_action_failed/502，got %s/%d", got.Code, got.HTTP)
	}
	if strings.Contains(got.Message, "参数有误") {
		t.Errorf("插件跑挂不该让用户去改输入: %q", got.Message)
	}
	if kept := pluginActionError(crash); kept.Code != "invalid_input" {
		t.Errorf("SaveConfig 的兜底不该跟着一起改（那条多半是用户改的配置值不合插件心意）: got %s", kept.Code)
	}

	// 坏配置那条：分类成 409，且界面拿到的 message 必须是插件给人看的那句话，
	// 不能再被泛化文案吃掉（这就是「新加的原因要当场找它的读者」）。
	human := "插件 demo 已保存的设置无法解析（可能是手工改过数据库或旧版本格式）：请在插件设置页重新保存一次，当前拒绝用默认配置顶替"
	resp := pluginActionError(&pluginConfigErrForTest{detail: human})
	if resp == nil || resp.Code != "plugin_config_unreadable" {
		t.Fatalf("坏配置应归到 plugin_config_unreadable, got %v", resp)
	}
	if msg := resp.WithDetail(errors.New(human)).Message; !strings.Contains(msg, "无法解析") || !strings.Contains(msg, "重新保存") {
		t.Errorf("人话句子必须原样出现在 message 里, got %q", msg)
	}
}

// pluginConfigErrForTest 复刻 plugins 包里那个未导出的载体：
// 接口层只依赖 errors.Is(ErrPluginConfigUnreadable) 与 Error() 文本。
type pluginConfigErrForTest struct{ detail string }

func (e *pluginConfigErrForTest) Error() string { return e.detail }
func (e *pluginConfigErrForTest) Unwrap() error { return plugins.ErrPluginConfigUnreadable }

// #301：启用/停用两个出口从前一律 400「请求参数有误，请检查后重试」，
// 而这里的每一档失败都不是用户输入的问题：被依赖、依赖没满足、权限没确认、本机没装。
// 更关键的是界面那条 catch 会把 message 原样弹出来 ⇒ 泛化文案＝原因彻底丢失。
func TestPluginStateErrorNotParameterBlame(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode string
		wantHTTP int
		inMsg    string // 界面必须看到的字
	}{
		{"被已启用的插件依赖", fmt.Errorf("%w：ptp-a 正被已启用的插件依赖（ptp-b），请先停用这些插件", plugins.ErrPluginStateRejected),
			"plugin_state_change_rejected", 409, "ptp-b"},
		{"依赖没满足", fmt.Errorf("%w：需要先满足该插件声明的依赖（需要先安装/启用依赖插件 ptp-c）", plugins.ErrPluginStateRejected),
			"plugin_state_change_rejected", 409, "ptp-c"},
		{"权限没确认", fmt.Errorf("%w：清单与插件当前声明的权限不一致（插件可能刚更新过）", plugins.ErrPluginPermissionsPending),
			"plugin_permissions_pending", 409, "插件权限"},
		{"本机没装", plugins.ErrPluginNotInstalled, "plugin_not_installed", 404, ""},
		{"设置读不出", &pluginConfigErrForTest{detail: "已保存的设置无法解析"}, "plugin_config_unreadable", 409, ""},
		{"请求体读不出形状", fmt.Errorf("%w（请求体里的 accepted_permissions 无法解析）: %w", errAcceptedPermissionsBad, errors.New("invalid character")),
			"invalid_input", 400, ""},
		{"存储故障兜底", errors.New("database is locked"), "plugin_state_change_rejected", 409, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := pluginStateError(tc.err)
			if got.Code != tc.wantCode {
				t.Fatalf("归类错了：code=%s want %s (err=%v)", got.Code, tc.wantCode, tc.err)
			}
			if got.HTTP != tc.wantHTTP {
				t.Errorf("%s 状态码=%d，期望 %d", tc.wantCode, got.HTTP, tc.wantHTTP)
			}
			if strings.Contains(got.Message, "参数有误") && tc.wantCode != "invalid_input" {
				t.Errorf("把插件关系/权限问题推给用户输入: %q", got.Message)
			}
			if tc.inMsg != "" && !strings.Contains(got.Message, tc.inMsg) {
				t.Errorf("界面文案丢了关键信息 %q: %q", tc.inMsg, got.Message)
			}
		})
	}

	// 链层原文必须自带「下一步去哪儿」：分类器只负责选码，句子是 validator 里写的。
	// 这里按源码断，避免我自己手抄一份 fixture 文案（抄的会漂）。
	src := stripCommentsForTest(readFileForTest(t, "plugin_install.go"))
	if !strings.Contains(src, "重新打开插件页") {
		t.Errorf("validateAcceptedPermissions 的句子丢了下一步（用户只知道「参数有误」）")
	}
}

// readFileForTest 读本包源码（go test 的 CWD 就是包目录，用裸文件名）。
func readFileForTest(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// stripCommentsForTest 剥掉 // 注释：静态判据只认代码，否则我自己写的说明会造出假绿/假红。
func stripCommentsForTest(src string) string {
	var out []string
	for _, line := range strings.Split(src, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
