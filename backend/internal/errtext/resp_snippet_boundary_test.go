package errtext

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestRespSnippetEndsOnRuneBoundary 160 字节的钳位必须落在 rune 边界上。
// 站点/下载器回的是 UTF-8 中文，正好切在一个汉字的第 2 或第 3 个字节上时，
// 半个 rune 进 JSON 会被编码成 U+FFFD：用户看到的不是「原文到此为止」，
// 而是尾巴上一个乱码方块，且它和被截断的真内容长得一样。
func TestRespSnippetEndsOnRuneBoundary(t *testing.T) {
	body := strings.Repeat("a", 60) + strings.Repeat("字幕", 60) // 60 ASCII + 每 6 字节两字，第 160 字节落在汉字中间
	if len(body) <= 160 {
		t.Fatalf("用例前提不成立：原文没真的超长（len=%d）", len(body))
	}
	out := RespSnippet([]byte(body))
	if !utf8.ValidString(out) {
		t.Errorf("截断结果不是合法 UTF-8（界面会出现乱码方块），tail=%q", []byte(out)[len(out)-6:])
	}
	if !strings.HasSuffix(out, "…") {
		t.Errorf("截断要留省略号，got %q", out)
	}
	if utf8.RuneCountInString(out) < 60 {
		t.Errorf("对齐边界不该吃掉大半原文，rune 数=%d", utf8.RuneCountInString(out))
	}
}

// TestRespSnippetShortInputUnchanged 未超长必须原样返回（反向对照，防我把它改成无脑截断）。
func TestRespSnippetShortInputUnchanged(t *testing.T) {
	if got := RespSnippet([]byte("短原文")); got != "短原文" {
		t.Errorf("got %q", got)
	}
}

// TestRespSnippetAllContinuationBytesKeepsContent 开头全是不成句的续字节（二进制垃圾响应）时
// 不能因为「找不到合法边界」把线索清空 —— 宁可回原样的乱码，也不能回空串。
func TestRespSnippetAllContinuationBytesKeepsContent(t *testing.T) {
	junk := make([]byte, 0, 200)
	for i := 0; i < 200; i++ {
		junk = append(junk, 0x80)
	}
	out := RespSnippet(junk)
	if out == "" || !strings.HasSuffix(out, "…") {
		t.Errorf("垃圾输入也要给出一段可读的截断标记，got %q", out)
	}
}
