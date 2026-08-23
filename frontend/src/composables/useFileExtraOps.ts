// useFileExtraOps — 文件管理扩展操作（在线编辑/压缩/解压/权限/目录大小/预览）。
// 从 useFileOps 抽出，独立 composable，避免单文件超 400 行。复用同一 fm 状态对象。
import { ElMessage, ElMessageBox } from 'element-plus'
import { fileApi } from '@/api/filemanager'
import type { FileEntry } from '@/api/filemanager'
import type { useFileManager } from './useFileManager'

type FileManager = ReturnType<typeof useFileManager>

export function useFileExtraOps(fm: FileManager, refresh: () => void) {

  // ── 在线编辑 ──
  function openEditFile(e: FileEntry) {
    fm.editTarget.value = e
    fm.editContent.value = ''
    fm.editVisible.value = true
    fm.editLoading.value = true
    fileApi.readFile(fm.curFs.value!.id, e.path).then((c) => { fm.editContent.value = c }).catch((err) => {
      ElMessage.error(err.message || '读取失败'); fm.editVisible.value = false
    }).finally(() => { fm.editLoading.value = false })
  }
  async function doSaveFile() {
    if (!fm.editTarget.value) return
    fm.editLoading.value = true
    try {
      await fileApi.writeFile(fm.curFs.value!.id, fm.editTarget.value.path, fm.editContent.value)
      ElMessage.success('已保存')
      fm.editVisible.value = false
      refresh()
    } catch (e: unknown) { ElMessage.error((e as Error).message || '保存失败') }
    finally { fm.editLoading.value = false }
  }

  // ── 压缩 ──
  function openArchive(e: FileEntry) {
    fm.archiveTarget.value = e
    fm.archiveName.value = e.is_dir ? e.name + '.tar.gz' : e.name.replace(/\.[^.]+$/, '') + '.tar.gz'
    fm.archiveFmt.value = 'targz'
    fm.archiveVisible.value = true
  }
  async function doArchive() {
    if (!fm.archiveTarget.value || !fm.archiveName.value.trim()) return ElMessage.warning('请填压缩包名')
    fm.archiveLoading.value = true
    try {
      const dst = fm.path.value.replace(/\/$/, '') + '/' + fm.archiveName.value.trim()
      await fileApi.archive(fm.curFs.value!.id, fm.archiveTarget.value.path, dst)
      ElMessage.success('压缩完成')
      fm.archiveVisible.value = false
      refresh()
    } catch (e: unknown) { ElMessage.error((e as Error).message || '压缩失败') }
    finally { fm.archiveLoading.value = false }
  }

  // ── 解压 ──
  async function doExtract(e: FileEntry) {
    try {
      await ElMessageBox.confirm(`解压 ${e.name} 到当前目录？`, '解压', { type: 'info' })
    } catch { return }
    try {
      await fileApi.extract(fm.curFs.value!.id, e.path, fm.path.value)
      ElMessage.success('解压完成')
      refresh()
    } catch (err: unknown) { ElMessage.error((err as Error).message || '解压失败') }
  }

  // ── 权限 ──
  function openChmod(e: FileEntry) {
    fm.chmodTarget.value = e
    fm.chmodMode.value = '755'
    fm.chmodRecursive.value = false
    fm.chmodVisible.value = true
  }
  async function doChmod() {
    if (!fm.chmodTarget.value) return
    const oct = parseInt(fm.chmodMode.value, 8)
    if (isNaN(oct)) return ElMessage.warning('请填有效的八进制权限（0-777）')
    fm.chmodLoading.value = true
    try {
      await fileApi.chmod(fm.curFs.value!.id, fm.chmodTarget.value.path, oct, fm.chmodRecursive.value)
      ElMessage.success('权限已修改')
      fm.chmodVisible.value = false
    } catch (e: unknown) { ElMessage.error((e as Error).message || '修改失败') }
    finally { fm.chmodLoading.value = false }
  }

  // ── 目录大小 ──
  async function calcDirSize(e: FileEntry) {
    fm.dirSizeLoading.value = { ...fm.dirSizeLoading.value, [e.path]: true }
    try {
      const r = await fileApi.dirSize(fm.curFs.value!.id, e.path)
      fm.dirSizeMap.value = { ...fm.dirSizeMap.value, [e.path]: r.sizeStr }
    } catch (err: unknown) { ElMessage.error((err as Error).message || '计算失败') }
    finally { fm.dirSizeLoading.value = { ...fm.dirSizeLoading.value, [e.path]: false } }
  }

  // ── 文件预览（文本） ──
  function openPreview(e: FileEntry) {
    fm.previewTarget.value = e
    fm.previewContent.value = ''
    fm.previewVisible.value = true
    fm.previewLoading.value = true
    fileApi.readFile(fm.curFs.value!.id, e.path).then((c) => { fm.previewContent.value = c }).catch((err) => {
      ElMessage.error(err.message || '读取失败'); fm.previewVisible.value = false
    }).finally(() => { fm.previewLoading.value = false })
  }

  return { openEditFile, doSaveFile, openArchive, doArchive, doExtract, openChmod, doChmod, calcDirSize, openPreview }
}
