// 发版后同步 OTA update-manifest.json：latestVersion + docker image tag + 本地产物 sha256。
// android/windows/agent 的 sha256 从 SHA256SUMS.txt 自动读；ios/macos 需把 Mac 上
// 构建的产物拷进 dist 目录（iOS 未签名 ipa / macOS zip）才会回填。
// url 仍需手动填网盘直链（脚本只自动填版本号 + 本地算的 sha256），填完上传到
// UPDATE_MANIFEST_URL 指向的公开地址（gist raw / 轻量仓库 raw / 静态站）。
// 用法：node scripts/update-manifest.mjs <version> <distDir>
import { readFileSync, writeFileSync } from 'node:fs'

const version = process.argv[2]
const distDir = process.argv[3]
if (!version || !distDir) {
  console.error('用法: node scripts/update-manifest.mjs <version> <distDir>')
  process.exit(1)
}

const manifest = JSON.parse(readFileSync('update-manifest.json', 'utf8'))
manifest.latestVersion = version

manifest.targets = manifest.targets || {}
manifest.targets.docker = {
  image: `ghcr.io/gntv456/pt-patronus:${version}`,
  instructions:
    (manifest.targets.docker && manifest.targets.docker.instructions) ||
    'docker compose pull && docker compose up -d',
}

// 从 SHA256SUMS.txt 读各产物 checksum
const sums = readFileSync(`${distDir}/SHA256SUMS.txt`, 'utf8').split('\n')
const sha = (name) => {
  const line = sums.find((s) => s.includes(name))
  return line ? line.split(/\s+/)[0] : ''
}
const fill = (key, file) => {
  manifest.targets[key] = manifest.targets[key] || {}
  if (sha(file)) manifest.targets[key].sha256 = sha(file)
}
fill('android', 'android-arm64')
fill('windows', 'windows-x64')
// iOS/macOS 客户端在 Mac 上构建（release-local.sh 不含），产物名遵循
// ptpatronus-<版本>-ios-unsigned.ipa / ptpatronus-<版本>-macos.zip，
// 手动拷进 dist 目录后同样自动回填 sha256；没有产物则跳过（保持原值）。
fill('ios', 'ios')
fill('macos', 'macos')
// 鸿蒙 hap 本地构建（scripts/build_ohos.ps1）+ 自签后拷进 dist 目录
fill('ohos', 'ohos-arm64')
fill('agent-linux-amd64', 'agent-linux-amd64')
fill('agent-linux-arm64', 'agent-linux-arm64')

writeFileSync('update-manifest.json', JSON.stringify(manifest, null, 2) + '\n')

// 发布前完整性校验：url 声明了下载渠道的 target 必须带 64 位 hex sha256
// （macos 等桌面端走外链手动安装，但校验不可省——OTA 客户端拿 sha256 做
// 防篡改依据，缺失即静默降级为无校验下载）。校验失败直接非零退出，
// 拦住"链接已发布但哈希未回填"的半成品 manifest。
const errors = []
let seenUrls = new Map() // url → target key（多端同链接检测）
for (const [key, t] of Object.entries(manifest.targets)) {
  if (key === 'docker') continue // 镜像完整性由 registry digest 保证，不走 sha256 字段
  const url = (t && t.url) || ''
  const sha = (t && t.sha256) || ''
  if (!url && !sha) continue // 未发布的 target 允许留空整体
  if (!/^[0-9a-f]{64}$/.test(sha)) errors.push(`targets.${key}: url 已填而 sha256 缺失/格式非法（len=${sha.length}）`)
  if (url.startsWith('PENDING_')) errors.push(`targets.${key}: url 仍是占位符（${url}）——发版前必须替换为产物直链`)
  if (url) {
    if (seenUrls.has(url)) {
      errors.push(`targets.${key} 与 targets.${seenUrls.get(url)} 使用同一 url（${url}）——网盘文件夹页会使 Android 应用内 OTA 判定（.apk 直链）失效，且无法按端独立回滚`)
    } else {
      seenUrls.set(url, key)
    }
  }
}
if (errors.length) {
  console.error('✗ update-manifest.json 完整性校验未通过：')
  for (const e of errors) console.error('  - ' + e)
  process.exit(1)
}
console.log(`✓ update-manifest.json: latestVersion=${version}, docker.image=:${version}, sha256 已填`)
console.log('  注意：targets.*.url 仍需手动填网盘直链，然后上传 manifest 到 UPDATE_MANIFEST_URL 公开地址')
