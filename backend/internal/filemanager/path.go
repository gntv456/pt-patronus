package filemanager

import (
	"path"
	"strings"
)

// resolvePath 将用户路径 p 规范化并约束在 base 之下，防止 ../ 越界到 base 之外（远端路径穿越）。
// base 为空时仅做清理（以根 / 为边界，仍能消除 ../ 与重复分隔符）。
// 返回以 "/" 开头的绝对路径。例：base=/media, p=../../../etc → /media/etc。
func resolvePath(base, p string) string {
	// 统一分隔符：Windows 反斜杠在 POSIX path.Clean 里只是普通字符会被原样保留，
	// 随后被后端文件系统或 agent 端解析为分隔符 → 路径穿越。先规范成 / 再清理。
	p = strings.ReplaceAll(p, "\\", "/")
	cleaned := path.Clean("/" + strings.TrimLeft(p, "/"))
	b := strings.Trim(base, "/")
	if b == "" {
		return cleaned
	}
	baseAbs := "/" + b
	if cleaned == baseAbs || strings.HasPrefix(cleaned, baseAbs+"/") {
		return cleaned
	}
	return path.Clean(baseAbs + "/" + strings.TrimLeft(cleaned, "/"))
}

func isResolvedStorageRoot(base, p string) bool {
	return resolvePath(base, strings.TrimSpace(p)) == resolvePath(base, "/")
}
