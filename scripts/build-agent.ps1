$ErrorActionPreference = 'Stop'

if ([string]::IsNullOrWhiteSpace($env:NXGO_NX_MANAGED)) {
    if (![string]::IsNullOrWhiteSpace($env:UGII_BASE_DIR)) {
        $env:NXGO_NX_MANAGED = Join-Path $env:UGII_BASE_DIR "NXBIN\managed"
        if (! (Test-Path $env:NXGO_NX_MANAGED)) {
            $env:NXGO_NX_MANAGED = Join-Path $env:UGII_BASE_DIR "UGII\managed"
        }
    }
}

if ([string]::IsNullOrWhiteSpace($env:NXGO_NX_MANAGED) -or !(Test-Path $env:NXGO_NX_MANAGED)) {
    throw 'NXGO_NX_MANAGED must point to the installed NX managed/NXOpen assembly directory.'
}

$agentSrc = Join-Path $PSScriptRoot '..\agent\bundle\AgentWorker.cs'
$agentOut = Join-Path $PSScriptRoot '..\agent\bundle\AgentWorker.dll'
$csc = 'C:\Windows\Microsoft.NET\Framework64\v4.0.30319\csc.exe'

$nxOpenDll = Join-Path $env:NXGO_NX_MANAGED 'NXOpen.dll'
$nxOpenUfDll = Join-Path $env:NXGO_NX_MANAGED 'NXOpen.UF.dll'
$nxOpenUtilDll = Join-Path $env:NXGO_NX_MANAGED 'NXOpen.Utilities.dll'

Write-Host "Compiling NXGO Agent with $csc against $env:NXGO_NX_MANAGED..."
& $csc /nologo /target:library /r:System.Core.dll /r:"$nxOpenDll" /r:"$nxOpenUfDll" /r:"$nxOpenUtilDll" /out:"$agentOut" "$agentSrc"

if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Host "NXGO Agent successfully built at $agentOut"
