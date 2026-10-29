param(
    [ValidateSet("final", "migration", "provider", "parallel", "stability")]
    [string]$Mode = "final"
)

$ErrorActionPreference = "Stop"

$failures = New-Object System.Collections.Generic.List[string]
$warnings = New-Object System.Collections.Generic.List[string]
$seenEnv = @{}

function Add-Failure {
    param([string]$Message)
    $failures.Add($Message) | Out-Null
}

function Add-Warning {
    param([string]$Message)
    $warnings.Add($Message) | Out-Null
}

function Test-CommandAvailable {
    param([string]$Name, [bool]$Required = $true)
    $cmd = Get-Command $Name -CommandType Application -ErrorAction SilentlyContinue
    if ($null -eq $cmd -and $Name -notmatch "\.exe$") {
        $cmd = Get-Command "$Name.exe" -CommandType Application -ErrorAction SilentlyContinue
    }
    if ($cmd -is [array]) {
        $cmd = $cmd[0]
    }
    if ($null -eq $cmd) {
        if ($Required) {
            Add-Failure "Missing required command: $Name"
        } else {
            Add-Warning "Optional command not found: $Name"
        }
        return
    }
    Write-Host "OK command: $Name -> $($cmd.Source)"
}

function Require-Env {
    param([string]$Name)
    if ($seenEnv.ContainsKey($Name)) {
        return [Environment]::GetEnvironmentVariable($Name)
    }
    $seenEnv[$Name] = $true
    $value = [Environment]::GetEnvironmentVariable($Name)
    if ([string]::IsNullOrWhiteSpace($value)) {
        Add-Failure "Missing required environment variable: $Name"
        return $null
    }
    Write-Host "OK env: $Name"
    return $value
}

function Require-EnvEquals {
    param([string]$Name, [string]$Expected)
    $value = Require-Env $Name
    if ($null -eq $value) {
        return
    }
    if ($value.Trim().ToLowerInvariant() -ne $Expected.ToLowerInvariant()) {
        Add-Failure "$Name must be '$Expected' for this evidence mode."
    }
}

function Require-IntEnv {
    param([string]$Name, [int]$Min, [string]$Description)
    $value = Require-Env $Name
    if ($null -eq $value) {
        return
    }
    $parsed = 0
    if (-not [int]::TryParse($value, [ref]$parsed) -or $parsed -lt $Min) {
        Add-Failure "$Name must be a $Description."
    }
}

function Require-PositiveIntEnv {
    param([string]$Name)
    Require-IntEnv $Name 1 "positive integer"
}

function Require-NonNegativeIntEnv {
    param([string]$Name)
    Require-IntEnv $Name 0 "non-negative integer"
}

function Require-Path {
    param([string]$Name, [string]$Path, [string]$PathType, [string]$Label)
    if ([string]::IsNullOrWhiteSpace($Path)) {
        Add-Failure "Missing path for $Name"
        return
    }
    if (-not (Test-Path -LiteralPath $Path -PathType $PathType)) {
        Add-Failure "$Name does not exist or is not a ${Label}: $Path"
        return
    }
    Write-Host "OK ${Label}: $Name -> $Path"
}

function Require-File {
    param([string]$Name, [string]$Path)
    Require-Path $Name $Path "Leaf" "file"
}

function Require-Directory {
    param([string]$Name, [string]$Path)
    Require-Path $Name $Path "Container" "directory"
}

function Validate-SourceCounts {
    param([string]$Path)
    if ([string]::IsNullOrWhiteSpace($Path)) {
        return
    }
    Require-File "SOURCE_COUNTS" $Path
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        return
    }
    try {
        $json = Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json
        $required = @(
            "households.csv",
            "swimmers.csv",
            "groups.csv",
            "historical_best_times.csv",
            "open_invoices.csv",
            "payment_history.csv"
        )
        foreach ($key in $required) {
            if (-not ($json.PSObject.Properties.Name -contains $key)) {
                Add-Failure "SOURCE_COUNTS is missing key: $key"
            }
        }
    } catch {
        Add-Failure "SOURCE_COUNTS is not valid JSON: $($_.Exception.Message)"
    }
}

Write-Host "Evidence preflight mode: $Mode"

Test-CommandAvailable "go"
Test-CommandAvailable "curl"
Test-CommandAvailable "bash" ($false)
Test-CommandAvailable "python" ($false)

$needsMigration = $Mode -in @("final", "migration")
$needsAPI = $Mode -in @("final", "provider", "parallel", "stability")
$needsProvider = $Mode -in @("final", "provider")
$needsParallel = $Mode -in @("final", "parallel")
$needsStability = $Mode -in @("final", "stability")

if ($needsMigration) {
    $inputDir = Require-Env "INPUT_DIR"
    Require-Directory "INPUT_DIR" $inputDir
    $sourceCounts = Require-Env "SOURCE_COUNTS"
    Validate-SourceCounts $sourceCounts
    Require-NonNegativeIntEnv "SOURCE_BILLING_TOTAL_CENTS"
    Require-Env "ORG_ID" | Out-Null
}

if ($needsAPI) {
    Require-Env "API_BASE_URL" | Out-Null
    Require-Env "AUTH_TOKEN" | Out-Null
    Require-Env "ORG_ID" | Out-Null
}

if ($needsProvider) {
    Require-Env "EXPECT_STRIPE_WEBHOOK_EVENT_ID" | Out-Null
    Require-EnvEquals "CONFIRM_STRIPE_REFUND" "true"
    Require-Env "REFUND_INVOICE_ID" | Out-Null
    Require-Env "REFUND_PAYMENT_ID" | Out-Null
    Require-PositiveIntEnv "REFUND_AMOUNT_CENTS"
    Require-Env "POSTMARK_CAMPAIGN_ID" | Out-Null
    Require-EnvEquals "CONFIRM_POSTMARK_SEND" "true"
}

if ($needsParallel) {
    $meetIDs = Require-Env "MEET_IDS"
    Require-EnvEquals "NO_MANUAL_PATCHING_ATTESTATION" "true"
    if (-not [string]::IsNullOrWhiteSpace($meetIDs)) {
        $count = ($meetIDs -split "," | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }).Count
        if ($count -lt 2) {
            Add-Failure "MEET_IDS must contain at least two comma-separated IDs."
        }
    }
}

if ($needsStability) {
    Require-EnvEquals "NO_OPEN_P0_P1_ATTESTATION" "true"
    $supportFile = Require-Env "SUPPORT_INCIDENTS_FILE"
    Require-File "SUPPORT_INCIDENTS_FILE" $supportFile
}

if ($warnings.Count -gt 0) {
    Write-Host ""
    Write-Host "Warnings:"
    foreach ($warning in $warnings) {
        Write-Host "- $warning"
    }
}

if ($failures.Count -gt 0) {
    Write-Host ""
    Write-Error ("Preflight failed:`n- " + ($failures -join "`n- "))
    exit 1
}

Write-Host ""
Write-Host "Evidence preflight passed."
