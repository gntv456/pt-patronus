// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { ref, computed, type Ref } from 'vue'
import { i18n } from '@/i18n'
import {
  siteApi,
  type Site,
  type TestResult,
  type CheckinResult,
  type CheckinRecord,
  type SiteConnectivityStatus,
} from '@/api/site'

function errResp(e: unknown): string | undefined {
  return (e as { response?: { data?: { error?: string } } })?.response?.data?.error
}


const t = (k: string) => i18n.global.t(k)

export function useSitesActions(
  sites: Ref<Site[]>,
  connectivity: Ref<Record<number, SiteConnectivityStatus>>,
  reload: () => Promise<void>,
  operationsMatrixReload: () => Promise<void>,
) {
  const testing = ref<number | null>(null)
  const checkining = ref<number | null>(null)
  const checkinAllLoading = ref(false)

  const recordsDrawer = ref(false)
  const records = ref<CheckinRecord[]>([])
  const recordsLoading = ref(false)
  // 签到记录单页 100 条封顶（可 offset 翻页）：抽屉只装第一页，被截断时得说出来。
  const recordsWindow = ref<{ shown: number; total?: number; offset: number; truncated: boolean }>({
    shown: 0,
    total: undefined,
    offset: 0,
    truncated: false,
  })

  async function testSite(s: Site) {
    testing.value = s.id
    try {
      const r: TestResult = await siteApi.test(s.id)
      connectivity.value = {
        ...connectivity.value,
        [s.id]: {
          site_id: s.id,
          site_name: s.name,
          status: r.ok ? 'connected' : 'disconnected',
          level: r.level,
          error: r.error,
        },
      }
      if (r.ok) ElMessage.success(`${s.name}：连接正常，等级 ${r.level || '-'}`)
      else ElMessage.warning(`${s.name}：${r.error || t('opusesitesactions.k1')}`)
    } catch (e: unknown) {
      const msg = errResp(e) || '请求失败'
      connectivity.value = {
        ...connectivity.value,
        [s.id]: { site_id: s.id, site_name: s.name, status: 'disconnected', error: msg },
      }
      ElMessage.error(msg)
    } finally {
      testing.value = null
    }
  }

  // 单站签到：仅 NexusPHP 站适配器实现了 CheckinCapable（馒头等走 API key 无传统签到）。
  async function checkinSite(s: Site) {
    checkining.value = s.id
    try {
      const r: CheckinResult = await siteApi.checkin(s.id)
      if (r.success) {
        if (r.bonus > 0) ElMessage.success(`${s.name}：签到成功，获得 ${r.bonus} 魔力`)
        else if (r.message?.includes('已签到')) ElMessage.info(`${s.name}：今天已签到`)
        else ElMessage.success(`${s.name}：${r.message || t('opusesitesactions.k2')}`)
      } else {
        const reason = r.error || r.message || '签到失败'
        if (reason.includes('not supported')) ElMessage.warning(`${s.name}：该站点暂不支持签到`)
        else if (reason.includes('not logged in')) ElMessage.warning(`${s.name}：未登录，请先登录`)
        else ElMessage.error(`${s.name}：${reason}`)
      }
    } catch (e: unknown) {
      const msg = errResp(e)
      ElMessage.error(msg || `${s.name}：请求失败`)
    } finally {
      checkining.value = null
    }
  }

  // 一键签到全部启用站点。不支持签到的站（馒头等）聚合时跳过，不计入失败，避免误导。
  async function checkinAll() {
    checkinAllLoading.value = true
    try {
      const { results, listError } = await siteApi.checkinAll()
      // 「清单读不出」与「没有启用的站点」是两件事，必须分开说（#258）。
      if (listError) {
        ElMessage.error(`站点清单读不出来：${listError}`)
        return
      }
      if (!results.length) {
        ElMessage.info(t('opusesitesactions.k3'))
        return
      }
      let ok = 0
      let already = 0
      const failed: string[] = []
      for (const r of results) {
        if (r.success) {
          if (r.message?.includes('已签到')) already++
          else ok++
        } else if (!(r.error || '').includes('not supported')) {
          failed.push(r.site_name)
        }
      }
      const summary = `成功 ${ok} / 已签 ${already}`
      if (!failed.length) ElMessage.success(`一键签到完成：${summary}`)
      else ElMessage.warning(`一键签到完成：${summary} / 失败 ${failed.length}（${failed.join('、')}）`)
    } catch (e: unknown) {
      const msg = errResp(e)
      ElMessage.error(msg || t('opusesitesactions.k4'))
    } finally {
      checkinAllLoading.value = false
    }
  }

  // 签到记录抽屉：siteId 省略=全部站点最近 100 条（含 cron 自动 + 手动）。
  async function openRecords(siteId?: number) {
    recordsDrawer.value = true
    recordsLoading.value = true
    try {
      const page = await siteApi.records(siteId)
      records.value = page.records
      recordsWindow.value = {
        shown: page.records.length,
        total: page.total,
        offset: page.offset,
        truncated: page.truncated,
      }
    } catch {
      ElMessage.error(t('opusesitesactions.k5'))
      records.value = []
      recordsWindow.value = { shown: 0, total: undefined, offset: 0, truncated: false }
    } finally {
      recordsLoading.value = false
    }
  }

  async function removeSite(s: Site) {
    try {
      await ElMessageBox.confirm(`确认删除站点「${s.name}」？`, t('common.delete'), { type: 'warning' })
    } catch {
      return // 用户取消
    }
    try {
      await siteApi.remove(s.id)
      ElMessage.success(t('common.deleted'))
      await reload()
      await operationsMatrixReload()
    } catch (e: any) {
      ElMessage.error(e?.message || t('common.deleteFailed'))
    }
  }

  async function batchRemoveSites(selected: Site[]) {
    if (!selected.length) return
    const names = selected.map((s) => s.name).join('、')
    try {
      await ElMessageBox.confirm(`确认删除选中的 ${selected.length} 个站点？\n${names}`, t('opusesitesactions.k6'), {
        type: 'warning',
      })
    } catch {
      return // 用户取消
    }
    try {
      const res = await siteApi.batchDelete(selected.map((s) => s.id))
      if (res.failed?.length) {
        ElMessage.warning(`已删除 ${res.deleted} 个，失败 ${res.failed.length} 个`)
      } else {
        ElMessage.success(`已删除 ${res.deleted} 个站点`)
      }
      await reload()
      await operationsMatrixReload()
    } catch (e: any) {
      ElMessage.error(e?.message || t('opusesitesactions.k7'))
    }
  }

  return {
    testing,
    checkining,
    checkinAllLoading,
    recordsDrawer,
    records,
    recordsLoading,
    recordsWindow,
    testSite,
    checkinSite,
    checkinAll,
    openRecords,
    removeSite,
    batchRemoveSites,
  }
}
