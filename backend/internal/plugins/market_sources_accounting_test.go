package plugins

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/event"
)

// 配了多个市场源时，聚合面必须把「哪个源没取到」说清楚，并按各源自己的公钥判信任等级。
// 两处曾经同形：① 单源失败只 slog.Warn 后跳过，用户看到的是「仓库里就这些插件」；
// ② 聚合索引本身不带任何源的 public_key，finalizeIndex 拿空串比 → 签名源也被打成「未签名」，
// 而这是用户在市场列表里唯一能看到的来源可信信号。
func TestFetchMarketMultiReportsSourceFailuresAndTrust(t *testing.T) {
	root := t.TempDir()
	goodDir := filepath.Join(root, "good")
	if err := os.MkdirAll(goodDir, 0o755); err != nil {
		t.Fatal(err)
	}

	item := baseMarketItem()
	item.SHA256 = "aaa"
	item.Signature = "z3N=" // 源有公钥 + 条目有签名 → 逐源盖章应得到 signed 而不是 unsigned
	goodIdx := MarketIndex{
		Version:   1,
		PublicKey: "W3Zob2V5IHB1YmxpYyBrZXkgZm9yIHRlc3Rpbmcp==", // 非空即视为该源已签名发布
		Plugins:   []MarketPlugin{item},
	}
	raw, err := json.Marshal(goodIdx)
	if err != nil {
		t.Fatal(err)
	}
	goodPath := filepath.Join(goodDir, "plugin-market.json")
	if err := os.WriteFile(goodPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	// 坏源：文件存在但不是合法 JSON（避开「本地默认源文件不存在=未配置」的空市场捷径）。
	badPath := filepath.Join(root, "broken-market.json")
	if err := os.WriteFile(badPath, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil, nil, nil)
	m.SetPluginRoot(filepath.Join(root, "plugins"))

	idx, err := m.FetchMarketMulti(context.Background(), []string{goodPath, badPath})
	if err != nil {
		t.Fatalf("一个源坏、一个源好时应返回聚合结果而不是报错：%v", err)
	}
	if len(idx.Plugins) != 1 {
		t.Fatalf("聚合条目数 = %d，期望 1（%v）", len(idx.Plugins), idx.Plugins)
	}
	if idx.SourcesOK != 1 {
		t.Errorf("SourcesOK = %d，期望 1", idx.SourcesOK)
	}
	if len(idx.SourcesFailed) != 1 {
		t.Fatalf("坏源必须进账面，got %+v", idx.SourcesFailed)
	}
	if idx.SourcesFailed[0].Source != badPath || idx.SourcesFailed[0].Error == "" {
		t.Errorf("失败源记录不完整：%+v", idx.SourcesFailed[0])
	}
	// 关键：带公钥的源里的插件不能被说成未签名。
	if got := idx.Plugins[0].TrustLevel; got == "" || got == "unsigned" {
		t.Errorf("TrustLevel = %q，签名源的条目不该是空或 unsigned", got)
	}

	// 反证：全源皆坏时不能假装「空市场」，要报错（否则用户以为仓库被清空了）。
	if _, err := m.FetchMarketMulti(context.Background(), []string{badPath}); err == nil {
		t.Error("全部源都取不到时必须报错，不能回空索引")
	}
}
