import { describe, expect, it, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

// 后端同名拒绝的原文（真机实测：POST /storages/1/rename?path=/a.txt&name=b.txt）
const COLLISION = '目标目录 / 下已存在同名条目「b.txt」，已拒绝覆盖'

const state = vi.hoisted(() => ({ renameResult: undefined as unknown }))

vi.mock('@/api/filemanager', () => ({
  fileApi: {
    list: vi.fn(async () => [{ id: 1, name: 'local', type: 'local', url: '', username: '', base_path: '/', agent_id: 0 }]),
    test: vi.fn(async () => ({ ok: true })),
    files: vi.fn(async () => []),
    // api 层把 200+{ok:false,error} 转成异常（filemanager.ts:69），这里复刻该形状。
    rename: vi.fn(async () => {
      if (state.renameResult instanceof Error) throw state.renameResult
      return { ok: true }
    }),
  },
}))
vi.mock('@/api/cloud', () => ({
  cloudApi: { listDrives: vi.fn(async () => []), test: vi.fn(async () => ({ ok: true })), browse: vi.fn(async () => []) },
}))
vi.mock('@/api/monitor', () => ({ agentApi: { list: vi.fn(async () => []) } }))
vi.mock('element-plus', () => ({
  ElMessage: { error: vi.fn(), warning: vi.fn(), success: vi.fn(), info: vi.fn() },
  ElMessageBox: { confirm: vi.fn() },
}))

import { ElMessage } from 'element-plus'
import { useFileManager } from './useFileManager'
import { useFileOps } from './useFileOps'

// 数据丢失护栏的「可见那一半」：后端拒绝覆盖之后，原因必须真的到达用户，
// 而不是被 catch 吞掉或仍弹「已重命名」（useFileOps.ts:76-84）。
describe('useFileOps · 改名撞同名时把拒绝原因显示出来', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    state.renameResult = new Error(COLLISION)
    // useFileManager 现在要读 auth.isAdmin（/storages 是 admin-only）：
    // 这里按管理员身份跑，改名对象才是一个真实的 fs 存储。
    localStorage.setItem('user', JSON.stringify({ username: 'u1', is_admin: true }))
    setActivePinia(createPinia())
  })

  async function ready() {
    const fm = useFileManager()
    await fm.loadStorages()
    const ops = useFileOps(fm)
    fm.renameTarget.value = { name: 'a.txt', path: '/a.txt', is_dir: false, size: 3, mod_time: '' } as never
    fm.renameNewName.value = 'b.txt'
    return { fm, ops }
  }

  it('拒绝原因原样弹错，且不得弹成功', async () => {
    const { ops } = await ready()
    await ops.doRename()
    expect(ElMessage.error).toHaveBeenCalledWith(COLLISION)
    expect(ElMessage.success).not.toHaveBeenCalled()
  })

  it('正控：后端放行时走成功分支（证明上一条红不是因为根本没调用）', async () => {
    state.renameResult = undefined
    const { ops } = await ready()
    await ops.doRename()
    expect(ElMessage.success).toHaveBeenCalledWith('已重命名')
    expect(ElMessage.error).not.toHaveBeenCalled()
  })
})
