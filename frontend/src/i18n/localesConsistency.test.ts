// zh-CN / en-US 翻译表 key 集合一致性：
//   - 两表逐层展开成点分 key 全集，双向 diff 必须为空
//   - zh 缺失 → 英文用户回落中文（静默漏译）；en 缺失 → 中文用户切英文缺文案
//   - 结构错位（一边是字符串一边是对象）同样报出
// 新增文案只改了一张表时，此测试红——把 key 补到另一张即可。
import { describe, expect, it } from 'vitest'
import zhCN from './locales/zh-CN'
import enUS from './locales/en-US'
import cameraBridge from './locales/cameraBridge'

// 摄像头接入指引是独立模块（主表在行数棘轮上，装不下这类易增长的长文案），
// 实际 messages = 主表 + 指引模块（见 i18n/index.ts 的 loadLocale）。
// 这里按同样口径比对两边：指引模块自身必须 zh/en 对齐，
// 否则切到英文的设备监控页会静默回落中文。

type Node = string | { [k: string]: Node }

/** 递归展开嵌套对象为点分 key 集合；字符串叶子即终止。 */
function flatten(obj: Node, prefix = ''): Map<string, 'leaf' | 'branch'> {
  const out = new Map<string, 'leaf' | 'branch'>()
  for (const [k, v] of Object.entries(obj)) {
    const path = prefix ? `${prefix}.${k}` : k
    if (typeof v === 'string') out.set(path, 'leaf')
    else {
      out.set(path, 'branch')
      for (const [p, kind] of flatten(v, path)) out.set(p, kind)
    }
  }
  return out
}

// 主表与指引模块合并后再比对——实际 messages 就是这个组合（见 i18n/index.ts 的 loadLocale）。
// 用 unknown 中转再断言为对象，避免 TS 把联合类型判成 string 拒绝展开。
const zhKeys = flatten({
  ...(zhCN as unknown as Record<string, Node>),
  ...(cameraBridge.zh as unknown as Record<string, Node>),
})
const enKeys = flatten({
  ...(enUS as unknown as Record<string, Node>),
  ...(cameraBridge.en as unknown as Record<string, Node>),
})

describe('i18n 翻译表一致性', () => {
  it('两表 key 数量同级（zh 与 en 展开后条目数一致）', () => {
    // 数量不等不直接判失败（下方逐 key diff 会给出精确清单），先给出可读的规模对比。
    expect({
      zh: zhKeys.size,
      en: enKeys.size,
    }).toEqual({ zh: zhKeys.size, en: zhKeys.size })
  })

  it('en-US 不缺 key（缺失=英文用户静默看到中文）', () => {
    const missing = [...zhKeys.keys()].filter((k) => !enKeys.has(k))
    expect(missing, `en-US 缺失 ${missing.length} 个 key:\n${missing.slice(0, 30).join('\n')}`).toEqual([])
  })

  it('zh-CN 不缺 key（缺失=切英文后回中文缺文案）', () => {
    const missing = [...enKeys.keys()].filter((k) => !zhKeys.has(k))
    expect(missing, `zh-CN 缺失 ${missing.length} 个 key:\n${missing.slice(0, 30).join('\n')}`).toEqual([])
  })

  it('同名 key 结构一致（一边字符串一边对象即报出）', () => {
    const mismatched: string[] = []
    for (const [k, kind] of zhKeys) {
      const other = enKeys.get(k)
      if (other !== undefined && other !== kind) mismatched.push(`${k}: zh=${kind} en=${other}`)
    }
    expect(mismatched, `结构错位 ${mismatched.length} 处:\n${mismatched.slice(0, 20).join('\n')}`).toEqual([])
  })
})
