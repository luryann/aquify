$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location (Join-Path $repoRoot "server")

$envFile = Join-Path $repoRoot ".env.local"
if (Test-Path $envFile) {
    Get-Content $envFile | ForEach-Object {
        if ($_ -match '^\s*#' -or $_ -notmatch '=') {
            return
        }
        $name, $value = $_ -split '=', 2
        if (-not [string]::IsNullOrWhiteSpace($name) -and -not [Environment]::GetEnvironmentVariable($name)) {
            Set-Item -Path "Env:$name" -Value $value
        }
    }
}

& "C:\Program Files\Go\bin\go.exe" run .\cmd\api
