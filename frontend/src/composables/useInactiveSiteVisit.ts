/**
 * 站点运营矩阵的「访问不活跃站点」逻辑（从 SiteOperationsMatrix.vue 抽出）。
 * 依次在弹窗里打开超过 30 天未访问的启用站点，间隔 8 秒，支持取消。
 */
import { onBeforeUnmount } from 'vue'
import type { ComputedRef } from 'vue'
import type { Site } from '@/api/site'
import { safeUrl } from '@/utils/safeUrl'

type InactiveTarget = { row: { site_id: number; site_name: string; last_access_at: string }; site?: Site }

function wait(duration: number) {
  return new Promise<void>((resolve) => window.setTimeout(resolve, duration))
}

export function useInactiveSiteVisit(inactiveSites: ComputedRef<InactiveTarget[]>) {
  let visitPopup: Window | null = null
  let visitCancelled = false

  async function visitInactiveSites(
    onProgress?: (current: number, total: number, siteName: string) => void,
  ): Promise<number> {
    const targets = [...inactiveSites.value]
    if (!targets.length) {
      ElMessage.info('没有最后访问超过 30 天的启用站点')
      return 0
    }

    const popup = window.open('about:blank', 'ptpatronus_inactive_sites')
    if (!popup) {
      ElMessage.error('浏览器阻止了访问窗口，请允许本站打开弹窗后重试')
      return 0
    }

    visitPopup = popup
    visitCancelled = false
    let visited = 0
    for (let index = 0; index < targets.length; index++) {
      if (visitCancelled || popup.closed) {
        ElMessage.warning(`访问任务已停止，已完成 ${visited} / ${targets.length} 个站点`)
        return visited
      }
      const { site } = targets[index]
      if (!site) continue
      const url = safeUrl(site.base_url)
      onProgress?.(index + 1, targets.length, site.name)
      try {
        popup.location.href = url
        popup.focus()
        await wait(8_000)
        visited++
        if (!popup.closed) popup.location.href = 'about:blank'
        await wait(250)
      } catch {
        // 单个站点导航失败时继续处理后续站点。
      }
    }

    if (!popup.closed) popup.close()
    visitPopup = null
    ElMessage.success(`已依次访问 ${visited} 个超过 30 天未访问的站点`)
    return visited
  }

  onBeforeUnmount(() => {
    visitCancelled = true
    if (visitPopup && !visitPopup.closed) visitPopup.close()
    visitPopup = null
  })

  return { visitInactiveSites }
}
