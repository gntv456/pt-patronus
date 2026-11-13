// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 把筛选 / Tab / 视图模式 / 周期等界面状态同步到 URL query，
// 使页面可分享、刷新与后退可恢复（skill: deep-linking / back-behavior）。
//
// 无 router 注入的场景（部分单测直接挂载组件、未 app.use(router)）会降级为
// 组件内本地 ref：功能等价、不抛未捕获错误，也不影响测试断言。
import { computed, ref, type WritableComputedRef } from 'vue'
import { useRoute, useRouter } from 'vue-router'

function safeRouter(): { route: ReturnType<typeof useRoute> | undefined; router: ReturnType<typeof useRouter> | undefined } {
  try {
    const route = useRoute()
    const router = useRouter()
    const ok = !!route && typeof route.query === 'object' && !!router && typeof router.replace === 'function'
    return ok ? { route, router } : { route: undefined, router: undefined }
  } catch {
    return { route: undefined, router: undefined }
  }
}

export function useQueryState(key: string, initial = ''): WritableComputedRef<string> {
  const { route, router } = safeRouter()
  if (!route || !router) {
    const local = ref(initial)
    return computed<string>({ get: () => local.value, set: (v: string) => { local.value = v } })
  }
  return computed<string>({
    get: () => (typeof route.query[key] === 'string' ? (route.query[key] as string) : initial),
    set: (value: string) => {
      const next = { ...route.query }
      if (value === '' || value === initial) delete next[key]
      else next[key] = value
      void router.replace({ query: next })
    },
  })
}

/** 批量读取若干 query 键，返回带默认值的响应式对象 */
export function useQueryStates<K extends string>(
  defaults: Record<K, string>,
): Record<K, WritableComputedRef<string>> {
  const { route, router } = safeRouter()
  const state = {} as Record<K, WritableComputedRef<string>>
  for (const key of Object.keys(defaults) as K[]) {
    if (!route || !router) {
      const local = ref(defaults[key])
      state[key] = computed<string>({ get: () => local.value, set: (v: string) => { local.value = v } })
      continue
    }
    state[key] = computed<string>({
      get: () => (typeof route.query[key] === 'string' ? (route.query[key] as string) : defaults[key]),
      set: (value: string) => {
        const next = { ...route.query }
        if (value === '' || value === defaults[key]) delete next[key]
        else next[key] = value
        void router.replace({ query: next })
      },
    })
  }
  return state
}
