import { describe, expect, it, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const state = vi.hoisted(() => ({
  drivesFail: false,
  // null=成功；{status} = 抛一个带 response.status 的错误（模拟 403/500）
  fsFail: null as null | { status?: number },
  fsListCalls: 0,
}))

vi.mock('@/api/filemanager', () => ({
  fileApi: {
    list: vi.fn(async () => {
      state.fsListCalls++
      if (state.fsFail) throw { response: { status: state.fsFail.status }, message: 'fs boom' }
      return [{ id: 1, name: 'local', type: 'local', url: '', username: '', base_path: '/', agent_id: 0 }]
    }),
    test: vi.fn(async () => ({ ok: true })),
    files: vi.fn(async () => []),
  },
}))
vi.mock('@/api/cloud', () => ({
  cloudApi: {
    listDrives: vi.fn(async () => {
      if (state.drivesFail) throw new Error('drives 500')
      return [{ id: 2, name: '115', type: '115', token_masked: '', root_path: '/', is_default: false }]
    }),
    test: vi.fn(async () => ({ ok: true })),
    browse: vi.fn(async () => []),
  },
}))
vi.mock('@/api/monitor', () => ({ agentApi: { list: vi.fn(async () => []) } }))
vi.mock('element-plus', () => ({
  ElMessage: { error: vi.fn(), warning: vi.fn(), success: vi.fn(), info: vi.fn() },
  ElMessageBox: { confirm: vi.fn() },
}))

import { useFileManager } from './useFileManager'

beforeEach(() => {
  state.drivesFail = false
  state.fsFail = null
  state.fsListCalls = 0
  // /storages 整组是 admin-only：默认按管理员身份跑，非管理员分支单独测。
  localStorage.setItem('user', JSON.stringify({ username: 'u1', is_admin: true }))
  setActivePinia(createPinia())
})

// 云盘列表读失败 ≠ 没配网盘：drivesFailed 必须置位，侧栏据此显示「加载失败」。
describe('useFileManager · 云盘列表读失败口径', () => {
  it('listDrives 失败时置 drivesFailed，本地存储照常入列', async () => {
    state.drivesFail = true
    const fm = useFileManager()
    await fm.loadStorages()
    expect(fm.drivesFailed.value).toBe(true)
    expect(fm.list.value.map((x) => x.kind)).toEqual(['fs'])
  })

  it('正控：读取成功时 drivesFailed 为 false 且云盘入列', async () => {
    const fm = useFileManager()
    await fm.loadStorages()
    expect(fm.drivesFailed.value).toBe(false)
    expect(fm.list.value.map((x) => x.kind)).toEqual(['fs', 'cloud'])
  })
})

// /storages 读不了（故障或非管理员必然的 403）不能把云盘一起拖没：
// 旧写法只给云盘那侧挂了 catch，Promise.all 直接被存储侧抛掉，侧栏只剩一条错误提示。
describe('useFileManager · 存储后端侧失败不拖垮云盘', () => {
  it('管理员 + /storages 500：fsFailed 置位，云盘照常入列', async () => {
    state.fsFail = { status: 500 }
    const fm = useFileManager()
    await fm.loadStorages()
    expect(fm.fsFailed.value).toBe(true)
    expect(fm.fsForbidden.value).toBe(false)
    expect(fm.list.value.map((x) => x.kind)).toEqual(['cloud'])
  })

  it('管理员 + /storages 403：说「仅管理员」而不是「加载失败」，云盘照常入列', async () => {
    state.fsFail = { status: 403 }
    const fm = useFileManager()
    await fm.loadStorages()
    expect(fm.fsForbidden.value).toBe(true)
    expect(fm.fsFailed.value).toBe(false)
    expect(fm.list.value.map((x) => x.kind)).toEqual(['cloud'])
  })

  it('非管理员：根本不发 GET /storages，云盘照常入列并说明原因', async () => {
    localStorage.setItem('user', JSON.stringify({ username: 'u2', is_admin: false }))
    setActivePinia(createPinia())
    const fm = useFileManager()
    await fm.loadStorages()
    expect(state.fsListCalls).toBe(0)
    expect(fm.fsForbidden.value).toBe(true)
    expect(fm.list.value.map((x) => x.kind)).toEqual(['cloud'])
  })

  it('正控：管理员两边都成功', async () => {
    const fm = useFileManager()
    await fm.loadStorages()
    expect(state.fsListCalls).toBe(1)
    expect(fm.fsFailed.value).toBe(false)
    expect(fm.fsForbidden.value).toBe(false)
    expect(fm.list.value.map((x) => x.kind)).toEqual(['fs', 'cloud'])
  })
})
