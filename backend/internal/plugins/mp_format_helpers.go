package plugins

import (
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
)

func mpSHA1Hex(s string) string {
	sum := sha1.Sum([]byte(s))
	return fmt.Sprintf("%x", sum[:])
}

func mpSHA256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", sum[:])
}

func mpXiaomiProtoInt(v any) int {
	switch strings.ToLower(strings.TrimSpace(pluginString(v))) {
	case "1", "tcp":
		return 1
	case "2", "udp":
		return 2
	default:
		return 3
	}
}

func mpXiaomiProtoText(v int) string {
	switch v {
	case 1:
		return "tcp"
	case 2:
		return "udp"
	default:
		return "both"
	}
}

func mpFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case string:
		f, _ := strconvParseFloat(x)
		return f
	default:
		return 0
	}
}

func strconvParseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscan(strings.TrimSpace(s), &f)
	return f, err
}

func mpFormatBytes(v float64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	return fmt.Sprintf("%.2f %s", v, units[i])
}

func mpFormatSpeed(v float64) string {
	return mpFormatBytes(v) + "/s"
}

func mpUptime(seconds int) string {
	if seconds <= 0 {
		return "-"
	}
	days := seconds / 86400
	hours := (seconds % 86400) / 3600
	mins := (seconds % 3600) / 60
	return fmt.Sprintf("%d天 %d小时 %d分钟", days, hours, mins)
}

func mpXiaomiStatusText(status map[string]any) string {
	return fmt.Sprintf("路由器：%s\n在线设备：%v\n实时下行：%s\n实时上行：%s\n本期下载：%s\n本期上传：%s\n运行：%s",
		status["router_name"], status["online_count"], status["downspeed_human"], status["upspeed_human"], status["download_human"], status["upload_human"], status["uptime_human"])
}

func mpMbps(bytesLen int, d time.Duration) float64 {
	if d <= 0 {
		return 0
	}
	return float64(bytesLen) * 8 / d.Seconds() / 1000000
}

func mpSpeedtestText(out map[string]any) string {
	lines := []string{fmt.Sprintf("下行：%.2f Mbps", out["download_mbps"])}
	if v, ok := out["upload_mbps"]; ok {
		lines = append(lines, fmt.Sprintf("上行：%.2f Mbps", v))
	}
	return strings.Join(lines, "\n")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
