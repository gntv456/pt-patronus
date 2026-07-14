package app

import (
	"testing"

	"github.com/hxpt/ptpatronus/internal/mcp"
)

// TestClassifyToolRisks 验证 read/write 分类 + destructive 显式标注不被覆盖 + 兜底为 write。
func TestClassifyToolRisks(t *testing.T) {
	tools := []mcp.Tool{
		{Name: "torrents_delete", Risk: mcp.RiskDestructive, RiskNote: "删种子"}, // 显式 destructive 须保留
		{Name: "sites.list"},       // read
		{Name: "system.info"},      // read
		{Name: "subscribe_create"}, // write（不在 read 表）
		{Name: "totally_new_tool"}, // 未在 read 表 → 兜底 write
		{Name: ""},                 // 无名工具也不崩，兜底 write
	}
	classifyToolRisks(tools)

	if tools[0].Risk != mcp.RiskDestructive || tools[0].RiskNote != "删种子" {
		t.Errorf("destructive 显式标注被覆盖: risk=%q note=%q", tools[0].Risk, tools[0].RiskNote)
	}
	if tools[1].Risk != mcp.RiskRead {
		t.Errorf("sites.list 应为 read, got %q", tools[1].Risk)
	}
	if tools[2].Risk != mcp.RiskRead {
		t.Errorf("system.info 应为 read, got %q", tools[2].Risk)
	}
	if tools[3].Risk != mcp.RiskWrite {
		t.Errorf("subscribe_create 应为 write, got %q", tools[3].Risk)
	}
	if tools[4].Risk != mcp.RiskWrite {
		t.Errorf("未知工具应兜底为 write, got %q", tools[4].Risk)
	}
	if tools[5].Risk != mcp.RiskWrite {
		t.Errorf("无名工具应兜底为 write, got %q", tools[5].Risk)
	}
}
