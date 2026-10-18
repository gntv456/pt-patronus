// useTransfer 的「目录标签/排序/目录同步/识别词/识别词云端共享」子模块。
// （从 useTransfer.ts 拆出；自含全部状态与方法，行为不变。）
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { transferApi, type DirTag, type SortRules, type DirSyncTask, type IdentifyWord, type SharedRepo } from '@/api/transfer'

export function useTransferRules() {
  // —— 目录标签 + 排序规则 + 目录同步 ——
  const showTags = ref(false)
  const dirTags = ref<DirTag[]>([])
  const sortRules = ref<SortRules>({
    resolution: { '2160p': 100, '1080p': 70, '720p': 30, unknown: 0 },
    group_bonus: {},
    target_size_gb: 20,
    seeders_weight: 5,
    free_bonus: 10,
    free2x_bonus: 15,
    recent_days: 7,
    recent_bonus: 5,
  })
  const tagsSaving = ref(false)
  async function openTags() {
    try {
      const [tags, rules] = await Promise.all([transferApi.getDirTags(), transferApi.getSortRules()])
      dirTags.value = tags
      sortRules.value = { ...sortRules.value, ...rules }
    } catch {
      /* 默认 */
    }
    showTags.value = true
  }
  async function saveTagsAndRules() {
    tagsSaving.value = true
    try {
      await transferApi.setDirTags(dirTags.value.filter((t) => t.name.trim() && t.path.trim()))
      await transferApi.setSortRules(sortRules.value)
      ElMessage.success('标签与排序规则已保存')
      showTags.value = false
    } catch (e) {
      ElMessage.error((e as Error).message || '保存失败')
    } finally {
      tagsSaving.value = false
    }
  }

  const showDirSync = ref(false)
  const syncTasks = ref<DirSyncTask[]>([])
  const syncSaving = ref(false)
  const syncingName = ref('')
  async function openDirSync() {
    try {
      syncTasks.value = await transferApi.getDirSync()
    } catch {
      syncTasks.value = []
    }
    showDirSync.value = true
  }
  function addSyncTask() {
    syncTasks.value.push({ name: '', src_dir: '', dst_dir: '', mode: 'link', recursive: true, enabled: true })
  }
  async function saveSyncTasks() {
    syncSaving.value = true
    try {
      await transferApi.setDirSync(syncTasks.value)
      ElMessage.success('同步任务已保存')
      showDirSync.value = false
    } catch (e) {
      ElMessage.error((e as Error).message || '保存失败')
    } finally {
      syncSaving.value = false
    }
  }
  async function runSync(t: DirSyncTask) {
    syncingName.value = t.name
    try {
      const r = await transferApi.runDirSync(t.name)
      ElMessage.success('同步完成：扫描 ' + r.scanned + ' 同步 ' + r.synced + ' 跳过 ' + r.skipped)
    } catch (e) {
      ElMessage.error((e as Error).message || '运行失败')
    } finally {
      syncingName.value = ''
    }
  }

  // —— 识别词（本地自定义：整理识别前规整名称）——
  const showIdentify = ref(false)
  const identifyWords = ref<IdentifyWord[]>([])
  const identifySaving = ref(false)
  const identifyTestName = ref('')
  const identifyTestResult = ref('')
  async function openIdentify() {
    try {
      identifyWords.value = await transferApi.getIdentifyWords()
    } catch {
      identifyWords.value = []
    }
    loadSharedRepos() // 共享仓库列表随对话框加载（失败静默→空列表）
    identifyTestName.value = ''
    identifyTestResult.value = ''
    showIdentify.value = true
  }
  function addIdentifyWord() {
    identifyWords.value.push({ kind: 'replace', from: '', to: '', note: '' })
  }
  function removeIdentifyWord(i: number) {
    identifyWords.value.splice(i, 1)
  }
  async function saveIdentify() {
    identifySaving.value = true
    try {
      await transferApi.setIdentifyWords(identifyWords.value)
      ElMessage.success('识别词已保存（即时生效）')
      showIdentify.value = false
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '保存失败')
    } finally {
      identifySaving.value = false
    }
  }
  async function testIdentify() {
    if (!identifyTestName.value.trim()) return
    try {
      const r = await transferApi.testIdentifyWords(identifyTestName.value.trim())
      identifyTestResult.value = `规整后：${r.normalized} → 标题「${r.title}」${r.season ? ' ' + r.season : ''}${r.episode ? ' ' + r.episode : ''}`
    } catch (e: unknown) {
      identifyTestResult.value = (e as Error).message || '测试失败'
    }
  }

  // —— 名称识别测试（对标 MSaber）：本地元数据 + 媒体源匹配一步到位 ——
  const recognizeTesting = ref(false)
  async function testRecognize() {
    if (!identifyTestName.value.trim()) return
    recognizeTesting.value = true
    try {
      const r = await transferApi.testRecognize(identifyTestName.value.trim(), 0, true)
      // 「媒体源挂了」与「库里真没这部片」分开：前者提示的是源/网络问题，
      // 后者才是要补识别词或换名——混成一句会让人照着错的方向改文件名。
      const mediaPart = r.media_error
        ? `⚠️ 媒体源查询失败：${r.media_error}`
        : r.matched
          ? `✅ 媒体源匹配：${r.matched.title}${r.matched.year ? `(${r.matched.year})` : ''}`
          : '⚠️ 媒体源未匹配（可补识别词或换名）'
      const parts = [
        `规整后：${r.normalized}`,
        `标题「${r.title}」${r.year ? `(${r.year})` : ''}`,
        r.is_tv
          ? `剧集 ${r.season ? 'S' + String(r.season).padStart(2, '0') : ''}${r.episode ? 'E' + String(r.episode).padStart(2, '0') : ''}`
          : '电影',
        r.resolution ? `分辨率 ${r.resolution}` : '',
        r.group ? `发布组 ${r.group}` : '',
        mediaPart,
      ].filter(Boolean)
      identifyTestResult.value = parts.join(' → ')
    } catch (e: unknown) {
      identifyTestResult.value = (e as Error).message || '识别测试失败'
    } finally {
      recognizeTesting.value = false
    }
  }

  // —— 识别词云端共享（仓库列表 + 预览 + 一键导入；本地优先不覆盖） ——
  const sharedRepos = ref<SharedRepo[]>([])
  const shareImporting = ref(false)
  const sharePreview = ref<{ total: number; new: number } | null>(null)
  async function loadSharedRepos() {
    try {
      sharedRepos.value = await transferApi.getSharedRepos()
    } catch {
      sharedRepos.value = []
    }
  }
  function addSharedRepo() {
    sharedRepos.value.push({ name: '', url: '' })
  }
  function removeSharedRepo(i: number) {
    sharedRepos.value.splice(i, 1)
  }
  async function saveSharedRepos() {
    try {
      await transferApi.setSharedRepos(sharedRepos.value.filter((r) => r.url))
      ElMessage.success('共享仓库已保存')
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '保存失败')
    }
  }
  async function importSharedRepo(url: string) {
    if (!url) return
    shareImporting.value = true
    sharePreview.value = null
    try {
      const p = await transferApi.previewSharedRepo(url)
      sharePreview.value = { total: p.total, new: p.new }
      if (p.new === 0) {
        ElMessage.info(`仓库 ${p.total} 条均已存在，无需导入`)
        return
      }
      const r = await transferApi.importSharedRepo(url)
      ElMessage.success(`已导入 ${r.added} 条（共 ${r.total} 条）`)
      identifyWords.value = await transferApi.getIdentifyWords()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '导入失败')
    } finally {
      shareImporting.value = false
    }
  }

  return {
    showTags,
    dirTags,
    sortRules,
    tagsSaving,
    openTags,
    saveTagsAndRules,
    showDirSync,
    syncTasks,
    syncSaving,
    syncingName,
    openDirSync,
    addSyncTask,
    saveSyncTasks,
    runSync,
    showIdentify,
    identifyWords,
    identifySaving,
    identifyTestName,
    identifyTestResult,
    recognizeTesting,
    openIdentify,
    addIdentifyWord,
    removeIdentifyWord,
    saveIdentify,
    testIdentify,
    testRecognize,
    sharedRepos,
    shareImporting,
    sharePreview,
    loadSharedRepos,
    addSharedRepo,
    removeSharedRepo,
    saveSharedRepos,
    importSharedRepo,
  }
}
