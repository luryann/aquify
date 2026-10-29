<#
.SYNOPSIS
  Starts local Keycloak (and Mailpit) for DARE platform development.

.DESCRIPTION
  The Keycloak server distribution is managed under .tools/ (gitignored) and is
  downloaded automatically on first run. All Keycloak configuration lives in
  deploy/keycloak/:
    - realm-aquify-local.json  (realm import: clients, roles, seed users, SMTP)
    - themes/aquify/           (branded login theme, synced on every start)

  The realm is imported on first start only, so users/passwords/MFA created
  locally survive restarts. Use -Reset to wipe Keycloak data and re-import.

.PARAMETER Reset
  Delete all local Keycloak data (users, sessions, realm) and re-import the
  realm from deploy/keycloak/realm-aquify-local.json.

.PARAMETER Port
  HTTP port for Keycloak (default: $env:KEYCLOAK_PORT or 8180).

.PARAMETER NoMailpit
  Skip starting Mailpit (SMTP sink on :1025, web UI on :8025).
#>
param(
    [switch]$Reset,
    [int]$Port = $(if ($env:KEYCLOAK_PORT) { [int]$env:KEYCLOAK_PORT } else { 8180 }),
    [switch]$NoMailpit
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$kcVersion = "26.5.6"
$toolsDir = Join-Path $repoRoot ".tools"
$kcDir = Join-Path $toolsDir "keycloak-$kcVersion"
$kcBat = Join-Path $kcDir "bin\kc.bat"
$realmFile = Join-Path $repoRoot "deploy\keycloak\realm-aquify-local.json"
$themeSrc = Join-Path $repoRoot "deploy\keycloak\themes\aquify"
$themeDst = Join-Path $kcDir "themes\aquify"
$dataDir = Join-Path $kcDir "data"
$importDir = Join-Path $dataDir "import"
$h2Db = Join-Path $dataDir "h2\keycloakdb.mv.db"

# ── Keycloak distribution (auto-download into .tools/) ───────────────────────
if (-not (Test-Path $kcBat)) {
    New-Item -ItemType Directory -Force -Path $toolsDir | Out-Null
    $zipPath = Join-Path $toolsDir "keycloak-$kcVersion.zip"
    $url = "https://github.com/keycloak/keycloak/releases/download/$kcVersion/keycloak-$kcVersion.zip"
    Write-Host "Downloading Keycloak $kcVersion (one-time, ~250 MB) ..." -ForegroundColor Cyan
    Invoke-WebRequest -Uri $url -OutFile $zipPath -UseBasicParsing
    Write-Host "Extracting to $kcDir ..."
    Expand-Archive -Path $zipPath -DestinationPath $toolsDir -Force
    Remove-Item $zipPath -Force
    if (-not (Test-Path $kcBat)) {
        throw "Keycloak download/extract failed: $kcBat not found"
    }
}

# ── Java runtime ──────────────────────────────────────────────────────────────
if (-not $env:JAVA_HOME -or -not (Test-Path (Join-Path $env:JAVA_HOME "bin\java.exe"))) {
    $jdk = Get-ChildItem "C:\Program Files\Microsoft\jdk-*" -Directory -ErrorAction SilentlyContinue |
        Sort-Object Name -Descending | Select-Object -First 1
    if (-not $jdk) {
        $jdk = Get-ChildItem "C:\Program Files\Eclipse Adoptium\jdk-*" -Directory -ErrorAction SilentlyContinue |
            Sort-Object Name -Descending | Select-Object -First 1
    }
    if (-not $jdk) {
        throw "No JDK found. Install a JDK 17+ (e.g. 'winget install Microsoft.OpenJDK.21') or set JAVA_HOME."
    }
    $env:JAVA_HOME = $jdk.FullName
}
$env:Path = (Join-Path $env:JAVA_HOME "bin") + ";" + $env:Path

# ── Theme sync (deploy/keycloak/themes/aquify is the source of truth) ────────
if (Test-Path $themeDst) { Remove-Item -Recurse -Force $themeDst }
New-Item -ItemType Directory -Force -Path (Split-Path $themeDst) | Out-Null
Copy-Item -Recurse -Force $themeSrc $themeDst
Write-Host "Synced login theme from deploy/keycloak/themes/aquify"

# ── Realm import (first run or -Reset only; preserves local users otherwise) ─
if ($Reset -and (Test-Path $dataDir)) {
    Write-Host "-Reset: wiping Keycloak data (users, sessions, realm) ..." -ForegroundColor Yellow
    Remove-Item -Recurse -Force $dataDir
}
New-Item -ItemType Directory -Force -Path $importDir | Out-Null
if (-not (Test-Path $h2Db)) {
    Copy-Item -Force $realmFile (Join-Path $importDir "realm-aquify-local.json")
    Write-Host "First start: realm 'aquify' will be imported from deploy/keycloak/realm-aquify-local.json"
} else {
    Remove-Item (Join-Path $importDir "realm-aquify-local.json") -ErrorAction SilentlyContinue
    Write-Host "Existing Keycloak data found: skipping realm import (users/passwords preserved; use -Reset for a clean slate)"
}

# ── Mailpit (SMTP sink so password-reset/verification emails work) ──────────
if (-not $NoMailpit) {
    $smtpUp = (Test-NetConnection -ComputerName 127.0.0.1 -Port 1025 -WarningAction SilentlyContinue).TcpTestSucceeded
    if (-not $smtpUp) {
        $mailpitExe = Join-Path $toolsDir "mailpit\mailpit.exe"
        if (-not (Test-Path $mailpitExe)) {
            Write-Host "Downloading Mailpit (SMTP sink for local auth emails) ..." -ForegroundColor Cyan
            $mpZip = Join-Path $toolsDir "mailpit.zip"
            Invoke-WebRequest -Uri "https://github.com/axllent/mailpit/releases/latest/download/mailpit-windows-amd64.zip" -OutFile $mpZip -UseBasicParsing
            Expand-Archive -Path $mpZip -DestinationPath (Join-Path $toolsDir "mailpit") -Force
            Remove-Item $mpZip -Force
        }
        Start-Process -FilePath $mailpitExe -WindowStyle Hidden
        Write-Host "Mailpit started: SMTP on :1025, inbox UI at http://localhost:8025"
    } else {
        Write-Host "SMTP already listening on :1025 (Mailpit or docker) — not starting another instance"
    }
}

# ── Start Keycloak ────────────────────────────────────────────────────────────
$env:KC_BOOTSTRAP_ADMIN_USERNAME = if ($env:KEYCLOAK_ADMIN) { $env:KEYCLOAK_ADMIN } else { "admin" }
$env:KC_BOOTSTRAP_ADMIN_PASSWORD = if ($env:KEYCLOAK_ADMIN_PASSWORD) { $env:KEYCLOAK_ADMIN_PASSWORD } else { "admin" }
$env:JAVA_OPTS_APPEND = if ($env:KEYCLOAK_JAVA_OPTS_APPEND) { $env:KEYCLOAK_JAVA_OPTS_APPEND } else { "--add-opens java.base/java.lang=ALL-UNNAMED" }

Write-Host ""
Write-Host "Starting Keycloak $kcVersion on http://localhost:$Port" -ForegroundColor Green
Write-Host "  Admin console: http://localhost:$Port/admin  ($($env:KC_BOOTSTRAP_ADMIN_USERNAME)/$($env:KC_BOOTSTRAP_ADMIN_PASSWORD))"
Write-Host "  Realm:         aquify"
# Print the users the realm import actually seeds (service accounts excluded)
# instead of a hand-kept list that drifts when roles change.
$seedUsers = (Get-Content -Raw $realmFile | ConvertFrom-Json).users |
    Where-Object { $_.username -notlike "service-account-*" } |
    ForEach-Object { "$($_.username) [$($_.realmRoles -join ',')]" }
Write-Host "  Seed users:    $($seedUsers -join '; ')"
Write-Host "  Seed password: ChangeMe123!"
Write-Host ""

& $kcBat start-dev --http-port=$Port --import-realm
