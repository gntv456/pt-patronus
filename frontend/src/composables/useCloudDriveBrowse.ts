import { ref, type Ref } from 'vue'
import { cloudApi, type CloudDrive, type CloudFile } from '@/api/cloud'

export function useCloudDriveBrowse() {
  const showBrowse = ref(false)
  const browseDrive = ref<CloudDrive | null>(null)
  const browsePath = ref('/')
  const browseFiles = ref<CloudFile[]>([])
  const browseLoading = ref(false)
  const browseTarget = ref<((p: string) => void) | null>(null)

  async function fetchBrowse() {
    if (!browseDrive.value) return
    browseLoading.value = true
    try {
      browseFiles.value = await cloudApi.browse(browseDrive.value.id, browsePath.value)
    } catch (e: any) {
      ElMessage.error(e?.message || '浏览失败')
      browseFiles.value = []
    } finally {
      browseLoading.value = false
    }
  }

  async function openBrowse(d: CloudDrive, target: (p: string) => void) {
    browseDrive.value = d
    browseTarget.value = target
    browsePath.value = d.root_path || '/'
    showBrowse.value = true
    await fetchBrowse()
  }

  function enterDir(f: CloudFile) {
    if (!f.is_dir) return
    browsePath.value = f.path
    fetchBrowse()
  }

  function browseUp() {
    if (browsePath.value === '/') return
    const parts = browsePath.value.split('/').filter(Boolean)
    parts.pop()
    browsePath.value = '/' + parts.join('/')
    fetchBrowse()
  }

  function pickDir() {
    if (browseTarget.value) browseTarget.value(browsePath.value)
    showBrowse.value = false
  }

  return {
    showBrowse,
    browseDrive,
    browsePath,
    browseFiles,
    browseLoading,
    openBrowse,
    fetchBrowse,
    enterDir,
    browseUp,
    pickDir,
  }
}

export function fmtSize(n: number) {
  if (!n) return '-'
  const g = 1 << 30
  if (n >= g) return (n / g).toFixed(1) + ' GB'
  if (n >= 1 << 20) return (n / (1 << 20)).toFixed(1) + ' MB'
  return (n / (1 << 10)).toFixed(1) + ' KB'
}
