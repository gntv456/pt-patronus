// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import type { Ref } from 'vue'
import { i18n } from '@/i18n'
import { subscribeApi, type Subscription, type GlobalDefaults } from '@/api/subscribe'

// 订阅三态开关簇：评分保护 / 洗版 / 仅官种。
// 订阅自定义优先（enable_rating_guard/best_version/official_only *bool 三态），null 回退全局默认。
// 抽出为 composable 收敛 Subscriptions.vue 脚本体积；三组 set/reset 同构（乐观更新 + 失败回滚，reset 走「恢复全局」）。
// ElMessage 依赖 unplugin-auto-import 的 ElementPlusResolver 全局注入（与 .vue 一致，无需手写 import）。

const t = (k: string) => i18n.global.t(k)

export function useSubscriptionToggles(globalDefaults: Ref<GlobalDefaults>) {
  // 有效值：订阅自定义优先，null 回退全局默认。
  function effRatingGuard(s: Subscription) {
    return s.enable_rating_guard ?? globalDefaults.value.enable_rating_guard
  }
  function effBestVersion(s: Subscription) {
    return s.best_version ?? globalDefaults.value.best_version
  }
  function effOfficialOnly(s: Subscription) {
    return s.official_only ?? globalDefaults.value.official_only
  }

  // 三态开关：设订阅自定义值（true/false）；恢复跟随全局走 reset。
  async function setRatingGuard(s: Subscription, val: boolean) {
    const prev = s.enable_rating_guard
    s.enable_rating_guard = val
    try {
      await subscribeApi.update(s.id, { enable_rating_guard: val })
    } catch (e: unknown) {
      s.enable_rating_guard = prev
      ElMessage.error((e as Error).message || t('common.updateFailed'))
    }
  }
  async function resetRatingGuard(s: Subscription) {
    const prev = s.enable_rating_guard
    s.enable_rating_guard = null
    try {
      await subscribeApi.reset(s.id, ['enable_rating_guard'])
      ElMessage.success(t('opusesubscriptiontoggles.k1'))
    } catch (e: unknown) {
      s.enable_rating_guard = prev
      ElMessage.error((e as Error).message || t('common.updateFailed'))
    }
  }

  async function setBestVersion(s: Subscription, val: boolean) {
    const prev = s.best_version
    s.best_version = val
    try {
      await subscribeApi.update(s.id, { best_version: val })
    } catch (e: unknown) {
      s.best_version = prev
      ElMessage.error((e as Error).message || t('common.updateFailed'))
    }
  }
  async function resetBestVersion(s: Subscription) {
    const prev = s.best_version
    s.best_version = null
    try {
      await subscribeApi.reset(s.id, ['best_version'])
      ElMessage.success(t('opusesubscriptiontoggles.k2'))
    } catch (e: unknown) {
      s.best_version = prev
      ElMessage.error((e as Error).message || t('common.updateFailed'))
    }
  }

  async function setOfficialOnly(s: Subscription, val: boolean) {
    const prev = s.official_only
    s.official_only = val
    try {
      await subscribeApi.update(s.id, { official_only: val })
    } catch (e: unknown) {
      s.official_only = prev
      ElMessage.error((e as Error).message || t('common.updateFailed'))
    }
  }
  async function resetOfficialOnly(s: Subscription) {
    const prev = s.official_only
    s.official_only = null
    try {
      await subscribeApi.reset(s.id, ['official_only'])
      ElMessage.success(t('opusesubscriptiontoggles.k3'))
    } catch (e: unknown) {
      s.official_only = prev
      ElMessage.error((e as Error).message || t('common.updateFailed'))
    }
  }

  return {
    effRatingGuard,
    effBestVersion,
    effOfficialOnly,
    setRatingGuard,
    resetRatingGuard,
    setBestVersion,
    resetBestVersion,
    setOfficialOnly,
    resetOfficialOnly,
  }
}
