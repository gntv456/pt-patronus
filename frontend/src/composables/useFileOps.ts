// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { ElMessage, ElMessageBox } from 'element-plus'
import { i18n } from '@/i18n'
import { fileApi, targetReadFailedNotice, type FileStorage, type FileEntry } from '@/api/filemanager'
import { cloudApi } from '@/api/cloud'
import type { useFileManager } from './useFileManager'
import type { StorageItem } from './useFileManager'

type FileManager = ReturnType<typeof useFileManager>
const ILLEGAL_NAME_RE = /[\/\\:*?"<>|]/


const t = (k: string, params?: Record<string, unknown>) => i18n.global.t(k, params ?? {})

export function useFileOps(fm: FileManager) {
  const refresh = fm.refresh
  const loadStorages = fm.loadStorages

  // ── 写操作（仅 fs；只读后端模板隐藏入口）──
  async function doMkdir() {
    if (!fm.curFs.value || fm.submitting.value) return
    const name = fm.mkdirName.value.trim()
    if (!name) return
    if (ILLEGAL_NAME_RE.test(name)) {
      ElMessage.warning(t('opusefileops.k1'))
      return
    }
    fm.submitting.value = true
    try {
      await fileApi.mkdir(fm.curFs.value.id, fm.path.value.replace(/\/$/, '') + '/' + name)
      ElMessage.success(t('opusefileops.k2'))
      fm.mkdirVisible.value = false
      fm.mkdirName.value = ''
      await refresh()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('opusefileops.k3'))
    } finally {
      fm.submitting.value = false
    }
  }

  async function handleUpload(opt: { file: File }) {
    if (!fm.curFs.value) return
    const dup = fm.entries.value.find((e) => !e.is_dir && e.name === opt.file.name)
    if (dup) {
      try {
        await ElMessageBox.confirm(`「${opt.file.name}」已存在，上传将覆盖，是否继续？`, t('opusefileops.k4'), {
          type: 'warning',
        })
      } catch {
        return
      }
    }
    fm.uploadProgress.value = 0
    try {
      await fileApi.upload(fm.curFs.value.id, fm.path.value, opt.file, (p) => (fm.uploadProgress.value = p))
      ElMessage.success(t('opusefileops.k5'))
      await refresh()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('opusefileops.k6'))
    } finally {
      fm.uploadProgress.value = -1
    }
  }

  function openRename(e: FileEntry) {
    fm.renameTarget.value = e
    fm.renameNewName.value = e.name
    fm.renameVisible.value = true
  }
  async function doRename() {
    if (!fm.curFs.value || !fm.renameTarget.value || fm.submitting.value) return
    const name = fm.renameNewName.value.trim()
    if (!name) return
    if (ILLEGAL_NAME_RE.test(name)) {
      ElMessage.warning(t('opusefileops.k1'))
      return
    }
    fm.submitting.value = true
    try {
      await fileApi.rename(fm.curFs.value.id, fm.renameTarget.value.path, name)
      ElMessage.success(t('opusefileops.k7'))
      fm.renameVisible.value = false
      await refresh()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('opusefileops.k8'))
    } finally {
      fm.submitting.value = false
    }
  }

  async function delOne(e: FileEntry) {
    if (!fm.curFs.value) return
    try {
      await ElMessageBox.confirm(`确认删除「${e.name}」？${e.is_dir ? t('opusefileops.k9') : ''}`, t('common.delete'), {
        type: 'warning',
      })
    } catch {
      return
    }
    try {
      await fileApi.removeFile(fm.curFs.value.id, e.path)
      ElMessage.success(t('common.deleted'))
      await refresh()
    } catch (e2: unknown) {
      ElMessage.error((e2 as Error).message || t('common.deleteFailed'))
    }
  }

  // 批量删除取消标志
  let batchDelCancelled = false
  function cancelBatchDel() {
    batchDelCancelled = true
  }

  async function delBatch() {
    if (!fm.curFs.value || !fm.selected.value.length) return
    const items = [...fm.selected.value]
    try {
      await ElMessageBox.confirm(`确认删除选中的 ${items.length} 项？`, t('opusefileops.k10'), { type: 'warning' })
    } catch {
      return
    }
    batchDelCancelled = false
    fm.batchDelProgress.value = { done: 0, total: items.length }
    let ok = 0
    let fail = 0
    for (let i = 0; i < items.length; i++) {
      if (batchDelCancelled) break
      const e = items[i]
      try {
        await fileApi.removeFile(fm.curFs.value.id, e.path)
        ok++
      } catch (err) {
        fail++
        ElMessage.error(`「${e.name}」删除失败：${(err as Error).message}`)
      }
      fm.batchDelProgress.value = { done: i + 1, total: items.length }
    }
    fm.batchDelProgress.value = null
    const cancelled = batchDelCancelled
    if (ok > 0) {
      const tail = (fail > 0 ? t('opusefileops.tailFailed', { n: fail }) : '') + (cancelled ? t('opusefileops.tailCancelled') : '')
      ElMessage.success(`已删除 ${ok} 项${tail}`)
    } else if (!cancelled) {
      ElMessage.error(t('opusefileops.k11'))
    }
    await refresh()
  }

  function openMoveCopy(mode: 'move' | 'copy', sources: FileEntry[]) {
    fm.moveCopyMode.value = mode
    fm.moveCopySources.value = sources
    fm.moveCopyPath.value = fm.path.value
    fm.moveCopyVisible.value = true
    refreshMoveCopy()
  }
  async function refreshMoveCopy() {
    if (!fm.curFs.value) return
    fm.moveCopyLoading.value = true
    try {
      const all = await fileApi.files(fm.curFs.value.id, fm.moveCopyPath.value)
      fm.moveCopyEntries.value = all.filter((e) => e.is_dir)
    } catch {
      fm.moveCopyEntries.value = []
    } finally {
      fm.moveCopyLoading.value = false
    }
  }
  function moveCopyEnter(e: FileEntry) {
    fm.moveCopyPath.value = e.path
    refreshMoveCopy()
  }
  function moveCopyUp() {
    const parts = fm.moveCopyPath.value.replace(/\/$/, '').split('/').filter(Boolean)
    parts.pop()
    fm.moveCopyPath.value = '/' + parts.join('/')
    if (fm.moveCopyPath.value !== '/') fm.moveCopyPath.value += '/'
    refreshMoveCopy()
  }
  async function doMoveCopy() {
    if (!fm.curFs.value || !fm.moveCopySources.value.length || fm.submitting.value) return
    // 目标目录同名覆盖确认：先取目标已有文件名，源 basename 命中则弹确认（后端静默覆盖会丢数据）。
    // 列目录失败不再当「预检通过」：读不到目标就等于无法确认同名，交给用户决定。
    try {
      const targetEntries = await fileApi.files(fm.curFs.value.id, fm.moveCopyPath.value)
      const existing = new Set(targetEntries.map((e) => e.name))
      const conflicts = fm.moveCopySources.value.filter((s) => existing.has(s.name))
      if (conflicts.length) {
        try {
          await ElMessageBox.confirm(
            `目标存在同名 ${conflicts.length} 项（如「${conflicts[0].name}」），${fm.moveCopyMode.value === 'move' ? t('opusefileops.k12') : t('opusefileops.k13')}将覆盖，是否继续？`,
            t('opusefileops.k4'),
            { type: 'warning' },
          )
        } catch {
          return // 用户取消覆盖
        }
      }
    } catch {
      // 读不到目标 ≠ 没有同名项（后端 move/copy 是同名静默覆盖）：这份不确定要交给用户决定，
      // 从前这里当作「预检通过」直接放行，用户毫不知情就把同名文件盖掉了。
      const n = targetReadFailedNotice(fm.moveCopyMode.value, fm.moveCopySources.value.length)
      const go = await ElMessageBox.confirm(n.body, n.title, { type: 'warning' }).catch(() => false)
      if (!go) return
    }
    const fn = fm.moveCopyMode.value === 'move' ? fileApi.move : fileApi.copy
    const verb = fm.moveCopyMode.value === 'move' ? '移动' : '复制'
    fm.submitting.value = true
    let ok = 0
    try {
      for (const e of fm.moveCopySources.value) {
        try {
          await fn(fm.curFs.value.id, e.path, fm.moveCopyPath.value)
          ok++
        } catch (err) {
          ElMessage.error(`「${e.name}」${verb}失败：${(err as Error).message}`)
        }
      }
      // 全部失败时不弹 success（避免「已移动 0 项」误导）；至少一项成功才关对话框并刷新。
      if (ok > 0) {
        ElMessage.success(`已${verb} ${ok} 项`)
        fm.moveCopyVisible.value = false
        await refresh()
      } else {
        ElMessage.error(`选中项全部${verb}失败`)
      }
    } finally {
      fm.submitting.value = false
    }
  }

  // ── STRM 即时生成（仅云盘目录）──
  function openStrm(e: FileEntry) {
    fm.strmDir.value = e
    fm.strmForm.value = { output_dir: '', mount_root: '', template: '', recursive: true, keep_ext: true }
    fm.strmVisible.value = true
  }
  async function doStrm() {
    if (!fm.curCloud.value || !fm.strmDir.value || !fm.strmForm.value.output_dir)
      return ElMessage.warning(t('opusefileops.k14'))
    fm.strmLoading.value = true
    try {
      const r = await cloudApi.generate({
        drive_id: fm.curCloud.value.id,
        cloud_path: fm.strmDir.value.path,
        output_dir: fm.strmForm.value.output_dir,
        mount_root: fm.strmForm.value.mount_root,
        template: fm.strmForm.value.template || undefined,
        recursive: fm.strmForm.value.recursive,
        keep_ext: fm.strmForm.value.keep_ext,
      })
      ElMessage.success(
        t('opusefileops.strmDone', { gen: r.generated, skip: r.skipped, errs: r.errors.length ? t('opusefileops.strmErrs', { n: r.errors.length }) : '' }),
      )
      fm.strmVisible.value = false
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('opusefileops.k15'))
    } finally {
      fm.strmLoading.value = false
    }
  }

  // ── 存储后端 CRUD ──
  function openCreate() {
    fm.editingId.value = null
    fm.form.value = { name: '', type: 'webdav', url: '', username: '', password: '', base_path: '/', agent_id: 0 }
    fm.storageDialog.value = true
  }
  function openEdit(fsItem: FileStorage) {
    fm.editingId.value = fsItem.id
    fm.form.value = {
      name: fsItem.name,
      type: fsItem.type,
      url: fsItem.url,
      username: fsItem.username,
      password: '',
      base_path: fsItem.base_path || '/',
      agent_id: fsItem.agent_id || 0,
    }
    fm.storageDialog.value = true
  }
  async function submit() {
    if (fm.submitting.value) return
    if (!fm.form.value.name) return ElMessage.warning(t('opusefileops.k16'))
    if (fm.form.value.type === 'agent' && !fm.form.value.agent_id) return ElMessage.warning(t('opusefileops.k17'))
    if (fm.form.value.type === 'rclone' && !fm.form.value.username) return ElMessage.warning(t('opusefileops.k18'))
    if (['webdav', 'alist', 'smb'].includes(fm.form.value.type) && !fm.form.value.url)
      return ElMessage.warning(t('opusefileops.k19'))
    fm.submitting.value = true
    try {
      if (fm.editingId.value) {
        await fileApi.update(fm.editingId.value, fm.form.value)
        ElMessage.success(t('common.updated'))
      } else {
        await fileApi.create(fm.form.value)
        ElMessage.success(t('common.added'))
      }
      fm.storageDialog.value = false
      await loadStorages()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('common.failed'))
    } finally {
      fm.submitting.value = false
    }
  }
  async function delStorage(fsItem: FileStorage) {
    try {
      await ElMessageBox.confirm(`确认删除存储「${fsItem.name}」？仅移除配置，不影响远端文件。`, t('opusefileops.k20'), {
        type: 'warning',
      })
    } catch {
      return
    }
    try {
      await fileApi.remove(fsItem.id)
      ElMessage.success(t('common.deleted'))
      if (fm.current.value?.kind === 'fs' && fm.current.value.fs.id === fsItem.id) fm.current.value = null
      await loadStorages()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('common.deleteFailed'))
    }
  }

  // 删除正在编辑的存储（对话框内按钮；find 后需 narrow kind 才能取 .fs）。
  function delEditingStorage() {
    const found = fm.list.value.find((x) => x.kind === 'fs' && x.fs.id === fm.editingId.value)
    if (found && found.kind === 'fs') {
      delStorage(found.fs)
      fm.storageDialog.value = false
    }
  }

  // 存储项 ⋮ 菜单：测试连接 / 编辑 / 删除。云盘（cloud）的编辑/删除在「云盘」页，此处禁用。
  function onStorageMenu(cmd: string, it: StorageItem) {
    if (cmd === 'test') {
      fm.testOne(it)
    } else if (cmd === 'edit' && it.kind === 'fs') {
      openEdit(it.fs)
    } else if (cmd === 'delete' && it.kind === 'fs') {
      delStorage(it.fs)
    }
  }

  function toggleView() {
    fm.viewMode.value = fm.viewMode.value === 'list' ? 'grid' : 'list'
    localStorage.setItem('fm.view', fm.viewMode.value)
    fm.selected.value = []
  }
  function toggleSelect(e: FileEntry) {
    const i = fm.selected.value.findIndex((s) => s.path === e.path)
    if (i >= 0) fm.selected.value.splice(i, 1)
    else fm.selected.value.push(e)
  }
  function isSelected(e: FileEntry) {
    return fm.selected.value.some((s) => s.path === e.path)
  }

  function fmtBytes(n: number) {
    if (!n) return '-'
    const u = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
    let i = 0
    while (n >= 1024 && i < u.length - 1) {
      n /= 1024
      i++
    }
    return `${n.toFixed(1)} ${u[i]}`
  }
  function itemIcon(it: StorageItem) {
    if (it.kind === 'cloud') return '☁'
    return { local: '📁', agent: '🖥', webdav: '🌐', alist: '🗂', smb: '💻', rclone: '🔌' }[it.fs.type] || '💾'
  }
  function itemLabel(it: StorageItem) {
    return it.kind === 'fs' ? it.fs.type : it.drive.type
  }

  return {
    doMkdir,
    handleUpload,
    openRename,
    doRename,
    delOne,
    delBatch,
    cancelBatchDel,
    openMoveCopy,
    refreshMoveCopy,
    moveCopyEnter,
    moveCopyUp,
    doMoveCopy,
    openStrm,
    doStrm,
    openCreate,
    openEdit,
    submit,
    delStorage,
    delEditingStorage,
    onStorageMenu,
    toggleView,
    toggleSelect,
    isSelected,
    fmtBytes,
    itemIcon,
    itemLabel,
  }
}
