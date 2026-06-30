package monitor

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gosnmp/gosnmp"
	"github.com/hxpt/ptpatronus/internal/db/models"
)

// collectSNMP 用 SNMP v2c 采集负载/内存/存储（NAS/路由器通用兜底；creds.Username 作 community）。
// CPU% 无标准通用 OID，用负载近似（hrProcessorLoad 多数设备不实现）。
func collectSNMP(_ context.Context, t *models.MonitorTarget, creds Credentials) (Metrics, error) {
	host := snmpHost(t)
	if host == "" {
		return Metrics{}, fmt.Errorf("snmp host is required")
	}
	port := uint16(t.Port)
	if port == 0 {
		port = 161
	}
	params := &gosnmp.GoSNMP{
		Target:    host,
		Port:      port,
		Community: creds.Username, // SNMP community 放 username 字段
		Version:   gosnmp.Version2c,
		Timeout:   nasTimeout(t),
		MaxOids:   30,
	}
	if err := params.Connect(); err != nil {
		return Metrics{}, fmt.Errorf("snmp connect: %w", err)
	}
	defer params.Conn.Close()

	m := Metrics{Online: true}
	// 标量：负载 / 内存 / 系统描述 / 运行时长
	if res, err := params.Get([]string{
		".1.3.6.1.4.1.2021.10.1.3.1", // laLoad.1（1 分钟）
		".1.3.6.1.4.1.2021.10.1.3.2", // laLoad.2（5 分钟）
		".1.3.6.1.4.1.2021.10.1.3.3", // laLoad.3（15 分钟）
		".1.3.6.1.4.1.2021.4.5.0",    // memTotalReal（kB）
		".1.3.6.1.4.1.2021.4.6.0",    // memAvailReal（kB）
		".1.3.6.1.2.1.1.1.0",         // sysDescr
		".1.3.6.1.2.1.1.3.0",         // sysUpTime（百分秒）
	}); err == nil && len(res.Variables) >= 7 {
		m.Load1 = numFloat(snmpValue(res.Variables[0].Value))
		m.Load5 = numFloat(snmpValue(res.Variables[1].Value))
		m.Load15 = numFloat(snmpValue(res.Variables[2].Value))
		total := snmpInt(res.Variables[3].Value) * 1024
		avail := snmpInt(res.Variables[4].Value) * 1024
		m.MemTotal = uint64(total)
		m.MemUsed = uint64(total - avail)
		if total > 0 {
			m.MemUsage = float64(total-avail) / float64(total) * 100
		}
		if s, ok := res.Variables[5].Value.(string); ok {
			m.OS = s
		}
		// sysUpTime 单位百分秒（1/100s）
		m.Uptime = uint64(snmpInt(res.Variables[6].Value) / 100)
	}

	// 存储表 hrStorageEntry：按 index 关联 descr/units/size/used
	if entries, err := params.BulkWalkAll(".1.3.6.1.2.1.25.2.3.1"); err == nil {
		for _, d := range buildHRStorage(entries) {
			m.Disks = append(m.Disks, d)
		}
	}
	return m, nil
}

// snmpHost 从 t.Host 去 scheme/端口/path，取纯主机名/IP。
func snmpHost(t *models.MonitorTarget) string {
	h := strings.TrimSpace(t.Host)
	if i := strings.Index(h, "://"); i >= 0 {
		h = h[i+3:]
	}
	if i := strings.IndexByte(h, '/'); i >= 0 {
		h = h[:i]
	}
	if i := strings.LastIndexByte(h, ':'); i >= 0 {
		h = h[:i] // 去端口（SNMP 设备罕见 IPv6）
	}
	return h
}

// snmpInt 宽松转 int64（gosnmp 整数类型不定：int/int64/uint/string）。
func snmpInt(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int64:
		return n
	case uint:
		return int64(n)
	case uint64:
		return int64(n)
	case string:
		i, _ := strconv.ParseInt(n, 10, 64)
		return i
	}
	return 0
}

// snmpValue 转 float64（负载等）。
func snmpValue(v any) float64 { return float64(snmpInt(v)) }

type hrRow struct {
	descr string
	units int64
	size  int64
	used  int64
}

// buildHRStorage 把 hrStorageEntry 表的 PDU 按 index 聚合成磁盘用量。
// OID 形如 ....1.<col>.<index>：col=2 descr / 3 units / 4 size / 5 used。
func buildHRStorage(entries []gosnmp.SnmpPDU) []DiskMetric {
	rows := map[string]*hrRow{}
	for _, pdu := range entries {
		parts := strings.Split(pdu.Name, ".")
		if len(parts) < 2 {
			continue
		}
		col := parts[len(parts)-2]
		idx := parts[len(parts)-1]
		row := rows[idx]
		if row == nil {
			row = &hrRow{}
			rows[idx] = row
		}
		switch col {
		case "2":
			row.descr, _ = pdu.Value.(string)
		case "3":
			row.units = snmpInt(pdu.Value)
		case "4":
			row.size = snmpInt(pdu.Value)
		case "5":
			row.used = snmpInt(pdu.Value)
		}
	}
	var disks []DiskMetric
	for _, r := range rows {
		if r.size <= 0 || r.units <= 0 {
			continue
		}
		total := uint64(r.size * r.units)
		used := uint64(r.used * r.units)
		var usage float64
		if total > 0 {
			usage = float64(used) / float64(total) * 100
		}
		disks = append(disks, DiskMetric{Device: r.descr, Total: total, Used: used, Usage: usage, Type: "partition"})
	}
	return disks
}
