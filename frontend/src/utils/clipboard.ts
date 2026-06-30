import { ElMessage } from 'element-plus'

// copyText 把文本写入剪贴板，返回是否成功。
// 优先 navigator.clipboard（HTTPS / localhost），回退 textarea+execCommand
// （纯 HTTP 的局域网部署下 navigator.clipboard 为 undefined，必须走回退，否则按钮恒失败）。
// 成功/失败均弹 ElMessage 提示，故调用方无需再处理——如需静默请改用底层 writeClipboard。
export async function copyText(text: string, okMsg = '已复制', failMsg = '复制失败，请手动选择文本'): Promise<boolean> {
  if (await writeClipboard(text)) {
    ElMessage.success(okMsg)
    return true
  }
  ElMessage.warning(failMsg)
  return false
}

// writeClipboard 仅写入剪贴板，不弹提示。供需要自定义提示/回调的调用方使用。
export async function writeClipboard(text: string): Promise<boolean> {
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      // 权限被拒/剪贴板被占用 → 落到回退
    }
  }
  try {
    const ta = document.createElement('textarea')
    ta.value = text
    ta.style.position = 'fixed'
    ta.style.top = '-9999px'
    ta.style.opacity = '0'
    document.body.appendChild(ta)
    ta.focus()
    ta.select()
    const ok = document.execCommand('copy')
    document.body.removeChild(ta)
    return ok
  } catch {
    return false
  }
}
