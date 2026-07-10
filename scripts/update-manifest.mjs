// 发版后同步 OTA update-manifest.json：latestVersion + docker image tag + 本地产物 sha256。
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
fill('agent-linux-amd64', 'agent-linux-amd64')
fill('agent-linux-arm64', 'agent-linux-arm64')

writeFileSync('update-manifest.json', JSON.stringify(manifest, null, 2) + '\n')
console.log(`✓ update-manifest.json: latestVersion=${version}, docker.image=:${version}, sha256 已填`)
console.log('  注意：targets.*.url 仍需手动填网盘直链，然后上传 manifest 到 UPDATE_MANIFEST_URL 公开地址')
