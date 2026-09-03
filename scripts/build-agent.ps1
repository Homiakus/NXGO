$ErrorActionPreference = 'Stop'

if ([string]::IsNullOrWhiteSpace($env:NXGO_NX_MANAGED)) {
    if (![string]::IsNullOrWhiteSpace($env:UGII_BASE_DIR)) {
        $env:NXGO_NX_MANAGED = Join-Path $env:UGII_BASE_DIR "NXBIN\managed_core"
        if (!(Test-Path $env:NXGO_NX_MANAGED)) {
            $env:NXGO_NX_MANAGED = Join-Path $env:UGII_BASE_DIR "NXBIN\managed"
        }
    }
}

if ([string]::IsNullOrWhiteSpace($env:NXGO_NX_MANAGED) -or !(Test-Path $env:NXGO_NX_MANAGED)) {
    throw 'NXGO_NX_MANAGED must point to the installed NX managed/NXOpen assembly directory.'
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
$jsonDll = Join-Path $agentBin 'Newtonsoft.Json.dll'
if (!(Test-Path $protocolDll)) { throw "Canonical Protocol output missing: $protocolDll" }
if (!(Test-Path $coreDll)) { throw "Canonical Agent Core output missing: $coreDll" }
if (!(Test-Path $hostDll)) { throw "Canonical NXHost output missing: $hostDll" }
if (!(Test-Path $jsonDll)) { throw "Canonical JSON runtime output missing: $jsonDll" }
Write-Host "Canonical Agent built: $hostDll"
Write-Host "Canonical wire runtime: $protocolDll + $jsonDll"
