$ErrorActionPreference = 'Stop'

if ([string]::IsNullOrWhiteSpace($env:NXGO_NX_MANAGED)) {
    throw 'NXGO_NX_MANAGED must point to the installed NX managed/NXOpen assembly directory.'
}

$project = Join-Path $PSScriptRoot '..\agent\NXGO.Agent.NXHost\NXGO.Agent.NXHost.csproj'
dotnet build $project -c Release /p:NXGO_NX_MANAGED="$env:NXGO_NX_MANAGED"
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
