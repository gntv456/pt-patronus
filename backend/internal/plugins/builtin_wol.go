package plugins

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"strings"
)

// wolPlugin — 远程 WOL 唤醒（对标 MSaber 网络工具「远程 WOL 唤醒」）。
// 发送 UDP 魔术包（6×0xFF + 16×MAC）到目标广播地址:9（或配置端口）。
type wolPlugin struct{}

func NewWOLPlugin() Plugin { return wolPlugin{} }

func (wolPlugin) Manifest() Manifest {
	return Manifest{
		ID:          "wol-wake",
		Name:        "远程 WOL 唤醒",
		Version:     "1.0.0",
		Author:      "PTPatronus",
		Description: "向局域网设备发送 Wake-on-LAN 魔术包远程唤醒（NAS/主机开机）。",
		Runtime:     RuntimeBuiltin,
		Builtin:     true,
		Permissions: []string{"network:udp", "action:execute"},
		Actions: []ActionSpec{
			{
				Name:        "wake",
				Label:       "唤醒设备",
				Description: "发送魔术包唤醒指定 MAC 地址的设备。",
				InputSchema: []ConfigField{
					{Key: "mac", Label: "MAC 地址", Type: "text", Required: true, Help: "如 AA:BB:CC:DD:EE:FF"},
					{Key: "broadcast", Label: "广播地址", Type: "text", Default: "255.255.255.255"},
					{Key: "port", Label: "端口", Type: "number", Default: 9},
				},
			},
		},
		ConfigSchema: []ConfigField{
			{Key: "targets", Label: "常用设备", Type: "textarea",
				Help: "每行一个：名称,MAC,广播地址,端口（如 nas,AA:BB:CC:DD:EE:FF,192.168.1.255,9）。action 留空 mac 时按名称取。"},
			{Key: "default_port", Label: "默认端口", Type: "number", Default: 9},
		},
	}
}

func (wolPlugin) DefaultConfig() map[string]any {
	return map[string]any{"default_port": 9}
}

func (wolPlugin) HandleEvent(context.Context, Runtime, Event) error { return nil }

// parseMAC 规范化 MAC（接受 : - 空格分隔与裸十六进制）。
func parseMAC(s string) (net.HardwareAddr, error) {
	s = strings.TrimSpace(s)
	s = strings.NewReplacer(":", "", "-", "", " ", "", ".", "").Replace(s)
	if len(s) != 12 {
		return nil, fmt.Errorf("MAC 格式错误: %q", s)
	}
	raw, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("MAC 十六进制错误: %v", err)
	}
	return net.HardwareAddr(raw), nil
}

// wolSend 构造并发送魔术包（6×0xFF + 16×MAC）到 udp broadcast:port。
func wolSend(mac net.HardwareAddr, broadcast string, port int) error {
	packet := make([]byte, 6+16*len(mac))
	for i := 0; i < 6; i++ {
		packet[i] = 0xFF
	}
	for i := 0; i < 16; i++ {
		copy(packet[6+i*len(mac):], mac)
	}
	conn, err := net.Dial("udp", net.JoinHostPort(broadcast, fmt.Sprintf("%d", port)))
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.Write(packet); err != nil {
		return err
	}
	return nil
}

func (wolPlugin) ExecuteAction(ctx context.Context, rt Runtime, action string, input map[string]any) (map[string]any, error) {
	if action != "wake" {
		return nil, unknownActionErr(action)
	}
	cfg := rt.Config()
	macStr := pluginString(input["mac"])
	broadcast := pluginString(input["broadcast"])
	port := pluginInt(input["port"], pluginInt(cfg["default_port"], 9))
	// 名称取 targets（mac 为空时按名称查找）
	if macStr == "" {
		name := pluginString(input["name"])
		if name == "" {
			return nil, fmt.Errorf("mac 或 name 必填")
		}
		for _, line := range pluginSplitList(pluginAnyString(cfg["targets"])) {
			parts := strings.Split(line, ",")
			if len(parts) >= 2 && strings.TrimSpace(parts[0]) == name {
				macStr = strings.TrimSpace(parts[1])
				if broadcast == "" && len(parts) >= 3 {
					broadcast = strings.TrimSpace(parts[2])
				}
				if len(parts) >= 4 {
					port = pluginInt(parts[3], port)
				}
				break
			}
		}
		if macStr == "" {
			return nil, fmt.Errorf("targets 中未找到设备: %s", name)
		}
	}
	if broadcast == "" {
		broadcast = "255.255.255.255"
	}
	mac, err := parseMAC(macStr)
	if err != nil {
		return nil, err
	}
	if err := wolSend(mac, broadcast, port); err != nil {
		rt.Log(logError, "wake", "wol send failed", map[string]any{"err": err.Error()})
		return nil, err
	}
	rt.Log(logInfo, "wake", "wol sent", map[string]any{"mac": macStr, "to": broadcast, "port": port})
	return map[string]any{"ok": true, "mac": macStr, "broadcast": broadcast, "port": port}, nil
}
