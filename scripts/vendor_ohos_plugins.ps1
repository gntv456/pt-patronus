#Requires -Version 5.1
<#
.SYNOPSIS
    鸿蒙 ohos 插件本地化（vendor）脚本：build_ohos.ps1 在 pub get 之后、构建之前调用。

.DESCRIPTION
    背景（docs/鸿蒙端完善.md 踩坑 #9）：hvigor 6.x 的 ArkTS 编译器（rollup + es2abc）
    无法为「项目根之外」的源码生成 OhmUrl —— 插件 ohos/ 目录留在 pub 缓存
    （C:\Users\...\Pub\Cache\git\...）时，CompileArkTS 阶段必然报
    "Failed to resolve OhmUrl" / sourcemap 崩溃。3.41-ohos fork 的 TS hvigor 插件
    把插件以 file:<pub-cache>/ohos 的 overrides 挂给 entry，路径就在项目外。

    解法：构建前把每个 ohos 插件的 <plugin>/ohos 目录整体复制到
    mobile/ohos/plugins/<name>/ohos（项目内），并同步改写
    mobile/.flutter-plugins-dependencies 里 ohos 平台的 path 指向本地副本。
    flutter 工具链后续（GeneratedPluginRegistrant.ets 生成、TS 插件 overrides）
    都从这个文件取路径，因此一处改写全局生效。

    幂等：每次构建重新复制（插件版本变更时自动同步）。
    ohos/plugins/ 已加入 mobile/.gitignore（构建产物，不入库）。
#>
param(
    [string]$MobileDir = (Join-Path (Split-Path -Parent $PSScriptRoot) 'mobile')
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$PluginsDir = Join-Path $MobileDir 'ohos\plugins'
$DepsFile = Join-Path $MobileDir '.flutter-plugins-dependencies'

if (-not (Test-Path -LiteralPath $DepsFile)) {
    throw "缺少 $DepsFile（先在鸿蒙构建状态跑 flutter pub get）"
}

# 解析 JSON（PowerShell 5.1 的 ConvertFrom-Json 无压力，文件 < 1MB）
$json = Get-Content -LiteralPath $DepsFile -Raw -Encoding UTF8 | ConvertFrom-Json
$ohosPlugins = @($json.plugins.ohos)
if ($ohosPlugins.Count -eq 0) { Write-Host '[vendor] 无 ohos 插件，跳过'; return }

New-Item -ItemType Directory -Force -Path $PluginsDir | Out-Null
$changed = $false

foreach ($p in $ohosPlugins) {
    $src = Join-Path $p.path 'ohos'
    if (-not (Test-Path -LiteralPath $src)) { continue }
    $dstRoot = Join-Path $PluginsDir $p.name
    $dst = Join-Path $dstRoot 'ohos'
    if (Test-Path -LiteralPath $dst) { Remove-Item -LiteralPath $dst -Recurse -Force }
    New-Item -ItemType Directory -Force -Path $dstRoot | Out-Null
    # robocopy 比 Copy-Item 快且保留结构；/E 全量 /NFL /NDL /NJH /NJS 静默
    # /XJ 跳过 junction/symlink（pub git 缓存里 ohos/oh_modules 是上次 ohpm
    # install 留下的链接，跟进去复制会因链接目标权限失败 exit>=8）。
    & robocopy $src $dst /E /XJ /NFL /NDL /NJH /NJS /NP | Out-Null
    if ($LASTEXITCODE -ge 8) { throw "复制失败：$src -> $dst" }
    # hvigor 的受限模块解析无法从 vendored 位置 require '@ohos/hvigor-ohos-plugin'
    # （原版 hvigorfile 的裸 re-export 会 Script Error），统一改写成「从 hvigor
    # 运行时已加载的 @ohos/hvigor 定位其 workspace」的技巧——与项目根
    # hvigorfile.ts 的 requireCachedOhosPackage 同款。每次 vendor 都重写，
    # 保证插件升级换新后不会带回原版写法。
    $hvigorfile = Join-Path $dst 'hvigorfile.ts'
    $hfContent = @'
import path from 'path'

// PTPatronUS vendored build：从 hvigor 运行时已加载的 @ohos/hvigor 模块定位其
// workspace node_modules（与项目根 hvigorfile.ts 同款技巧），绕开 hvigor 对
// vendored 插件 hvigorfile 的受限模块解析。
function requireCachedOhosPackage(packageName: string) {
    const hvigorSegment = `${path.sep}@ohos${path.sep}hvigor${path.sep}`;
    const hvigorModule = Object.keys(require.cache)
        .map((it) => path.normalize(it))
        .find((it) => it.includes(hvigorSegment));
    if (!hvigorModule) {
        return require(packageName);
    }
    const workspaceNodeModules = hvigorModule.split(hvigorSegment)[0];
    return require(path.join(workspaceNodeModules, '@ohos', packageName.split('/')[1]));
}

export const harTasks = requireCachedOhosPackage('@ohos/hvigor-ohos-plugin').harTasks;
'@
    [System.IO.File]::WriteAllText($hvigorfile, $hfContent, [System.Text.UTF8Encoding]::new($false))
    # 改写 path 指向本地副本（转义供 JSON 用）
    $localPath = $dst
    $p.path = $localPath.Replace('\', '\\')
    $changed = $true
    Write-Host "[vendor] $($p.name) -> ohos\plugins\$($p.name)\ohos"
}

if ($changed) {
    $out = $json | ConvertTo-Json -Depth 20 -Compress
    [System.IO.File]::WriteAllText($DepsFile, $out, [System.Text.UTF8Encoding]::new($false))
    Write-Host '[vendor] .flutter-plugins-dependencies 已改写为本地插件路径'
}
