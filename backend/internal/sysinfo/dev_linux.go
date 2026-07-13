//go:build linux

package sysinfo

import (
	"os"
	"syscall"
	"unsafe"
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

// fsidOf 返回路径所属文件系统的 fsid（statfs 的 f_fsid）。
// 用于跨子卷去重：btrfs/zfs 等同一文件系统的所有 subvolume/snapshot 共享相同 fsid，
// 而每个 subvolume 的 st_dev 是动态 anon_dev 各不相同——故这类存储池必须按 fsid 去重，
// 否则同一物理池会被每个子卷重复计入（飞牛 btrfs 实测把 50.92TB 池重复累加 ~22 次成 1.1PB）。
// statfs 失败或 fsid 全 0（个别伪 FS）时返 0，collectDisks 据此退化为 st_dev 去重。
func fsidOf(path string) uint64 {
	var s syscall.Statfs_t
	if err := syscall.Statfs(path, &s); err != nil {
		return 0
	}
	// syscall.Fsid 的字段名是 cgo 生成、随 Go 版本变化（Val / X__val），用 unsafe 当 [2]int32 读最稳。
	raw := (*[2]int32)(unsafe.Pointer(&s.Fsid))
	hi, lo := uint64(uint32(raw[0])), uint64(uint32(raw[1]))
	if hi == 0 && lo == 0 {
		return 0
	}
	return hi<<32 | lo
}
