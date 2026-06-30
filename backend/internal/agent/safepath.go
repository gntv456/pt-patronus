package agent

import (
	"path"
	"strings"
)

// safePath 把用户路径 p 钳制在 base 之下，防止 ../ 越界（远程路径穿越）。
// 逻辑同 filemanager.resolvePath，但 agent 不能 import filemanager（后者会拉入 oper 依赖，破坏 agent 轻量），
// 故独立实现。返回以 "/" 开头的绝对 POSIX 路径。例：base=/media, p=../../../etc → /media/etc。
func safePath(base, p string) string {
	// 统一分隔符：Windows 反斜杠在 POSIX path.Clean 里只是普通字符，会被原样保留；
	// 进而在 agent 端经 filepath.FromSlash 还原为 \ 后被 OS 当作分隔符 → 路径穿越。
	// 先把 \ 规范成 / 再做 POSIX 清理，堵住 Windows agent 的反斜杠绕过。
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
