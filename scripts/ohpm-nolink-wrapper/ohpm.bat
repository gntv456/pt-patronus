@echo off
rem ohpm --no-link wrapper: appends --no-link to "ohpm install" calls so
rem link-based installs become real copies (avoids EPERM without symlink
rem privilege). build_ohos.ps1 prepends this dir to PATH.
setlocal
set "nolink="
if /I "%~1"=="install" set "nolink=--no-link"
call "D:\SDK\ohos-commandline\oh-command-line-tools\ohpm\bin\ohpm.bat" %* %nolink%
exit /b %ERRORLEVEL%
