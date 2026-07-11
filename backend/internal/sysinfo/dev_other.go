//go:build !linux

package sysinfo

// devOf 非 Linux 平台（Windows/macOS/开发环境）返 0，collectDisks 见 0 即跳过 st_dev 去重。
// 这些平台无 syscall.Stat_t 或语义不同；开发环境无需精确去重，生产部署在 Linux 有 dev_linux.go。
func devOf(_ string) uint64 { return 0 }
