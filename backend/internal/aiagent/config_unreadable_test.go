package aiagent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/llm"
)

// failKV 模拟「记录存在但读不出来」。底层 SystemConfigOper.Get 已经把「键不存在」归一成 ("", nil)，
// 所以非 nil 的错误一定是真故障——这条区分必须由用例钉住，否则后来人又会把它当成「还没配过」。
type failKV struct{ written int }

func (k *failKV) Get(key string) (string, error) { return "", errors.New("database is locked") }
func (k *failKV) Set(key, value string) error    { k.written++; return nil }

// TestLoadKeepsUnreadableApartFromUnconfigured #271：读失败与坏记录都必须出声，
// 「真的没配过」才是回默认值且不报错的那一档。
func TestLoadKeepsUnreadableApartFromUnconfigured(t *testing.T) {
	crypto := security.NewCrypto("master")

	// 1) KV 读失败
	ac, _, err := Load(&failKV{}, crypto)
	if !errors.Is(err, ErrConfigUnreadable) {
		t.Fatalf("KV 读失败必须带回 ErrConfigUnreadable，got %v", err)
	}
	if ac.Enabled {
		t.Error("读失败时仍应退到默认值供调用方安全使用")
	}

	// 2) 记录被写坏（合法字符串但不是这份配置的形状）
	broken := &fakeKV{m: map[string]string{configKey: "{\"enabled\": true, \"max_iterations\": \"不是数字\"}"}}
	if _, _, err := Load(broken, crypto); !errors.Is(err, ErrConfigUnreadable) {
		t.Errorf("解析失败必须出声，got %v", err)
	}

	// 3) 正控：真的没配过 ⇒ 无错误（这条若红，就是每个新装用户都会收到 503）
	if _, _, err := Load(&fakeKV{m: map[string]string{}}, crypto); err != nil {
		t.Errorf("没有记录应视为未配置而非故障，got %v", err)
	}

	// 4) 正控：健康的配置不会被误判成坏了
	raw, err := json.Marshal(Config{Enabled: true, Provider: "openai", Model: "m", SystemPrompt: "p", MaxIterations: 4})
	if err != nil {
		t.Fatal(err)
	}
	ac, _, err = Load(&fakeKV{m: map[string]string{configKey: string(raw)}}, crypto)
	if err != nil || !ac.Enabled || ac.MaxIterations != 4 {
		t.Errorf("健康配置被误判：err=%v ac=%+v", err, ac)
	}
}

// TestReadyConfigUnreadableIsNotDisabled 旧行为：读失败 ⇒ 默认值 Enabled=false ⇒ 判成「未启用」，
// 界面劝用户去「设置里启用」，而它本来就是启用的。
func TestReadyConfigUnreadableIsNotDisabled(t *testing.T) {
	agent := New("test", nil, nil, nil, nil, &failKV{}, security.NewCrypto("master"))
	if got := agent.Ready(); got != ReadyConfigUnreadable {
		t.Fatalf("读不出配置要判成 ReadyConfigUnreadable，got %v", got)
	}
	// 正控：没配过仍然是「未启用」（新装实例不该看到一句故障）
	fresh := New("test", nil, nil, nil, nil, &fakeKV{m: map[string]string{}}, security.NewCrypto("master"))
	if got := fresh.Ready(); got != ReadyDisabled {
		t.Errorf("真的没配过应仍是 ReadyDisabled，got %v", got)
	}
}

// TestRunReportsUnreadableConfigInsteadOfNotEnabled 对话出口：同一件事在流里也不能说成「未启用」。
func TestRunReportsUnreadableConfigInsteadOfNotEnabled(t *testing.T) {
	agent := New("test", nil, nil, nil, nil, &failKV{}, security.NewCrypto("master"))
	var texts []string
	for ev := range agent.Run(context.Background(), 1, "你好") {
		if ev.Type == llm.EventError {
			texts = append(texts, ev.Error)
		}
	}
	if len(texts) != 1 {
		t.Fatalf("应恰好一条错误事件，got %v", texts)
	}
	if !strings.Contains(texts[0], "读不出来") {
		t.Errorf("要说清是配置读不出来：%q", texts[0])
	}
	// 旧那句是缺陷本体；只判「没出现那句」，不误伤「这不是未启用」这类否定句。
	if strings.Contains(texts[0], "未启用，请先在") {
		t.Errorf("不能把读故障渲染成用户的开关状态：%q", texts[0])
	}
	if n := strings.Count(texts[0], ErrConfigUnreadable.Error()); n != 1 {
		t.Errorf("哨兵前缀出现 %d 次（叠两次界面就是「读不出来（读不出来: …）」）：%q", n, texts[0])
	}
	// 失败发生在写库之前：不该留下一条「用户消息」却没有回应（会话列表攒垃圾条目）。
	if kv, ok := agent.kv.(*failKV); ok && kv.written > 0 {
		t.Errorf("配置读不出时不应写任何东西，written=%d", kv.written)
	}
}
