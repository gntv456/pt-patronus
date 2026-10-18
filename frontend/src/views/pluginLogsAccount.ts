// pluginLogsAccount.ts —— 插件日志窗口提示的单一文案来源。
// 两处消费（插件「运行日志」抽屉、插件「审计面板」）共用一份判定，避免同一条话抄两遍后漂移。

/**
 * 单页封顶 500 条：被截断时把窗口说出来，否则更早的插件日志（含报错那些）
 * 在界面上等于不存在；总数读不出时不写成 0，一次拿全则不出声。
 */
export function pluginLogsNotice(page: { shown: number; total?: number; truncated: boolean }): string {
  if (!page.truncated) return ''
  if (page.total == null) {
    return `已显示最近 ${page.shown} 条日志，但总数读取失败——可能还有更多`
  }
  if (page.total <= page.shown) return ''
  return `仅显示最近 ${page.shown} 条日志（该插件共 ${page.total} 条，更早的未加载）`
}
