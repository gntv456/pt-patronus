//go:build linux

package sysinfo

import (
	"os"
	"syscall"
)

// devOf 返回路径所在设备的 st_dev，用于同设备多挂载点/bind mount 去重。
// os.Stat 失败或类型断言不成立时返 0（collectDisks 见 0 即跳过去重）。
func devOf(path string) uint64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	if s, ok := fi.Sys().(*syscall.Stat_t); ok {
		return uint64(s.Dev)
	}
	return 0
}
