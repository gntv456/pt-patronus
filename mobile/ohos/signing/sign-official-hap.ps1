param(
    [string]$PropertiesFile,
    [string]$UnsignedHap,
    [string]$OutputDir
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$SigningDir = $PSScriptRoot
$OhosDir = Split-Path -Parent $SigningDir
$RepoRoot = Split-Path -Parent (Split-Path -Parent $OhosDir)

if (-not $PropertiesFile) {
    $PropertiesFile = Join-Path $SigningDir 'official\signing.properties'
}
if (-not $UnsignedHap) {
    $UnsignedHap = Join-Path $OhosDir 'entry\build\default\outputs\default\entry-default-unsigned.hap'
}
if (-not $OutputDir) {
    $OutputDir = Join-Path $RepoRoot 'dist'
}

function Read-PropertiesFile {
    param([string]$Path)

    if (-not (Test-Path -LiteralPath $Path)) {
        $importScript = Join-Path $SigningDir 'import-official-signing.ps1'
        throw "Missing signing properties: $Path. Import official DevEco/Huawei signing material first: powershell -ExecutionPolicy Bypass -File `"$importScript`" -SourceDir <export-dir>"
    }

    $props = @{}
    foreach ($line in Get-Content -LiteralPath $Path) {
        $trimmed = $line.Trim()
        if ($trimmed.Length -eq 0 -or $trimmed.StartsWith('#')) {
            continue
        }
        $separator = $trimmed.IndexOf('=')
        if ($separator -lt 1) {
            continue
        }
        $key = $trimmed.Substring(0, $separator).Trim()
        $value = $trimmed.Substring($separator + 1).Trim()
        $props[$key] = [Environment]::ExpandEnvironmentVariables($value)
    }
    return $props
}

function Get-RequiredValue {
    param(
        [hashtable]$Props,
        [string]$Key
    )

    if (-not $Props.ContainsKey($Key) -or [string]::IsNullOrWhiteSpace([string]$Props[$Key])) {
        throw "Missing required signing property: $Key"
    }
    return [string]$Props[$Key]
}

function Resolve-ConfiguredPath {
    param([string]$Path)

    $normalized = $Path -replace '\\\\', '\'
    if ([IO.Path]::IsPathRooted($normalized)) {
        return $normalized
    }
    return Join-Path (Split-Path -Parent $PropertiesFile) $normalized
}

function Get-PackInfo {
    $packInfoPath = Join-Path (Split-Path -Parent $UnsignedHap) 'pack.info'
    if (Test-Path -LiteralPath $packInfoPath) {
        return Get-Content -Raw -LiteralPath $packInfoPath | ConvertFrom-Json
    }

    $appJsonPath = Join-Path $OhosDir 'AppScope\app.json5'
    $appJson = Get-Content -Raw -LiteralPath $appJsonPath
    $versionName = [regex]::Match($appJson, '"versionName"\s*:\s*"([^"]+)"').Groups[1].Value
    $versionCode = [regex]::Match($appJson, '"versionCode"\s*:\s*(\d+)').Groups[1].Value
    if (-not $versionName -or -not $versionCode) {
        throw 'Unable to determine app version from pack.info or AppScope/app.json5.'
    }

    return [pscustomobject]@{
        summary = [pscustomobject]@{
            app = [pscustomobject]@{
                version = [pscustomobject]@{
                    name = $versionName
                    code = [int]$versionCode
                }
            }
            modules = @(
                [pscustomobject]@{
                    apiVersion = [pscustomobject]@{
                        compatible = 18
                    }
                }
            )
        }
    }
}

$props = Read-PropertiesFile -Path $PropertiesFile
$storeFile = Resolve-ConfiguredPath (Get-RequiredValue -Props $props -Key 'storeFile')
$storePassword = Get-RequiredValue -Props $props -Key 'storePassword'
$keyAlias = Get-RequiredValue -Props $props -Key 'keyAlias'
$keyPassword = Get-RequiredValue -Props $props -Key 'keyPassword'
$certPath = Resolve-ConfiguredPath (Get-RequiredValue -Props $props -Key 'certpath')
$profilePath = Resolve-ConfiguredPath (Get-RequiredValue -Props $props -Key 'profile')
$signAlg = if ($props.ContainsKey('signAlg') -and $props['signAlg']) { [string]$props['signAlg'] } else { 'SHA256withECDSA' }
$hapSignTool = if ($props.ContainsKey('hapSignTool') -and $props['hapSignTool']) {
    Resolve-ConfiguredPath ([string]$props['hapSignTool'])
} else {
    'D:\SDK\harmonyos-sdk\5.1.0\openharmony\toolchains\lib\hap-sign-tool.jar'
}

foreach ($requiredPath in @($UnsignedHap, $storeFile, $certPath, $profilePath, $hapSignTool)) {
    if (-not (Test-Path -LiteralPath $requiredPath)) {
        throw "Missing required file: $requiredPath"
    }
}

$packInfo = Get-PackInfo
$versionName = [string]$packInfo.summary.app.version.name
$versionCode = [string]$packInfo.summary.app.version.code
$compatibleVersion = [string]$packInfo.summary.modules[0].apiVersion.compatible
if (-not $compatibleVersion) {
    $compatibleVersion = '18'
}

New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
$artifactName = "PTPatronus-ohos-v$versionName+$versionCode-release-signed.hap"
$outputHap = Join-Path $OutputDir $artifactName
$verifyCertChain = Join-Path $OutputDir "PTPatronus-ohos-v$versionName+$versionCode-release-certchain.cer"
$verifyProfile = Join-Path $OutputDir "PTPatronus-ohos-v$versionName+$versionCode-release-profile.p7b"

$signArgs = @(
    '-jar', $hapSignTool,
    'sign-app',
    '-mode', 'localSign',
    '-keyAlias', $keyAlias,
    '-keyPwd', $keyPassword,
    '-appCertFile', $certPath,
    '-profileFile', $profilePath,
    '-profileSigned', '1',
    '-inFile', $UnsignedHap,
    '-signAlg', $signAlg,
    '-keystoreFile', $storeFile,
    '-keystorePwd', $storePassword,
    '-outFile', $outputHap,
    '-compatibleVersion', $compatibleVersion,
    '-signCode', '1'
)

& java @signArgs
if ($LASTEXITCODE -ne 0) {
    throw "hap-sign-tool sign-app failed with exit code $LASTEXITCODE"
}

$verifyArgs = @(
    '-jar', $hapSignTool,
    'verify-app',
    '-inFile', $outputHap,
    '-outCertChain', $verifyCertChain,
    '-outProfile', $verifyProfile
)

& java @verifyArgs
if ($LASTEXITCODE -ne 0) {
    throw "hap-sign-tool verify-app failed with exit code $LASTEXITCODE"
}

Write-Host "Signed HAP: $outputHap"
Write-Host "Verified certificate chain: $verifyCertChain"
Write-Host "Verified profile: $verifyProfile"
