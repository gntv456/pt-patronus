# Official DevEco/Huawei signing

Put only real Huawei/DevEco release signing material in this directory. The actual certificate files and `signing.properties` are ignored by git.

## Required files

Export or download these from DevEco Studio / Huawei AppGallery Connect for bundle `com.hxpt.ptpatronus`:

- Release keystore: `.p12` or `.pfx`
- App certificate chain: `.cer`
- Signed release profile: `.p7b`
- Key alias
- Keystore password
- Key password

The signed profile must match the app bundle name in `AppScope/app.json5`.

## Import

If the export directory contains exactly one keystore, one `.cer`, and one `.p7b`:

```powershell
powershell -ExecutionPolicy Bypass -File ..\import-official-signing.ps1 -SourceDir "D:\path\to\huawei-signing-export"
```

If there are multiple certificate files, pass exact paths:

```powershell
powershell -ExecutionPolicy Bypass -File ..\import-official-signing.ps1 `
  -StoreFile "D:\path\official.p12" `
  -CertFile "D:\path\official.cer" `
  -ProfileFile "D:\path\official.p7b"
```

To keep passwords out of `signing.properties`, write environment-variable placeholders:

```powershell
powershell -ExecutionPolicy Bypass -File ..\import-official-signing.ps1 `
  -SourceDir "D:\path\to\huawei-signing-export" `
  -UseEnvironmentPasswords

$env:PTPATRONUS_OHOS_STORE_PASSWORD="your-store-password"
$env:PTPATRONUS_OHOS_KEY_PASSWORD="your-key-password"
```

## Sign

Build the unsigned HAP, then sign it:

```powershell
cd D:\PTPatronus\mobile\ohos
.\hvigorw.bat --mode module -p module=entry@default assembleHap --no-daemon

cd D:\PTPatronus
powershell -ExecutionPolicy Bypass -File .\mobile\ohos\signing\sign-official-hap.ps1
```

The signed artifact is written to `D:\PTPatronus\dist` with the app version in the file name.
