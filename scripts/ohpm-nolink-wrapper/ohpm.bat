@echo off
rem ohpm --no-link wrapper: appends --no-link to ohpm install calls so
rem link-based installs become real copies (avoids EPERM without symlink
rem privilege). build_ohos.ps1 sets env ohpmBin to this file (absolute path)
rem and prepends its dir to PATH. NOTE: hvigor invokes "ohpm i --all" (short form).
setlocal
set "nolink="
if /I "%~1"=="install" set "nolink=--no-link"
if /I "%~1"=="i" set "nolink=--no-link"
call "D:\SDK\ohos-commandline\oh-command-line-tools\ohpm\bin\ohpm.bat" %* %nolink%
exit /b %ERRORLEVEL%
