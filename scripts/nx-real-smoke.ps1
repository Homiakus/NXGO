param(
    [string]$NxHome = $env:NXGO_NX_HOME
)

$ErrorActionPreference = 'Stop'
if ([string]::IsNullOrWhiteSpace($NxHome)) {
    $siemensRoot = Join-Path ${env:ProgramFiles} 'Siemens'
    $home = Get-ChildItem -LiteralPath $siemensRoot -Directory -ErrorAction SilentlyContinue |
        Where-Object { $_.Name -match 'NX\d{4}' } |
        Sort-Object Name -Descending |
        ForEach-Object { $_.FullName } |
        Where-Object { Test-Path (Join-Path $_ 'NXBIN\run_journal.exe') } |
        Select-Object -First 1
    if ($home) { $NxHome = $home }
}
if ([string]::IsNullOrWhiteSpace($NxHome)) {
    throw 'No Siemens NX installation with run_journal.exe was discovered. Set NXGO_NX_HOME to override discovery.'
}

$candidates = @(
    (Join-Path $NxHome 'NXBIN\run_journal.exe'),
    (Join-Path $NxHome 'UGII\run_journal.exe'),
    (Join-Path $NxHome 'run_journal.exe')
)
$runner = $candidates | Where-Object { Test-Path $_ } | Select-Object -First 1
if (-not $runner) {
    throw "run_journal.exe not found under NXGO_NX_HOME=$NxHome"
}

$journal = (Resolve-Path 'tests/nx/smoke.py').Path
$stamp = Get-Date -Format 'yyyyMMdd-HHmmss-fff'
$artifactDir = Join-Path (Resolve-Path '.').Path "artifacts\nx-smoke\$stamp"
New-Item -ItemType Directory -Force -Path $artifactDir | Out-Null
$marker = Join-Path $artifactDir 'marker.json'
$stdout = Join-Path $artifactDir 'run-journal.log'
$env:NXGO_SMOKE_MARKER = $marker
$env:UGII_KEEP_SYSTEM_LOG = 'YES'
$env:UGII_TMP_DIR = $artifactDir

Write-Host "NXGO real-NX smoke: $runner $journal"
& $runner $journal *>&1 | Tee-Object -FilePath $stdout
$code = $LASTEXITCODE
if ($code -ne 0) { throw "run_journal.exe exited with code $code; artifacts: $artifactDir" }
if (-not (Test-Path $marker)) { throw "NX journal exited without NXGO marker; real NX execution is unproven. Artifacts: $artifactDir" }

$payload = Get-Content $marker -Raw | ConvertFrom-Json
if ($payload.status -ne 'pass' -or $payload.kind -ne 'nxgo-real-nx-smoke') {
    throw "invalid NX smoke marker: $marker"
}
Write-Host "NXGO_REAL_NX_SMOKE_PASS artifacts=$artifactDir"
