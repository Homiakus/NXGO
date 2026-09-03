$ErrorActionPreference = 'Stop'

if ([string]::IsNullOrWhiteSpace($env:NXGO_NX_MANAGED)) {
    if (![string]::IsNullOrWhiteSpace($env:UGII_BASE_DIR)) {
        $env:NXGO_NX_MANAGED = Join-Path $env:UGII_BASE_DIR "NXBIN\managed_core"
        if (!(Test-Path $env:NXGO_NX_MANAGED)) {
            $env:NXGO_NX_MANAGED = Join-Path $env:UGII_BASE_DIR "NXBIN\managed"
        }
    }
}

if ([string]::IsNullOrWhiteSpace($env:NXGO_NX_MANAGED)) {
    $siemensRoot = Join-Path ${env:ProgramFiles} 'Siemens'
    $homes = @()
    if (Test-Path $siemensRoot) {
        $homes = Get-ChildItem -LiteralPath $siemensRoot -Directory -ErrorAction SilentlyContinue |
            Where-Object { $_.Name -match 'NX\d{4}' } |
            Sort-Object Name -Descending |
            ForEach-Object { $_.FullName }
    }
    foreach ($nxHomeCandidate in $homes) {
        foreach ($managedName in @('managed_core', 'managed')) {
            $candidate = Join-Path $nxHomeCandidate "NXBIN\$managedName"
            if (Test-Path (Join-Path $candidate 'NXOpen.dll')) {
                $env:NXGO_NX_MANAGED = $candidate
                break
            }
        }
        if (![string]::IsNullOrWhiteSpace($env:NXGO_NX_MANAGED)) { break }
    }
}

if ([string]::IsNullOrWhiteSpace($env:NXGO_NX_MANAGED) -or !(Test-Path $env:NXGO_NX_MANAGED)) {
    throw 'NXGO_NX_MANAGED must point to the installed NX managed_core/NXOpen assembly directory.'
}
if ([IO.Path]::GetFileName((Resolve-Path $env:NXGO_NX_MANAGED).Path) -ne 'managed_core') {
    throw 'Canonical NXHost requires NXGO_NX_MANAGED to be the NX2512 managed_core directory.'
}

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$agentBin = Join-Path $repoRoot 'agent\bin'
$hostProject = Join-Path $repoRoot 'agent\NXGO.Agent.NXHost\NXGO.Agent.NXHost.csproj'
$projectText = Get-Content -Raw -LiteralPath $hostProject
foreach ($requiredReference in @('NXGO.Agent.Core', 'NXGO.Protocol')) {
    if ($projectText -notmatch [regex]::Escape($requiredReference)) {
        throw "Canonical NXHost project is missing required reference: $requiredReference"
    }
}
New-Item -ItemType Directory -Force -Path $agentBin | Out-Null

Write-Host "Building canonical NXGO.Protocol + NXGO.Agent.Core + NXGO.Agent.NXHost against $env:NXGO_NX_MANAGED..."
& dotnet build $hostProject -c Release -p:NXGO_NX_MANAGED="$env:NXGO_NX_MANAGED" -o $agentBin --nologo
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

$protocolDll = Join-Path $agentBin 'NXGO.Protocol.dll'
$coreDll = Join-Path $agentBin 'NXGO.Agent.Core.dll'
$hostDll = Join-Path $agentBin 'NXGO.Agent.NXHost.dll'
$runtimeConfig = Join-Path $agentBin 'NXGO.Agent.NXHost.runtimeconfig.json'
$jsonDll = Join-Path $agentBin 'Newtonsoft.Json.dll'
if (!(Test-Path $protocolDll)) { throw "Canonical Protocol output missing: $protocolDll" }
if (!(Test-Path $coreDll)) { throw "Canonical Agent Core output missing: $coreDll" }
if (!(Test-Path $hostDll)) { throw "Canonical NXHost output missing: $hostDll" }
if (!(Test-Path $runtimeConfig)) { throw "Canonical NXHost runtimeconfig missing: $runtimeConfig" }
if (!(Test-Path $jsonDll)) { throw "Canonical JSON runtime output missing: $jsonDll" }

# Siemens' managed_core runner resolves application dependencies from its
# managed directory, not from the target DLL directory. The user-approved
# deployment contract therefore stages only NXGO's non-proprietary runtime
# dependencies beside the NXOpen assemblies. NXOpen/Siemens binaries are
# never copied or modified by this step.
foreach ($runtimeDependency in @($coreDll, $protocolDll, $jsonDll)) {
    Copy-Item -LiteralPath $runtimeDependency -Destination (Join-Path $env:NXGO_NX_MANAGED ([IO.Path]::GetFileName($runtimeDependency))) -Force
}
Write-Host "Staged NXGO managed_core dependencies in $env:NXGO_NX_MANAGED"
Write-Host "Canonical Agent built: $hostDll"
Write-Host "Canonical wire runtime: $protocolDll + $jsonDll"
