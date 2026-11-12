// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type twoFAHelperPlugin struct{}

func NewTwoFAHelperPlugin() Plugin { return twoFAHelperPlugin{} }

func (twoFAHelperPlugin) Manifest() Manifest {
	return Manifest{
		ID:          "twofa-helper",
		Name:        "两步验证助手",
		Version:     "1.2.7",
		Author:      "madrays / PTPatronus",
		Description: "管理 TOTP 密钥并生成当前 6 位验证码。",
		Runtime:     RuntimeBuiltin,
		Builtin:     true,
		Permissions: []string{"action:execute"},
		Actions: []ActionSpec{
			{
				Name:        "generate_codes",
				Label:       "生成验证码",
				Description: "按配置或输入的账号列表生成当前 TOTP 验证码。",
				InputSchema: []ConfigField{
					{Key: "accounts", Label: "账号列表", Type: "textarea", Help: "每行：名称|Base32密钥|URL，可留空使用插件配置。"},
				},
			},
			{
				Name:        "generate_one",
				Label:       "单个验证码",
				Description: "输入一个 Base32 密钥生成当前 TOTP 验证码。",
				InputSchema: []ConfigField{
					{Key: "name", Label: "名称", Type: "text", Default: "Demo"},
					{Key: "secret", Label: "Base32密钥", Type: "password", Required: true},
				},
			},
			{
				Name:        "otpauth_uri",
				Label:       "生成 otpauth URI",
				Description: "生成可导入认证器的 otpauth://totp URI。",
				InputSchema: []ConfigField{
					{Key: "issuer", Label: "签发方", Type: "text", Default: "PTPatronus"},
					{Key: "name", Label: "账号", Type: "text", Required: true},
					{Key: "secret", Label: "Base32密钥", Type: "password", Required: true},
				},
			},
		},
		ConfigSchema: []ConfigField{
			{Key: "accounts", Label: "账号列表", Type: "textarea", Group: "账号", Help: "每行：名称|Base32密钥|URL。配置会随插件配置保存，请只在可信环境使用。"},
			{Key: "digits", Label: "验证码位数", Type: "number", Group: "参数", Default: 6},
			{Key: "period_seconds", Label: "刷新周期(秒)", Type: "number", Group: "参数", Default: 30},
		},
	}
}

func (twoFAHelperPlugin) DefaultConfig() map[string]any {
	return map[string]any{"accounts": "", "digits": 6, "period_seconds": 30}
}

func (twoFAHelperPlugin) HandleEvent(context.Context, Runtime, Event) error { return nil }

func (twoFAHelperPlugin) ExecuteAction(ctx context.Context, rt Runtime, action string, input map[string]any) (map[string]any, error) {
	_ = ctx
	cfg := rt.Config()
	digits := pluginInt(cfg["digits"], 6)
	period := pluginInt(cfg["period_seconds"], 30)
	switch action {
	case "generate_codes":
		raw := strings.TrimSpace(pluginString(input["accounts"]))
		if raw == "" {
			raw = pluginString(cfg["accounts"])
		}
		accounts, err := plParseTOTPAccounts(raw)
		if err != nil {
			return nil, err
		}
		codes := make([]map[string]any, 0, len(accounts))
		for _, account := range accounts {
			code, remain, err := plTOTPCode(account.Secret, time.Now(), digits, period)
			if err != nil {
				codes = append(codes, map[string]any{"name": account.Name, "ok": false, "error": err.Error()})
				continue
			}
			codes = append(codes, map[string]any{
				"name":              account.Name,
				"code":              code,
				"url":               account.URL,
				"remaining_seconds": remain,
				"period_seconds":    period,
				"ok":                true,
			})
		}
		rt.Log(logInfo, action, "totp codes generated", map[string]any{"count": len(codes)})
		return map[string]any{"codes": codes}, nil
	case "generate_one":
		code, remain, err := plTOTPCode(pluginString(input["secret"]), time.Now(), digits, period)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(pluginString(input["name"]))
		if name == "" {
			name = "TOTP"
		}
		rt.Log(logInfo, action, "single totp code generated", map[string]any{"name": name})
		return map[string]any{"name": name, "code": code, "remaining_seconds": remain, "period_seconds": period}, nil
	case "otpauth_uri":
		issuer := strings.TrimSpace(pluginString(input["issuer"]))
		name := strings.TrimSpace(pluginString(input["name"]))
		secret := plCleanTOTPSecret(pluginString(input["secret"]))
		if name == "" || secret == "" {
			return nil, errors.New("账号和密钥不能为空")
		}
		u := url.URL{Scheme: "otpauth", Host: "totp", Path: "/" + issuer + ":" + name}
		q := u.Query()
		q.Set("secret", secret)
		q.Set("issuer", issuer)
		q.Set("digits", fmt.Sprint(digits))
		q.Set("period", fmt.Sprint(period))
		u.RawQuery = q.Encode()
		return map[string]any{"uri": u.String()}, nil
	default:
		return nil, unknownActionErr(action)
	}
}
