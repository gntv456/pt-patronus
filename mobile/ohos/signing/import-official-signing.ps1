param(
    [string]$SourceDir,
    [string]$StoreFile,
    [string]$CertFile,
    [string]$ProfileFile,
    [string]$KeyAlias,
    [string]$StorePassword,
    [string]$KeyPassword,
    [switch]$UseEnvironmentPasswords,
    [string]$SignAlg = 'SHA256withECDSA',
    [string]$HapSignTool = 'D:\SDK\harmonyos-sdk\5.1.0\openharmony\toolchains\lib\hap-sign-tool.jar',
    [switch]$Force
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$SigningDir = $PSScriptRoot
$OfficialDir = Join-Path $SigningDir 'official'
$PropertiesFile = Join-Path $OfficialDir 'signing.properties'

function Resolve-InputFile {
    param(
        [string]$ExplicitPath,
        [string]$Directory,
        [string[]]$Patterns,
        [string]$Label
    )

    if ($ExplicitPath) {
        if (-not (Test-Path -LiteralPath $ExplicitPath)) {
            throw "$Label not found: $ExplicitPath"
        }
        return (Resolve-Path -LiteralPath $ExplicitPath).Path
    }

    if (-not $Directory) {
        throw "Missing $Label. Pass -SourceDir or provide the file explicitly."
    }
    if (-not (Test-Path -LiteralPath $Directory)) {
        throw "SourceDir not found: $Directory"
    }

    $matches = foreach ($pattern in $Patterns) {
        Get-ChildItem -LiteralPath $Directory -File -Filter $pattern -ErrorAction SilentlyContinue
    }

    $uniqueMatches = @($matches | Sort-Object -Property FullName -Unique)
    if ($uniqueMatches.Count -eq 0) {
        throw "No $Label found in $Directory. Expected: $($Patterns -join ', ')"
    }
    if ($uniqueMatches.Count -gt 1) {
        $paths = ($uniqueMatches | ForEach-Object { $_.FullName }) -join [Environment]::NewLine
        throw "Multiple $Label files found. Pass the exact path explicitly:$([Environment]::NewLine)$paths"
    }

    return $uniqueMatches[0].FullName
}

function ConvertTo-PlainText {
    param([securestring]$SecureValue)

    $bstr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($SecureValue)
    try {
        return [Runtime.InteropServices.Marshal]::PtrToStringBSTR($bstr)
    }
    finally {
        [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($bstr)
    }
}

New-Item -ItemType Directory -Force -Path $OfficialDir | Out-Null

if ((Test-Path -LiteralPath $PropertiesFile) -and -not $Force) {
    throw "signing.properties already exists. Re-run with -Force to replace it."
}

$resolvedStore = Resolve-InputFile -ExplicitPath $StoreFile -Directory $SourceDir -Patterns @('*.p12', '*.pfx') -Label 'keystore'
$resolvedCert = Resolve-InputFile -ExplicitPath $CertFile -Directory $SourceDir -Patterns @('*.cer') -Label 'app certificate'
$resolvedProfile = Resolve-InputFile -ExplicitPath $ProfileFile -Directory $SourceDir -Patterns @('*.p7b') -Label 'signed profile'

$targetStoreExt = [IO.Path]::GetExtension($resolvedStore)
$targetStore = Join-Path $OfficialDir "official$targetStoreExt"
$targetCert = Join-Path $OfficialDir 'official.cer'
$targetProfile = Join-Path $OfficialDir 'official.p7b'

Copy-Item -LiteralPath $resolvedStore -Destination $targetStore -Force
Copy-Item -LiteralPath $resolvedCert -Destination $targetCert -Force
Copy-Item -LiteralPath $resolvedProfile -Destination $targetProfile -Force

if (-not $KeyAlias) {
    $KeyAlias = Read-Host 'DevEco/Huawei key alias'
}

if ($UseEnvironmentPasswords) {
    $StorePassword = '%PTPATRONUS_OHOS_STORE_PASSWORD%'
    $KeyPassword = '%PTPATRONUS_OHOS_KEY_PASSWORD%'
}
else {
    if (-not $StorePassword) {
        $StorePassword = ConvertTo-PlainText (Read-Host 'Keystore password' -AsSecureString)
    }
    if (-not $KeyPassword) {
        $KeyPassword = ConvertTo-PlainText (Read-Host 'Key password' -AsSecureString)
    }
}

$content = @(
    '# Local official DevEco/Huawei signing config. Do not commit this file.',
    '# The signed profile must match bundleName com.hxpt.ptpatronus.',
    "storeFile=$targetStore",
    "storePassword=$StorePassword",
    "keyAlias=$KeyAlias",
    "keyPassword=$KeyPassword",
    "certpath=$targetCert",
    "profile=$targetProfile",
    "signAlg=$SignAlg",
    "hapSignTool=$HapSignTool"
)

Set-Content -LiteralPath $PropertiesFile -Value $content -Encoding UTF8

Write-Host "Imported official signing files into: $OfficialDir"
Write-Host "Generated local signing config: $PropertiesFile"
Write-Host 'This config and certificate files are ignored by git.'
if ($UseEnvironmentPasswords) {
    Write-Host 'Set PTPATRONUS_OHOS_STORE_PASSWORD and PTPATRONUS_OHOS_KEY_PASSWORD before signing.'
}
