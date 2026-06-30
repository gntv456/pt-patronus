package monitor

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// collectTrueNAS 用 API Key（creds.Password 作 Token）取系统信息与存储池。
func collectTrueNAS(ctx context.Context, hc *httpclient.Client, t *models.MonitorTarget, creds Credentials) (Metrics, error) {
	base := normalizeBase(t.Host)
	if base == "" {
		return Metrics{}, fmt.Errorf("nas host is required")
	}
	hdr := map[string]string{"Authorization": "Token " + creds.Password}
	infoRaw, err := tnGet(ctx, hc, base+"/api/v2.0/system/info", hdr)
	if err != nil {
		return Metrics{}, fmt.Errorf("truenas info: %w", err)
	}
	m := Metrics{Online: true}
	var info map[string]any
	_ = json.Unmarshal(infoRaw, &info)
	m.OS = "TrueNAS " + firstStr(info, "version")
	m.Hostname = firstStr(info, "hostname")
	if v := numUint(info["uptime_seconds"]); v > 0 {
		m.Uptime = v
	}
	// 存储池
	if poolRaw, err := tnGet(ctx, hc, base+"/api/v2.0/pool", hdr); err == nil && len(poolRaw) > 0 {
		var pools []map[string]any
		if json.Unmarshal(poolRaw, &pools) == nil {
			for _, p := range pools {
				d := DiskMetric{Type: "pool"}
				d.Device = firstStr(p, "name")
				d.Total = numUint(p["size"])
				d.Used = numUint(p["allocated"])
				if d.Total > 0 {
					d.Usage = float64(d.Used) / float64(d.Total) * 100
				}
				m.Disks = append(m.Disks, d)
			}
		}
	}
	return m, nil
}

// tnGet 带 Token 鉴权 GET（ssrf=false：内网）。
func tnGet(ctx context.Context, hc *httpclient.Client, u string, hdr map[string]string) (json.RawMessage, error) {
	resp, err := hc.GetWithHeaders(ctx, u, "", hdr, false)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(resp.Body), nil
}
