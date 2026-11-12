// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { ref } from 'vue'
import { i18n } from '@/i18n'
import { subscribeApi, type Subscription } from '@/api/subscribe'
import { mediaApi } from '@/api/media'

// 订阅列表的加载与逐条操作（手动检查 / 暂停-恢复 / 改分辨率 / 改促销 / 改站点 / 删除）。
// 抽出为 composable 收敛 Subscriptions.vue 脚本体积；ElMessage / ElMessageBox 依赖
// unplugin-auto-import 的 ElementPlusResolver 全局注入（与 .vue 一致，无需手写 import）。

const t = (k: string) => i18n.global.t(k)

export function useSubscriptionList() {
  const list = ref<Subscription[]>([])
  const loading = ref(false)
  const checking = ref<number | null>(null)

  async function load() {
    loading.value = true
    try {
      const items = await subscribeApi.list()
      // 归一列表项切片字段：旧订阅/异常数据可能回 null，模板直接读 .length 会崩整个页面。
      list.value = items.map((s) => ({ ...s, promo_types: s.promo_types ?? [], site_ids: s.site_ids ?? [] }))
      void fillMissingPosters(list.value)
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('common.loadFailed'))
    } finally {
      loading.value = false
    }
  }

  // 兼容尚未升级后端时创建的旧订阅：页面端也按标题补图，最多同时请求 4 个。
  async function fillMissingPosters(items: Subscription[]) {
    const pending = items.filter((s) => !s.poster_url && (s.title || '').trim())
    let next = 0
    const worker = async () => {
      while (next < pending.length) {
        const s = pending[next++]
        try {
          s.poster_url = await mediaApi.poster(s.title, s.year)
        } catch {
          // 单张补图失败不影响订阅列表和其他海报。
        }
      }
    }
    await Promise.all(Array.from({ length: Math.min(4, pending.length) }, worker))
  }

  async function checkNow(s: Subscription) {
    checking.value = s.id
    try {
      const r = await subscribeApi.check(s.id)
      if (r.ok) {
        ElMessage.success(r.pushed > 0 ? `命中并推送 ${r.pushed} 条` : t('opusesubscriptionlist.k1'))
      } else {
        ElMessage.warning(r.error || t('opusesubscriptionlist.k2'))
      }
      await load()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('opusesubscriptionlist.k2'))
    } finally {
      checking.value = null
    }
  }

  async function toggleStatus(s: Subscription) {
    const next = s.status === 'paused' ? 'subscribed' : 'paused'
    try {
      await subscribeApi.update(s.id, { status: next })
      await load()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('common.actionFailed'))
    }
  }

  async function setResolution(s: Subscription) {
    try {
      await subscribeApi.update(s.id, { resolution: s.resolution })
      ElMessage.success(t('opusesubscriptionlist.k3'))
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('common.updateFailed'))
    }
  }

  // 促销类型多选：el-select 实时 change → 乐观更新 + 失败回滚（空数组=不限）。
  async function setPromoTypes(s: Subscription, vals: string[]) {
    const prev = s.promo_types
    s.promo_types = vals
    try {
      await subscribeApi.update(s.id, { promo_types: vals })
    } catch (e: unknown) {
      s.promo_types = prev
      ElMessage.error((e as Error).message || t('common.updateFailed'))
    }
  }

  // 参与站点：SiteFilterDialog 实时 emit 选中态，乐观更新本地 + 失败回滚（空数组=全部活跃站）。
  async function setSiteIds(s: Subscription, ids: number[]) {
    const prev = s.site_ids
    s.site_ids = ids
    try {
      await subscribeApi.update(s.id, { site_ids: ids })
    } catch (e: unknown) {
      s.site_ids = prev
      ElMessage.error((e as Error).message || t('common.updateFailed'))
    }
  }

  async function remove(s: Subscription) {
    try {
      await ElMessageBox.confirm(t('opusesubscriptionlist.k4'), t('opusesubscriptionlist.k5'), {
        type: 'warning',
        confirmButtonText: t('common.delete'),
        cancelButtonText: t('common.cancel'),
      })
    } catch {
      return // 用户取消
    }
    try {
      await subscribeApi.remove(s.id)
      await load()
      ElMessage.success(t('common.deleted'))
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('common.deleteFailed'))
    }
  }

  return { list, loading, checking, load, checkNow, toggleStatus, setResolution, setPromoTypes, setSiteIds, remove }
}
