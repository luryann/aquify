$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = Split-Path -Parent $scriptDir
$preflight = Join-Path $scriptDir "evidence-preflight.ps1"
$tmp = Join-Path $repoRoot ".tmp-evidence-preflight-test"

function Clear-EvidenceEnv {
    $names = @(
        "INPUT_DIR",
        "SOURCE_COUNTS",
        "SOURCE_BILLING_TOTAL_CENTS",
        "ORG_ID",
        "API_BASE_URL",
        "AUTH_TOKEN",
        "EXPECT_STRIPE_WEBHOOK_EVENT_ID",
        "CONFIRM_STRIPE_REFUND",
        "REFUND_INVOICE_ID",
        "REFUND_PAYMENT_ID",
        "REFUND_AMOUNT_CENTS",
        "POSTMARK_CAMPAIGN_ID",
        "CONFIRM_POSTMARK_SEND",
        "MEET_IDS",
        "NO_MANUAL_PATCHING_ATTESTATION",
        "NO_OPEN_P0_P1_ATTESTATION",
        "SUPPORT_INCIDENTS_FILE"
    )
    foreach ($name in $names) {
        Remove-Item -LiteralPath "Env:$name" -ErrorAction SilentlyContinue
    }
}

function Set-TestEnv {
    param([string]$Name, [string]$Value)
    Set-Item -LiteralPath "Env:$Name" -Value $Value
}

function Set-EvidenceEnv {
    param([string]$InputDir, [string]$SourceCounts, [string]$SupportFile)
    Set-TestEnv "INPUT_DIR" $InputDir
    Set-TestEnv "SOURCE_COUNTS" $SourceCounts
    Set-TestEnv "SOURCE_BILLING_TOTAL_CENTS" "100"
    Set-TestEnv "ORG_ID" "org-test"
    Set-TestEnv "API_BASE_URL" "https://api.example.com"
    Set-TestEnv "AUTH_TOKEN" "token"
    Set-TestEnv "EXPECT_STRIPE_WEBHOOK_EVENT_ID" "evt_123"
    Set-TestEnv "CONFIRM_STRIPE_REFUND" "true"
    Set-TestEnv "REFUND_INVOICE_ID" "inv_123"
    Set-TestEnv "REFUND_PAYMENT_ID" "pay_123"
    Set-TestEnv "REFUND_AMOUNT_CENTS" "1"
    Set-TestEnv "POSTMARK_CAMPAIGN_ID" "campaign_123"
    Set-TestEnv "CONFIRM_POSTMARK_SEND" "true"
    Set-TestEnv "MEET_IDS" "meet_1,meet_2"
    Set-TestEnv "NO_MANUAL_PATCHING_ATTESTATION" "true"
    Set-TestEnv "NO_OPEN_P0_P1_ATTESTATION" "true"
    Set-TestEnv "SUPPORT_INCIDENTS_FILE" $SupportFile
}

function Invoke-Preflight {
    param([string]$Mode)
    $exe = if ($PSVersionTable.PSEdition -eq "Core") { "pwsh" } else { "powershell" }
    $stdout = Join-Path $tmp "preflight-$Mode.out"
    $stderr = Join-Path $tmp "preflight-$Mode.err"
    $args = @("-NoProfile", "-ExecutionPolicy", "Bypass", "-File", $preflight, "-Mode", $Mode)
    $process = Start-Process -FilePath $exe -ArgumentList $args -NoNewWindow -Wait -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
    return $process.ExitCode
}

try {
    Clear-EvidenceEnv
    if (Test-Path -LiteralPath $tmp) {
        Remove-Item -LiteralPath $tmp -Recurse -Force
    }
    New-Item -ItemType Directory -Force -Path $tmp | Out-Null
    $counts = Join-Path $tmp "source-counts.json"
    $support = Join-Path $tmp "support.csv"
    @'
{
  "households.csv": 1,
  "swimmers.csv": 1,
  "groups.csv": 1,
  "historical_best_times.csv": 1,
  "open_invoices.csv": 1,
  "payment_history.csv": 1
}
'@ | Set-Content -LiteralPath $counts -Encoding UTF8
    "id,severity,status" | Set-Content -LiteralPath $support -Encoding UTF8

    $missingExit = Invoke-Preflight "final"
    if ($missingExit -eq 0) {
        throw "Expected preflight with missing env vars to fail."
    }

    Clear-EvidenceEnv
    Set-EvidenceEnv -InputDir $tmp -SourceCounts $counts -SupportFile $support
    $passExit = Invoke-Preflight "final"
    if ($passExit -ne 0) {
        throw "Expected preflight with complete env vars to pass."
    }

    Set-TestEnv "REFUND_AMOUNT_CENTS" "0"
    $invalidExit = Invoke-Preflight "final"
    if ($invalidExit -eq 0) {
        throw "Expected preflight with REFUND_AMOUNT_CENTS=0 to fail."
    }

    Write-Host "Evidence preflight regression tests passed."
} finally {
    Clear-EvidenceEnv
    if (Test-Path -LiteralPath $tmp) {
        Remove-Item -LiteralPath $tmp -Recurse -Force
    }
}
