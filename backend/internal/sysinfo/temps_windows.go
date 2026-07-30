//go:build windows

package sysinfo

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/sensors"
	"github.com/yusufpapurcu/wmi"
)

// collectTemps Windows 多源合并：
//  1. LibreHardwareMonitor WMI（推荐，无需本进程提权）
//  2. OpenHardwareMonitor WMI
//  3. gopsutil → MSAcpi_ThermalZoneTemperature（常需管理员，失败则空）
//  4. nvidia-smi GPU 温度（有 NVIDIA 驱动时）
//
// 多数消费级 Win10 默认不暴露 ACPI 温度，未装 LHM/OHM 且非管理员时会为空。
func collectTemps() []TempMetric {
	seen := map[string]bool{}
	var out []TempMetric
	add := func(name string, temp float64) {
		if temp <= 0 || temp > 150 {
			return
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		key := strings.ToLower(name)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, TempMetric{Name: name, Temp: temp})
	}

	for _, t := range wmiHardwareTemps(`root\LibreHardwareMonitor`) {
		add("lhm_"+sanitizeTempName(t.name), t.temp)
	}
	for _, t := range wmiHardwareTemps(`root\OpenHardwareMonitor`) {
		add("ohm_"+sanitizeTempName(t.name), t.temp)
	}
	if temps, err := sensors.SensorsTemperatures(); err == nil {
		for _, t := range temps {
			add(t.SensorKey, t.Temperature)
		}
	}
	for _, t := range nvidiaSMITemps() {
		add(t.Name, t.Temp)
	}
	return out
}

type namedTemp struct {
	name string
	temp float64
}

// lhmSensor 匹配 LibreHardwareMonitor / OpenHardwareMonitor 的 WMI Sensor 类。
type lhmSensor struct {
	Name       string
	SensorType string
	Value      float32
}

func wmiHardwareTemps(namespace string) []namedTemp {
	var dst []lhmSensor
	// SensorType 过滤在 WMI 查询里做，减少回传量。
	q := "SELECT Name, SensorType, Value FROM Sensor WHERE SensorType='Temperature'"
	if err := wmi.QueryNamespace(q, &dst, namespace); err != nil || len(dst) == 0 {
		return nil
	}
	var out []namedTemp
	for _, s := range dst {
		out = append(out, namedTemp{name: s.Name, temp: float64(s.Value)})
	}
	return out
}

func sanitizeTempName(s string) string {
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "/", "_")
	return s
}

// nvidiaSMITemps 调用 nvidia-smi 读 GPU 温度；无驱动/不在 PATH 则跳过。
func nvidiaSMITemps() []TempMetric {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "nvidia-smi",
		"--query-gpu=index,name,temperature.gpu",
		"--format=csv,noheader,nounits")
	b, err := cmd.Output()
	if err != nil {
		return nil
	}
	var out []TempMetric
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// "0, NVIDIA GeForce RTX 3060, 45"
		parts := strings.Split(line, ",")
		if len(parts) < 3 {
			continue
		}
		idx := strings.TrimSpace(parts[0])
		tempStr := strings.TrimSpace(parts[len(parts)-1])
		temp, err := strconv.ParseFloat(tempStr, 64)
		if err != nil {
			continue
		}
		out = append(out, TempMetric{Name: "nvidia_gpu_" + idx, Temp: temp})
	}
	return out
}
