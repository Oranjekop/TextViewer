param(
    [string]$OutputDirectory = 'dist\TextViewer',
    [ValidateSet('amd64', 'arm64')]
    [string]$Architecture = 'amd64',
    [string]$CertificateThumbprint,
    [switch]$BootstrapTools,
    [switch]$SkipTests
)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
. (Join-Path $PSScriptRoot 'tools.ps1')
$tools = Get-TextViewerTools -Bootstrap:$BootstrapTools
Push-Location $root
try {
    # MyGo temporarily generates .syso resources. Do not overlap this build
    # with tests, vet or another architecture build in the same directory.
    if (-not $SkipTests) {
        go test ./...
        Assert-TextViewerExit 'Go tests'
        go vet ./...
        Assert-TextViewerExit 'Go vet'
    }
    go run ./cmd/assets assets
    Assert-TextViewerExit 'Application assets'
    $resources = Join-Path $root "resources\windows-$Architecture"
    New-Item -ItemType Directory -Path $resources -Force | Out-Null
    $windowsPowerShell = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
    $metadataPath = Join-Path $root '.cache\signing\certificate.json'
    $signArgs = @('-NoLogo', '-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass',
        '-File', (Join-Path $PSScriptRoot 'sign-package.ps1'), '-Mode', 'Prepare',
        '-MetadataPath', $metadataPath, '-CertificatePath', (Join-Path $resources 'TextViewer.ContextMenu.cer'))
    if ($CertificateThumbprint) { $signArgs += @('-CertificateThumbprint', $CertificateThumbprint) }
    & $windowsPowerShell @signArgs
    Assert-TextViewerExit 'Package signing identity'
    $signer = Get-Content -LiteralPath $metadataPath -Raw | ConvertFrom-Json
    $target = 'x86_64-w64-windows-gnu'
    $manifestArchitecture = 'x64'
    if ($Architecture -eq 'arm64') {
        $target = 'aarch64-w64-windows-gnu'
        $manifestArchitecture = 'arm64'
    }
    & $tools.Clang "--target=$target" -std=c++17 -O2 -shared -static -DUNICODE -D_UNICODE `
        'shell\TextViewerShell.cpp' 'shell\TextViewerShell.def' `
        -o (Join-Path $resources 'TextViewerShell.dll') -lole32 -lshell32 -lshlwapi -luuid
    Assert-TextViewerExit 'Explorer command extension'
    $packageRoot = Join-Path $root ".cache\package-$Architecture"
    $packageAssets = Join-Path $packageRoot 'Assets'
    New-Item -ItemType Directory -Path $packageAssets -Force | Out-Null
    Copy-Item -LiteralPath 'assets\Logo.png', 'assets\Logo44.png' -Destination $packageAssets
    Copy-Item -LiteralPath 'assets\file-icons\text.png' -Destination (Join-Path $packageAssets 'FileTypeText.png')
    $manifest = [IO.File]::ReadAllText((Join-Path $root 'shell\AppxManifest.xml'))
    $version = (Get-Content -LiteralPath mygo.json -Raw | ConvertFrom-Json).version
    if ($version -notmatch '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$') {
        throw 'The application version must be MAJOR.MINOR.PATCH.'
    }
    $identityVersion = "$version.0"
    $manifest = [regex]::Replace($manifest, '(?s)(<Identity\b[^>]*\bVersion=")[^"]+(")', "`${1}$identityVersion`${2}")
    $manifest = $manifest.Replace('ProcessorArchitecture="x64"', "ProcessorArchitecture=`"$manifestArchitecture`"")
    $manifest = $manifest.Replace('CN=TextViewer Local Development', [Security.SecurityElement]::Escape($signer.Publisher))
    [IO.File]::WriteAllText((Join-Path $packageRoot 'AppxManifest.xml'), $manifest, [Text.UTF8Encoding]::new($false))
    & $tools.MakeAppx pack /o /d $packageRoot /nv /p (Join-Path $resources 'TextViewer.ContextMenu.msix')
    Assert-TextViewerExit 'Context menu identity package'
    $signTool = Join-Path (Split-Path -Parent $tools.MakeAppx) 'signtool.exe'
    & $windowsPowerShell -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass `
        -File (Join-Path $PSScriptRoot 'sign-package.ps1') -Mode Sign `
        -MetadataPath $metadataPath -SignToolPath $signTool `
        -PackagePath (Join-Path $resources 'TextViewer.ContextMenu.msix')
    Assert-TextViewerExit 'Context menu package signature'
    Copy-Item -LiteralPath 'shell\ContextMenu.ps1' -Destination $resources -Force
    go run github.com/egoist/mygo/cmd/mygo build -platform "windows/$Architecture" -o $OutputDirectory
    Assert-TextViewerExit 'MyGo application build'
    Write-Output "TextViewer build: $([IO.Path]::GetFullPath((Join-Path $root $OutputDirectory)))"
} finally {
    Pop-Location
}
