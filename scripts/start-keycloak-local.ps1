$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$keycloakDir = Join-Path $repoRoot "keycloak-26.5.6"
$realmFile = Join-Path $repoRoot "keycloak\realm-aquify-local.json"
$importDir = Join-Path $keycloakDir "data\import"
$h2Db = Join-Path $keycloakDir "data\h2\keycloakdb.mv.db"
$kcBat = Join-Path $keycloakDir "bin\kc.bat"
$jdkDir = "C:\Program Files\Microsoft\jdk-21.0.11.10-hotspot"

if (-not (Test-Path $kcBat)) {
    throw "Keycloak launcher not found at $kcBat"
}

New-Item -ItemType Directory -Force -Path $importDir | Out-Null

if (-not (Test-Path $h2Db)) {
    Copy-Item -LiteralPath $realmFile -Destination (Join-Path $importDir "realm-aquify-local.json") -Force
} else {
    Remove-Item -LiteralPath (Join-Path $importDir "realm-aquify-local.json") -ErrorAction SilentlyContinue
}

$env:KEYCLOAK_ADMIN = if ($env:KEYCLOAK_ADMIN) { $env:KEYCLOAK_ADMIN } else { "admin" }
$env:KEYCLOAK_ADMIN_PASSWORD = if ($env:KEYCLOAK_ADMIN_PASSWORD) { $env:KEYCLOAK_ADMIN_PASSWORD } else { "admin" }
$env:JAVA_OPTS_APPEND = if ($env:KEYCLOAK_JAVA_OPTS_APPEND) { $env:KEYCLOAK_JAVA_OPTS_APPEND } else { "--add-opens java.base/java.lang=ALL-UNNAMED" }
$env:JAVA_HOME = if ($env:JAVA_HOME) { $env:JAVA_HOME } else { $jdkDir }
$env:Path = (Join-Path $env:JAVA_HOME "bin") + ";" + $env:Path

& $kcBat start-dev --http-port=8180 --import-realm
