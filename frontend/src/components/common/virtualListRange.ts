// virtualListRange — 虚拟列表范围计算（纯函数，便于单测；<script setup> 不允许 export）
export function computeRange(scrollTop: number, viewportH: number, itemHeight: number, overscan: number, total: number) {
  const h = Math.max(1, itemHeight)
  // start 也要夹到 total 以内：滚动位置远超内容（数据突然变短）时，
  // 未夹会让 start > end，slice 出空列表并显示大片空白。
  const start = Math.min(Math.max(0, Math.floor(scrollTop / h) - overscan), Math.max(0, total - 1))
  const end = Math.min(total, Math.ceil((scrollTop + viewportH) / h) + overscan)
  const safeEnd = Math.max(start, end)
  return { start, end: safeEnd, padTop: start * h, padBottom: Math.max(0, (total - safeEnd) * h) }
}