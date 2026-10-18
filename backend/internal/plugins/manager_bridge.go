package plugins

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

func (m *Manager) Bridge(ctx context.Context, id, method string, params map[string]any) (map[string]any, error) {
	if params == nil {
		params = map[string]any{}
	}
	p, err := m.plugin(id)
	if err != nil {
		return nil, err
	}
	st, cfg, err := m.stateAndConfig(p)
	if err != nil {
		return nil, err
	}
	if !st.Enabled {
		return nil, ErrPluginDisabled
	}
	manifest := p.Manifest()
	rt := m.runtime(id, cfg)
	require := func(permission string) error {
		if hasPluginPermission(manifest.Permissions, permission) {
			return nil
		}
		return fmt.Errorf("permission %s is required", permission)
	}
	switch strings.TrimSpace(method) {
	case "capabilities":
		return map[string]any{
			"api_version":       CurrentPluginAPIVersion,
			"plugin_id":         id,
			"permissions":       manifest.Permissions,
			"host_capabilities": supportedHostCapabilities(),
			"methods": []string{
				"capabilities",
				"config.get",
				"log.write",
				"kv.get",
				"kv.set",
				"kv.delete",
				"event.publish",
				"notice.send",
				"sites.list",
				"action.invoke",
				"tool.list",
				"tool.call",
			},
		}, nil
	case "config.get":
		if err := require("config:read"); err != nil {
			return nil, err
		}
		return map[string]any{"config": rt.Config()}, nil
	case "log.write":
		if err := require("log:write"); err != nil {
			return nil, err
		}
		level := bridgeString(params, "level", logInfo)
		eventName := bridgeString(params, "event", "ui.bridge")
		message := bridgeString(params, "message", "")
		rt.Log(level, eventName, message, bridgeMap(params, "data"))
		return map[string]any{"ok": true}, nil
	case "kv.get":
		if err := require("kv:read"); err != nil {
			return nil, err
		}
		key := bridgeString(params, "key", "")
		if key == "" {
			return nil, errors.New("key is required")
		}
		var value any
		err := rt.KVGet(key, &value)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return map[string]any{"found": false}, nil
		}
		if err != nil {
			return nil, err
		}
		return map[string]any{"found": true, "value": value}, nil
	case "kv.set":
		if err := require("kv:write"); err != nil {
			return nil, err
		}
		key := bridgeString(params, "key", "")
		if key == "" {
			return nil, errors.New("key is required")
		}
		if err := rt.KVSet(key, params["value"]); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	case "kv.delete":
		if err := require("kv:delete"); err != nil {
			return nil, err
		}
		key := bridgeString(params, "key", "")
		if key == "" {
			return nil, errors.New("key is required")
		}
		if err := rt.KVDelete(key); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	case "event.publish":
		if err := require("event:publish"); err != nil {
			return nil, err
		}
		eventType := bridgeString(params, "type", "")
		if eventType == "" {
			return nil, errors.New("type is required")
		}
		rt.Publish(ctx, eventType, bridgeMap(params, "data"))
		return map[string]any{"published": true, "type": eventType}, nil
	case "notice.send":
		if err := require("notice:write"); err != nil {
			return nil, err
		}
		if err := rt.Notice(ctx, NoticeInput{
			Title: bridgeString(params, "title", ""),
			Body:  bridgeString(params, "body", ""),
			Level: bridgeString(params, "level", logInfo),
		}); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	case "sites.list":
		if err := require("site:read"); err != nil {
			return nil, err
		}
		return map[string]any{"sites": rt.SiteList()}, nil
	case "action.invoke":
		// 让插件视图（iframe dashboard）能调用插件声明的 action 拿实时数据，
		// 例如 CloudDrive2 助手面板调 info 取指标。action 在插件进程内执行，无需宿主权限门。
		action := bridgeString(params, "action", "")
		if action == "" {
			return nil, errors.New("action is required")
		}
		input, _ := params["input"].(map[string]any)
		return m.ExecuteAction(ctx, id, action, input)
	case "tool.list":
		// 列出当前插件权限可见的宿主工具（与 externalHostServer GET /tools 同源）。
		reg := m.HostTools()
		if reg == nil {
			return map[string]any{"tools": []HostTool{}}, nil
		}
		return map[string]any{"tools": reg.VisibleTools(manifest.Permissions)}, nil
	case "tool.call":
		// 让插件视图（iframe dashboard）也能调用宿主工具，与 external-http 进程的 POST /tools/{name} 等价。
		// 权限/超时/未找到 的处理统一走 Dispatch；Bridge 层只做参数解包与结果包装。
		reg := m.HostTools()
		name := bridgeString(params, "name", "")
		if reg == nil || name == "" {
			return nil, fmt.Errorf("tool %q not found", name)
		}
		input, _ := params["input"].(map[string]any)
		out, perm, notFound, err := reg.Dispatch(ctx, manifest.Permissions, name, input)
		if notFound {
			return nil, fmt.Errorf("tool %q not found", name)
		}
		if err == ErrPermissionDenied {
			return nil, fmt.Errorf("permission %s is required", perm)
		}
		if err != nil {
			return nil, err
		}
		return map[string]any{"result": out}, nil
	default:
		return nil, fmt.Errorf("unsupported bridge method %q", method)
	}
}

func bridgeString(params map[string]any, key, fallback string) string {
	value := strings.TrimSpace(fmt.Sprint(params[key]))
	if value == "" || value == "<nil>" {
		return fallback
	}
	return value
}

func bridgeMap(params map[string]any, key string) map[string]any {
	if v, ok := params[key].(map[string]any); ok {
		return v
	}
	return nil
}
