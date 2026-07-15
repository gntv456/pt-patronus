package v1

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

// SystemHandler.Metrics：即时采集 gopsutil，返回本机资源快照。
// 关键断言：online 恒为 true；disks 经 nil→空数组兜底必为数组（防前端 JSON null，记忆 [[go-nil-slice-json-null]]）。
func TestSystemMetrics(t *testing.T) {
	h := NewSystemHandler(nil) // Metrics 不依赖 update
	r := gin.New()
	r.GET("/metrics", h.Metrics)

	rec := doReq(t, r, http.MethodGet, "/metrics", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	m := bodyJSON(t, rec)

	if m["online"] != true {
		t.Errorf("online = %v, want true", m["online"])
	}
	// mem_total 无 omitempty 之外的平台差异：内存几乎总有值，断言其存在且为数值。
	if _, ok := m["mem_total"].(float64); !ok {
		t.Errorf("mem_total = %T (%v), want number", m["mem_total"], m["mem_total"])
	}
	// disks/temps 带 omitempty：有数据时必为数组类型，无数据（如 Windows 无温度传感器）则省略。
	// 真正的 null→[] 兜底在前端 ?? []，此处只校验「存在即为数组」。
	if disks, exists := m["disks"]; exists {
		if _, ok := disks.([]any); !ok {
			t.Errorf("disks = %T, want array when present", disks)
		}
	}
}
