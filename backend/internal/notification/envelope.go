// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package notification

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/hxpt/ptpatronus/internal/errtext"
)

// decodeEnvelope 解析渠道 HTTP 200 响应里的业务码。两个坑都要挡：
//   - 空响应体：没有业务码可判，「没读回来」不能算「已送达」；
//   - 不是 JSON：反向代理/网关/运营商塞回一个 200 的 HTML 拦截页时，从前各渠道写的是
//     `json.Unmarshal(raw, &be) == nil && 码不对` —— 解析失败短路成「判成功」，
//     于是面板显示已送达、失败重试也不触发（同 #211/#215/#216 的判据形状）。
//     渠道存在的全部意义是「出事的时候能说话」，判不出来就必须算失败并带上原文。
func decodeEnvelope(channel string, raw []byte, v any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return fmt.Errorf("%s: 上游只回了空响应体，无法确认消息是否送达（多半被反向代理或网关拦下）", channel)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("%s: 响应不是合法 JSON，无法确认消息是否送达（原文：%s）", channel, errtext.RespSnippet(raw))
	}
	return nil
}
