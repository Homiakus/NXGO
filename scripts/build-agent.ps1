$ErrorActionPreference = 'Stop'

if ([string]::IsNullOrWhiteSpace($env:NXGO_NX_MANAGED)) {
    if (![string]::IsNullOrWhiteSpace($env:UGII_BASE_DIR)) {
        $env:NXGO_NX_MANAGED = Join-Path $env:UGII_BASE_DIR "NXBIN\managed"
        if (!(Test-Path $env:NXGO_NX_MANAGED)) {
            $env:NXGO_NX_MANAGED = Join-Path $env:UGII_BASE_DIR "UGII\managed"
        }
    }
}

if ([string]::IsNullOrWhiteSpace($env:NXGO_NX_MANAGED) -or !(Test-Path $env:NXGO_NX_MANAGED)) {
    throw 'NXGO_NX_MANAGED must point to the installed NX managed/NXOpen assembly directory.'
}

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$agentBin = Join-Path $repoRoot 'agent\bin'
$hostProject = Join-Path $repoRoot 'agent\NXGO.Agent.NXHost\NXGO.Agent.NXHost.csproj'
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

# Transitional H4 parity path. The legacy monolithic journal remains the
# default full-CAD worker until the compiled NXHost reaches operation parity
# and passes the real-NX semantic gate. Building it here prevents regressions
# while migration proceeds operation-by-operation.
$legacySrc = Join-Path $repoRoot 'agent\bundle\AgentWorker.cs'
$legacyOut = Join-Path $repoRoot 'agent\bundle\AgentWorker.dll'
$csc = 'C:\Windows\Microsoft.NET\Framework64\v4.0.30319\csc.exe'

$nxOpenDll = Join-Path $env:NXGO_NX_MANAGED 'NXOpen.dll'
$nxOpenUfDll = Join-Path $env:NXGO_NX_MANAGED 'NXOpen.UF.dll'
$nxOpenUtilDll = Join-Path $env:NXGO_NX_MANAGED 'NXOpen.Utilities.dll'

Write-Host "Compiling transitional legacy AgentWorker with $csc..."
& $csc /nologo /target:library /r:System.Core.dll /r:"$nxOpenDll" /r:"$nxOpenUfDll" /r:"$nxOpenUtilDll" /out:"$legacyOut" "$legacySrc"
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Host "Legacy parity Agent built: $legacyOut"
