#Requires -Version 5.1
<#
.SYNOPSIS
    PTPatronus 鸿蒙（ohos/HarmonyOS）构建脚本。

.DESCRIPTION
    一次鸿蒙构建 = 切换 → vendor → 构建 → 无条件复原：

    1. 依赖切换：pubspec_overrides.ohos.yaml → pubspec_overrides.yaml
       （pub 官方机制，鸿蒙专用 CPF-Flutter 插件分支只在此刻生效；
       该文件只允许 dependency_overrides 字段）。
    2. file_picker 切换（两处）：
       a. 实现文件：file_picker_ohos_impl.dart 的内容临时替换
          file_picker_default.dart（两文件实现类同名 PlatformFilePickerImpl，
          门面与业务代码零改动）。
       b. 专用依赖：file_picker_ohos 追加进主 pubspec.yaml 的 dependencies 段
          （pubspec_overrides.yaml 不支持新增依赖，见上）。
    3. flutter pub get（ohos SDK 解析出 9 个 ohos 插件）。
    4. vendor（scripts/vendor_ohos_plugins.ps1）：把每个插件的 ohos/ 目录
       从 pub 缓存复制到 mobile/ohos/plugins/<name>/ohos——hvigor 6.x 的
       ArkTS 编译器无法为项目根之外的源码生成 OhmUrl（踩坑 #9），
       配合 ohos fork TS 插件里的 vendored 重定向与 ohos/build-profile.json5
       里手工声明的插件 modules（见 docs/鸿蒙端完善.md §4）。
    5. 用 ohos fork Flutter SDK（默认 D:\SDK\flutter_ohos_341）构建 HAP。
    6. 复原 1、2（finally 保证，中断/失败也不留脏工作区）。
       ohos/plugins/ 与 .flutter-plugins-dependencies 的 vendored 改写保留
       （构建产物，gitignore 排除；下次构建的 vendor 步骤会刷新）。

    产物：mobile/ohos/entry/build/default/outputs/default/entry-default-unsigned.hap
    签名：见 mobile/ohos/signing/sign-official-hap.ps1（构建后单独执行）。

.PARAMETER FlutterSdk
    ohos fork Flutter SDK 路径（其 bin 目录的父目录）。默认 D:\SDK\flutter_ohos_341。

.PARAMETER SkipPubGet
    跳过 flutter pub get（调试用，默认每次都取依赖确保 overrides 生效）。

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File scripts\build_ohos.ps1
#>
param(
    [string]$FlutterSdk = 'D:\SDK\flutter_ohos_341',
    # HOS_SDK_HOME / DEVECO_SDK_HOME：fork 工具链定位 HarmonyOS SDK。
    # 优先级：flutter 全局 config ohos-sdk > DEVECO_SDK_HOME > HOS_SDK_HOME。
    # 目录要求（ohos_sdk.dart initSdkVersionMap）：一级子目录含 sdk-pkg.json
    # （apiVersion=数字）。当前用 DevEco 6.1.1(API 24)：D:\SDK\harmonyos-sdk\
    # HarmonyOS-6.1.1（目录名必须等于 sdk-pkg.json 的 data.path，hvigor 6 的
    # HosLocalComponentLoader 以此校验位置）。API 18 不可用：引擎 har 引用
    # GlobalWindowMode/isCapsLockOn 等新 API（踩坑 #10）。
    # 注意：sdk-pkg.json 带 UTF-8 BOM 会令 JSON5 解析失败（华为包常见坑）。
    [string]$HosSdkHome = 'D:\SDK\harmonyos-sdk',
    # ohpm（鸿蒙包管理器，hvigor 装依赖用）所在 bin 目录；需含 ohpm.bat。
    [string]$OhpmBin = 'D:\SDK\ohos-commandline\oh-command-line-tools\ohpm\bin',
    [switch]$SkipPubGet
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$RepoRoot = Split-Path -Parent $PSScriptRoot
$MobileDir = Join-Path $RepoRoot 'mobile'
$OverridesSrc = Join-Path $MobileDir 'pubspec_overrides.ohos.yaml'
$OverridesDst = Join-Path $MobileDir 'pubspec_overrides.yaml'
$Pubspec = Join-Path $MobileDir 'pubspec.yaml'
$PubspecBackup = "$Pubspec.bak"
$FpDefault = Join-Path $MobileDir 'lib\core\platform\file_picker_default.dart'
$FpOhos = Join-Path $MobileDir 'lib\core\platform\file_picker_ohos_impl.dart'
$FpBackup = "$FpDefault.bak"

# pubspec_overrides.yaml 不支持 dependencies 字段，鸿蒙专用新增依赖
# （file_picker_ohos）以锚点注释方式追加进主 pubspec.yaml 的 dependencies
# 段内（dev_dependencies: 行之前的空行处，保持 2 空格缩进同层），构建后整段移除。
$OhosDepsMarker = '# OHOS-ONLY-DEPS:begin（scripts/build_ohos.ps1 构建期追加，勿手编）'
$OhosDepsBlock = @"
  $OhosDepsMarker
  file_picker_ohos:
    git:
      url: https://gitcode.com/CPF-Flutter/fluttertpc_file_picker.git
      ref: br_v10.3.8_ohos
  # OHOS-ONLY-DEPS:end
"@

function Assert-Path([string]$Path, [string]$What) {
    if (-not (Test-Path -LiteralPath $Path)) {
        throw "缺少 $What：$Path（详见 docs/鸿蒙端完善.md）"
    }
}

Assert-Path $FlutterSdk 'ohos fork Flutter SDK（-FlutterSdk 指定）'
Assert-Path (Join-Path $FlutterSdk 'bin\flutter.bat') 'ohos fork flutter.bat'
Assert-Path $HosSdkHome 'HarmonyOS SDK（-HosSdkHome 指定，需含 <版本>/sdk-pkg.json）'
Assert-Path $OhpmBin 'ohpm bin 目录（-OhpmBin 指定，需含 ohpm.bat）'
Assert-Path (Join-Path $OhpmBin 'ohpm.bat') 'ohpm.bat'
Assert-Path $OverridesSrc '鸿蒙依赖覆盖文件'
Assert-Path $FpOhos '鸿蒙 file_picker 实现'

$FlutterBat = Join-Path $FlutterSdk 'bin\flutter.bat'
$restored = $false

function Restore-BuildSwitches {
    if ($restored) { return }
    $restored = $true
    if (Test-Path -LiteralPath $FpBackup) {
        Move-Item -LiteralPath $FpBackup -Destination $FpDefault -Force
        Write-Host '[复原] file_picker_default.dart'
    }
    if (Test-Path -LiteralPath $PubspecBackup) {
        Move-Item -LiteralPath $PubspecBackup -Destination $Pubspec -Force
        Write-Host '[复原] pubspec.yaml'
    }
    if (Test-Path -LiteralPath $OverridesDst) {
        Remove-Item -LiteralPath $OverridesDst -Force
        Write-Host '[复原] pubspec_overrides.yaml（已删除）'
    }
}

function Add-OhosDeps {
    # 锚定 "dev_dependencies:" 行，把鸿蒙专用依赖块（2 空格缩进，与
    # dependencies 段同层）插到它前面的空行之后——dependencies 与
    # dev_dependencies 是同级 key，块内容缩进 2 格即归属 dependencies。
    # 读写字节细节：UTF-8 显式解码→按行拆分→插块→原行尾回写，全程
    # UTF-8 无 BOM；不用 Get-Content/WriteAllLines（默认编码/行尾会被
    # 改写，pub 的 YAML 解析器对 BOM 敏感）。
    $text = [System.IO.File]::ReadAllText($Pubspec, [System.Text.UTF8Encoding]::new($false))
    $eol = if ($text -match "`r`n") { "`r`n" } else { "`n" }
    $lines = [System.Collections.Generic.List[string]]($text -split "`r?`n", -1)
    $anchor = $lines.IndexOf('dev_dependencies:')
    if ($anchor -lt 0) { throw 'pubspec.yaml 中找不到 dev_dependencies: 锚点' }
    Copy-Item -LiteralPath $Pubspec -Destination $PubspecBackup -Force
    $block = [string[]]($OhosDepsBlock -split "`r?`n")
    # 插入点：dev_dependencies 前若紧邻空行，插到空行之前（保持段间空行）
    $insertAt = $anchor
    if ($insertAt -gt 0 -and $lines[$insertAt - 1] -eq '') { $insertAt-- }
    $lines.InsertRange($insertAt, $block)
    $out = ($lines -join $eol)
    [System.IO.File]::WriteAllText($Pubspec, $out, [System.Text.UTF8Encoding]::new($false))
}

try {
    # ---- 切换：依赖 overrides + file_picker 实现/依赖 ----
    Copy-Item -LiteralPath $OverridesSrc -Destination $OverridesDst -Force
    Write-Host '[切换] pubspec_overrides.yaml ← pubspec_overrides.ohos.yaml'

    Copy-Item -LiteralPath $FpDefault -Destination $FpBackup -Force
    Copy-Item -LiteralPath $FpOhos -Destination $FpDefault -Force
    Write-Host '[切换] file_picker_default.dart ← file_picker_ohos_impl.dart（原实现存 .bak）'

    Add-OhosDeps
    Write-Host '[切换] pubspec.yaml += file_picker_ohos（原文件存 .bak）'

    Push-Location $MobileDir
    try {
        # 依赖解析放在切换之后：pubspec_overrides.yaml 与追加依赖必须已就位。
        # ohos fork SDK 与官方 SDK 的 pub 缓存共享，pub get 会把 lockfile
        # 切到 git 源；复原后下一次官方构建的 pub get 会自动切回，属预期。
        # 注意：不直接 `& $FlutterBat ... 2>&1 | 管道` —— Windows PowerShell
        # 5.1 把原 stderr 行包装成 ErrorRecord 顺管道传递时，flutter 工具
        # 的进度输出会触发 "Unhandled exception"（实测复现）；改用 cmd /c
        # 在子进程内合并输出、整体字符串数组返回，稳定。
        if (-not $SkipPubGet) {
            Write-Host '[构建] flutter pub get（ohos 依赖）'
            $pubOut = & cmd /c "`"$FlutterBat`" pub get 2>&1"
            if ($LASTEXITCODE -ne 0) { throw "pub get 失败：`n$($pubOut -join "`n")" }
        }

        # vendor：pub 缓存里的插件 ohos/ 复制进项目 + 重写插件路径。
        # 必须在 pub get 之后（生成 .flutter-plugins-dependencies）、build 之前。
        # 注意：vendor 脚本内部用 robocopy（成功码 0-7），它的非零退出码会
        # 污染 $LASTEXITCODE；此处用 $?（脚本是否正常返回）判定，并在进入
        # 下一步前清零，避免干扰后续 flutter 命令的退出码判断。
        Write-Host '[vendor] 复制 ohos 插件到 mobile/ohos/plugins/'
        $vendorScript = Join-Path $PSScriptRoot 'vendor_ohos_plugins.ps1'
        $vendorOk = & $vendorScript -MobileDir $MobileDir
        if (-not $?) { throw 'vendor 步骤失败' }
        $global:LASTEXITCODE = 0

        Write-Host '[构建] flutter build hap --release'
        # 三路 SDK 定位保持一致（config > DEVECO_SDK_HOME > HOS_SDK_HOME），
        # 且 ohpm/hvigor 需在 PATH（hvigorw 装依赖时调用 ohpm）。
        $env:HOS_SDK_HOME = $HosSdkHome
        $env:DEVECO_SDK_HOME = $HosSdkHome
        $env:PATH = "$OhpmBin;$env:PATH"
        $configOut = & cmd /c "`"$FlutterBat`" config --ohos-sdk $HosSdkHome 2>&1"
        $buildStart = Get-Date
        $buildOut = & cmd /c "`"$FlutterBat`" build hap --release 2>&1"
        $buildExit = $LASTEXITCODE

        # 判定标准用「HAP 是否在本次构建窗口内新鲜产出」而非退出码：hvigor 成功
        # 末尾会打印「请配置调试签名」提示（正常流程），但 flutter.bat 经
        # cmd /c 包装后退出码偶发误报 1（Windows 批处理 errorlevel 传递怪癖，
        # 实测 HAP 已产出）。产物新鲜度 + 存在性是唯一可信信号。
        $hap = Join-Path $MobileDir 'ohos\entry\build\default\outputs\default\entry-default-unsigned.hap'
        $hapFresh = (Test-Path -LiteralPath $hap) -and ((Get-Item -LiteralPath $hap).LastWriteTime -ge $buildStart.AddSeconds(-5))
        if (-not $hapFresh) {
            throw "build hap 失败（exit=$buildExit，无新鲜 HAP 产出）：`n$($buildOut | Select-Object -Last 30 | Out-String)"
        }
        $buildOut | Select-Object -Last 5 | ForEach-Object { Write-Host "  $_" }

        Write-Host "[完成] 未签名 HAP：$hap" -ForegroundColor Green
        Write-Host '       下一步签名：mobile/ohos/signing/sign-official-hap.ps1'
    }
    finally {
        Pop-Location
    }
}
finally {
    Restore-BuildSwitches
    Write-Host '[复原] 工作区已还原为官方 SDK 状态（pubspec.lock 的 git 源残留会在下次 pub get 自动切回）'
}
