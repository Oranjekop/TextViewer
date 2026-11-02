param(
    [Parameter(Mandatory = $true)]
    [string]$Tag,
    [switch]$BootstrapTools,
    [switch]$SkipTests,
    [string]$CertificateThumbprint
)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot 'tools.ps1')
Push-Location $root
try {
    go run ./cmd/release prepare -tag $Tag
    Assert-TextViewerExit 'Release tag validation'
    $version = (Get-Content -LiteralPath mygo.json -Raw | ConvertFrom-Json).version
    $buildDirectory = "dist\native-$version"
    if (-not $SkipTests) {
        go test ./...
        Assert-TextViewerExit 'Go tests'
        go vet ./...
        Assert-TextViewerExit 'Go vet'
        go mod verify
        Assert-TextViewerExit 'Module verification'
        & (Join-Path $PSScriptRoot 'test-context-menu.ps1')
    }
    foreach ($architecture in @('amd64', 'arm64')) {
        $arguments = @{
            OutputDirectory = $buildDirectory
            Architecture = $architecture
            BootstrapTools = $BootstrapTools
            SkipTests = $true
        }
        if ($CertificateThumbprint) { $arguments.CertificateThumbprint = $CertificateThumbprint }
        & (Join-Path $PSScriptRoot 'build.ps1') @arguments
    }
    & (Join-Path $PSScriptRoot 'test-shell.ps1') -BuildDirectory "$buildDirectory\windows-amd64"
    go run ./cmd/release package -tag $Tag -build $buildDirectory -output "dist\release\$Tag"
    Assert-TextViewerExit 'Universal installer and portable archives'
    Write-Output "Release assets: $root\dist\release\$Tag"
} finally {
    Pop-Location
}
