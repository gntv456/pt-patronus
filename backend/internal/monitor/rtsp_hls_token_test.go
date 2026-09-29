package monitor

import (
	"math"
	"strings"
	"testing"

	"github.com/gosnmp/gosnmp"
)

// HLS 段 URL 必须携带 token：相对段 URL 按 RFC 3986 解析会丢弃 playlist 的 query，
// 标准播放器逐段请求若无 token 会被 authHLS 拒绝（401），HLS 链路断裂。
func TestRewriteSegmentToken(t *testing.T) {
	pl := "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:2\n#EXT-X-MEDIA-SEQUENCE:0\n" +
		"#EXTINF:1.000,\nsegment/0.ts\n#EXTINF:1.000,\nsegment/1.ts\n"
	out := rewriteSegmentToken(pl, "tok.en-1_2")
	if !strings.Contains(out, "segment/0.ts?token=tok.en-1_2\n") ||
		!strings.Contains(out, "segment/1.ts?token=tok.en-1_2\n") {
		t.Fatalf("segment URLs missing token:\n%s", out)
	}
	if c := strings.Count(out, "?token="); c != 2 {
		t.Fatalf("expected exactly 2 tokenized URLs, got %d:\n%s", c, out)
	}
	// 标签行不得被改写
	if strings.Contains(out, "TARGETDURATION?token") || strings.Contains(out, "EXTINF?token") {
		t.Fatalf("tag lines must not be rewritten:\n%s", out)
	}
}

// buildHRStorage 防 size*units int64 溢出 + used 钳制（nas_audit P2-003）。
func TestBuildHRStorageOverflowGuard(t *testing.T) {
	pdu := func(col, idx string, v any) gosnmp.SnmpPDU {
		return gosnmp.SnmpPDU{Name: ".1.3.6.1.2.1.25.2.3.1." + col + "." + idx, Type: gosnmp.Integer, Value: v}
	}
	// 行 1：size*units 必然溢出（size > MaxInt64/units）→ 整行跳过，不得产出负数/巨大 total
	// 行 2：used > size（异常设备）→ used 钳到 size，usage 封顶 100%
	entries := []gosnmp.SnmpPDU{
		pdu("3", "1", 1024),
		pdu("4", "1", math.MaxInt64/1000), // > MaxInt64/1024
		pdu("5", "1", 1),
		pdu("2", "2", "/dev/sda1"),
		pdu("3", "2", 1),
		pdu("4", "2", 100),
		pdu("5", "2", 500),
	}
	disks := buildHRStorage(entries)
	if len(disks) != 1 {
		t.Fatalf("expected 1 disk (overflow row skipped), got %d: %+v", len(disks), disks)
	}
	d := disks[0]
	if d.Total != 100 || d.Used != 100 {
		t.Fatalf("clamp failed: total=%d used=%d, want 100/100", d.Total, d.Used)
	}
	if d.Usage > 100.0001 {
		t.Fatalf("usage must be capped at 100%%, got %f", d.Usage)
	}
}
