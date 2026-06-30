# Dev launch script (PowerShell): load backend/.env -> go run.
#
# Usage:
#   powershell -ExecutionPolicy Bypass -File backend\dev.ps1
#
# First time (persist config so any shell - or Claude - can restart later):
#   copy backend\configs\config.example.yaml backend\.env
#   # then edit backend\.env with real SECRET_KEY / ACTIVATION_SECRET
#
# Without .env, falls back to the current shell's environment variables.
$ErrorActionPreference = 'Stop'
Set-Location -Path $PSScriptRoot   # cd to backend/

if (Test-Path .env) {
    Get-Content .env | ForEach-Object {
        $line = $_.Trim()
        if ($line -and -not $line.StartsWith('#') -and $line -match '^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$') {
            $k = $Matches[1]
            $v = $Matches[2].Trim().Trim('"').Trim("'")
            Set-Item -Path "Env:$k" -Value $v
        }
    }
    Write-Host "[dev] loaded backend/.env"
} else {
    Write-Host "[dev] no backend/.env found, using current env vars"
    Write-Host "[dev] to persist: copy configs/config.example.yaml .env and fill real values"
}

Write-Host "[dev] go run ./cmd/ptpatronus  (Ctrl+C to stop; re-run after code changes)"
go run ./cmd/ptpatronus
