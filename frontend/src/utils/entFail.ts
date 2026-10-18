import { ElMessage } from 'element-plus'

// 娱乐/游戏各卡每个读请求都各自写着 `.catch(() => [])`／`.catch(() => null)`：
// 站点 cookie 失效、被墙、超时时，界面上和「这个号真没数据」长得一模一样
// （0 次、暂无记录、无奖品），操作者不会知道其实什么都没读到。
// 这里保留「不炸卡」的行为，但把合并窗口内的失败收成一条提示。

const MERGE_WINDOW_MS = 1500
const REASON_MAX = 90

let timer: ReturnType<typeof setTimeout> | null = null

// label → 后端自产的那句中文原因（errcode 信封的 error 字段）。
// 只记 label 不够：「farm 未能读取」不告诉用户该干什么，
// 而「站点凭据解不开，请到站点里重新保存」是可照做的一步。
const reasons = new Map<string, string>()

/** 从失败响应里取「程序自己写给人看的那一句」；取不到就留空，宁可用兜底文案。 */
function reasonOf(e: unknown): string {
  const data = (e as { response?: { data?: { error?: string } } })?.response?.data
  const raw = typeof data?.error === 'string' ? data.error.trim() : ''
  return raw.length > REASON_MAX ? `${raw.slice(0, REASON_MAX)}…` : raw
}

export function aggregateEntFails(list: string[]): string {
  if (!list.length) return ''
  const head = list.slice(0, 4).join('、')
  return list.length > 4 ? `${head} 等 ${list.length} 项` : head
}

function items(): string[] {
  return [...reasons.keys()].map((label) => {
    const reason = reasons.get(label) || ''
    return reason ? `${label}（${reason}）` : label
  })
}

function flush() {
  timer = null
  const text = aggregateEntFails(items())
  reasons.clear()
  if (text) ElMessage.warning(`游戏数据未能读取：${text}（卡面显示的是空值，不代表站点真实结果）`)
}

/** 给 .catch() 用的兜底：记下这次失败的读数名（连同后端原因），返回原本的降级值。 */
export function entFail<T>(label: string, fallback: T): (e?: unknown) => T {
  return (e?: unknown) => {
    const reason = e === undefined ? '' : reasonOf(e)
    reasons.set(label, reason || reasons.get(label) || '')
    if (timer) clearTimeout(timer)
    timer = setTimeout(flush, MERGE_WINDOW_MS)
    return fallback
  }
}

// 测试用：丢弃未 flushing 的合并窗口，避免串到下一个用例。
export function resetEntFails() {
  reasons.clear()
  if (timer) clearTimeout(timer)
  timer = null
}
