// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"github.com/hxpt/ptpatronus/internal/cookiecloud"
)

// genCookieCloudToken 生成 n 位随机字母数字 token（UUID/密码用）。
// 字符集对齐官方扩展风格（去掉易混淆 0O1lI）。
func genCookieCloudToken(n int) string {
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789"
	out := make([]byte, n)
	max := big.NewInt(int64(len(chars)))
	for i := range out {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			// crypto/rand 失败极罕见；退化为时间熵避免阻塞保存
			out[i] = chars[time.Now().UnixNano()%int64(len(chars))]
			continue
		}
		out[i] = chars[idx.Int64()]
	}
	return string(out)
}

// serverStore 返回内置服务端存储（懒建）。
func (c *CookieCloudChain) serverStore() cookiecloud.ServerStore {
	c.srvMu.Lock()
	defer c.srvMu.Unlock()
	if c.srv == nil {
		c.srv = cookiecloud.NewMemServerStore()
	}
	return c.srv
}

// NormalizeMode 导出模式归一化（handler 用）。
func NormalizeMode(mode string) string { return normalizeCookieCloudMode(mode) }

// DecryptedPassword 解密当前配置密码（handler 一次性回传生成密码用）。
func (c *CookieCloudChain) DecryptedPassword(s CookieCloudSettings) (string, error) {
	return c.decryptedPassword(s)
}

// BuiltinEndpointPath 是内置服务端的公开 HTTP 路径前缀（浏览器扩展「服务器」填
// http://<ptp-host>/<此路径>；/get/{uuid}、/update 挂在其下）。
const BuiltinEndpointPath = "/api/v1/cookiecloud"

// BuiltinActive 报告内置服务端当前是否对外服务（mode=builtin 即响应，enabled 只控
// 定时同步——扩展推送是数据入口，不因同步开关关闭而拒绝，便于先推数据后开同步）。
func (c *CookieCloudChain) BuiltinActive() bool {
	return normalizeCookieCloudMode(c.GetSettings().Mode) == CookieCloudModeBuiltin
}

// ServerEnvelope 返回指定 uuid 的存储 envelope；ok=false 表示无数据（404）。
func (c *CookieCloudChain) ServerEnvelope(uuid string) (cookiecloud.ServerEnvelope, bool) {
	return c.serverStore().Get(uuid)
}

// ServerUpdate 处理扩展推送：解析校验 → 存密文 → 触发一次立即拉取同步（推送即同步）。
// 返回 (uuid, plainPassword, error)：plainPassword 非空表示本次保存刚生成了新密码
// （一次性回传前端展示），否则为空串。响应不因同步失败而失败——扩展照旧收 done。
func (c *CookieCloudChain) ServerUpdate(raw []byte) (string, error) {
	req, err := cookiecloud.ParseUpdateRequest(raw)
	if err != nil {
		return "", err
	}
	if !c.BuiltinActive() {
		return "", fmt.Errorf("内置服务端未启用")
	}
	s := c.GetSettings()
	if req.UUID != s.UUID {
		// uuid 不匹配：按不存在处理，不泄露端点状态
		return "", fmt.Errorf("not found")
	}
	// /cookiecloud/update 是公网无鉴权端点，只认 uuid 等于本实例——知道（或猜到）uuid 的人
	// 就能覆盖云端 blob，而紧接着的 triggerSync 会把这份外部输入应用进本地站点 cookie。
	// 写入门禁改成「能不能用本机配置的口令解密」：合法扩展推送本来就和 builtin 共享同一口令，
	// 解不开即视为伪造。失败口径与 uuid 不匹配一致，不给枚举者可分辨的信息。
	password, perr := c.decryptedPassword(s)
	if perr != nil || password == "" {
		slog.Warn("cookiecloud push rejected: server password unavailable", "err", perr)
		return "", fmt.Errorf("not found")
	}
	if verr := cookiecloud.VerifyPushProof(req.Encrypted, req.UUID, password); verr != nil {
		slog.Warn("cookiecloud push rejected: payload not decryptable with configured password")
		return "", fmt.Errorf("not found")
	}
	c.serverStore().Put(req.UUID, cookiecloud.ServerEnvelope{Encrypted: req.Encrypted, CryptoType: req.CryptoType})
	c.triggerSync()
	return req.UUID, nil
}

// triggerSync 异步触发一次立即同步（推送即同步）。并发/频率失控风险小：
// 扩展推送有本地 24h 去重（内容不变不重推），正常频率远低于定时 tick。
func (c *CookieCloudChain) triggerSync() {
	s := c.GetSettings()
	if !s.Enabled {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Warn("cookiecloud push-triggered sync panic", "recover", r)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		report, err := c.doSync(ctx, s)
		c.recordSyncResult(s, report, err)
	}()
}
