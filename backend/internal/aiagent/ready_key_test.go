// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package aiagent

import (
	"encoding/json"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/security"
)

func kvWith(t *testing.T, keyEnc string) *fakeKV {
	t.Helper()
	raw, err := json.Marshal(Config{Enabled: true, Provider: "openai", APIKeyEnc: keyEnc})
	if err != nil {
		t.Fatal(err)
	}
	return &fakeKV{m: map[string]string{configKey: string(raw)}}
}

// TestReadySplitsMissingKeyFromUnreadable 回归：Load 过去把「解密失败」咽掉、只留空 APIKey，
// 于是主密钥轮换（或备份恢复到新实例）之后，库里明明还有 API Key 的用户被告知「未配置 API key」，
// 被引去重填一个其实还在的值。三档必须分开。
func TestReadySplitsMissingKeyFromUnreadable(t *testing.T) {
	enc, err := security.NewCrypto("old-master-key").Encrypt("sk-abcdef")
	if err != nil {
		t.Fatal(err)
	}
	kv := kvWith(t, enc)

	// 1) 换过主密钥：解不开 ≠ 没填
	rotated := New("test", nil, nil, nil, nil, kv, security.NewCrypto("rotated-master-key"))
	if got := rotated.Ready(); got != ReadyKeyUnreadable {
		t.Fatalf("密钥解不开要判成 ReadyKeyUnreadable，got %v", got)
	}
	if _, _, keyErr := Load(kv, security.NewCrypto("rotated-master-key")); keyErr == nil {
		t.Error("Load 必须把解不开的原因交回调用方")
	}

	// 2) 同一把主密钥：照常可用（别把健康配置说成坏了）
	same := New("test", nil, nil, nil, nil, kv, security.NewCrypto("old-master-key"))
	if got := same.Ready(); got != ReadyOK {
		t.Fatalf("主密钥没换过应可对话，got %v", got)
	}

	// 3) 真的没填：仍报「未配置」，不能被新判据吞成「解不开」
	noKey := New("test", nil, nil, nil, nil, kvWith(t, ""), security.NewCrypto("old-master-key"))
	if got := noKey.Ready(); got != ReadyNoKey {
		t.Fatalf("没填 key 仍是 ReadyNoKey，got %v", got)
	}

	// 4) 未启用优先于一切（开关关着时不该谈密钥）
	offRaw, err := json.Marshal(Config{Enabled: false, APIKeyEnc: "whatever"})
	if err != nil {
		t.Fatal(err)
	}
	offKV := &fakeKV{m: map[string]string{configKey: string(offRaw)}}
	if got := New("test", nil, nil, nil, nil, offKV, security.NewCrypto("old-master-key")).Ready(); got != ReadyDisabled {
		t.Errorf("Enabled=false 应优先判未启用，got %v", got)
	}
}
