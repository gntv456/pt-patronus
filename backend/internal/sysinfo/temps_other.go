//go:build !windows

package sysinfo

import "github.com/shirou/gopsutil/v4/sensors"

// collectTemps Linux/macOS：gopsutil 读 hwmon/sensors。
func collectTemps() []TempMetric {
	var out []TempMetric
	temps, err := sensors.SensorsTemperatures()
	if err != nil {
		return out
	}
	for _, t := range temps {
		if t.Temperature > 0 && t.Temperature < 150 {
			out = append(out, TempMetric{Name: t.SensorKey, Temp: t.Temperature})
		}
	}
	return out
}
