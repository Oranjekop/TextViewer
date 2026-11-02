function Get-TextViewerTools {
    param([switch]$Bootstrap)
    $root = Split-Path -Parent $PSScriptRoot
    $cache = Join-Path $root '.cache'
    $llvmRoot = Join-Path $cache 'llvm-mingw-20261020-ucrt-x86_64'
    $sdkRoot = Join-Path $cache 'windows-sdk'
    $clang = Join-Path $llvmRoot 'bin\clang++.exe'
    $makeAppx = Join-Path $sdkRoot 'bin\10.0.26100.0\x64\makeappx.exe'
    if ($env:LLVM_MINGW_ROOT) {
        $clang = Join-Path $env:LLVM_MINGW_ROOT 'bin\clang++.exe'
    }
    if ($env:MAKEAPPX_PATH) { $makeAppx = $env:MAKEAPPX_PATH }

    function Get-VerifiedArchive {
        param([string]$Url, [string]$Name, [string]$SHA256, [string]$Destination)
        New-Item -ItemType Directory -Path $cache -Force | Out-Null
        $archive = Join-Path $cache $Name
        if (-not (Test-Path -LiteralPath $archive)) {
            Invoke-WebRequest -Uri $Url -OutFile $archive -UseBasicParsing
        }
        $actual = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash
        if (-not [string]::Equals($actual, $SHA256, [StringComparison]::OrdinalIgnoreCase)) {
            throw "Checksum mismatch: $archive"
        }
        Expand-Archive -LiteralPath $archive -DestinationPath $Destination -Force
    }

    if (-not (Test-Path -LiteralPath $clang)) {
        if (-not $Bootstrap -or $env:LLVM_MINGW_ROOT) {
            throw 'LLVM-MinGW is missing. Run scripts/build.ps1 -BootstrapTools or set LLVM_MINGW_ROOT.'
        }
        Get-VerifiedArchive `
            -Url 'https://github.com/mstorsjo/llvm-mingw/releases/download/20261020/llvm-mingw-20261020-ucrt-x86_64.zip' `
            -Name 'llvm-mingw-20261020-ucrt-x86_64.zip' `
            -SHA256 'fd5462b7f5d93b2ff981fcd3f9c7e427e47f3203c64ea91c27bcc444eebd74cd' `
            -Destination $cache
    }
    if (-not (Test-Path -LiteralPath $makeAppx)) {
        if (-not $Bootstrap -or $env:MAKEAPPX_PATH) {
            throw 'MakeAppx is missing. Run scripts/build.ps1 -BootstrapTools or set MAKEAPPX_PATH.'
        }
        Get-VerifiedArchive `
            -Url 'https://api.nuget.org/v3-flatcontainer/microsoft.windows.sdk.buildtools/10.0.26100.9457/microsoft.windows.sdk.buildtools.10.0.26100.9457.nupkg' `
            -Name 'windows-sdk-buildtools-10.0.26100.9457.zip' `
            -SHA256 '4c2e0d85f3490e7f7c065c35b4c47fe913b1bc9627e9711ae349b2cdc7edc791' `
            -Destination $sdkRoot
    }
    return @{ Clang = $clang; MakeAppx = $makeAppx }
}

function Assert-TextViewerExit {
    param([string]$Step)
    if ($LASTEXITCODE -ne 0) {
        throw "$Step failed with exit code $LASTEXITCODE."
    }
}

function Get-TextViewerNSIS {
    param([switch]$Bootstrap)
    if ($env:MAKENSIS -and (Test-Path -LiteralPath $env:MAKENSIS)) { return $env:MAKENSIS }
    $installed = Get-Command makensis -ErrorAction SilentlyContinue
    if ($installed) { return $installed.Source }
    $root = Split-Path -Parent $PSScriptRoot
    $cache = Join-Path $root '.cache'
    $directories = @((Join-Path $cache 'nsis-3.13'))
    foreach ($base in @(${env:ProgramFiles(x86)}, $env:ProgramFiles)) {
        if ($base) { $directories += Join-Path $base 'NSIS' }
    }
    $directories += @(Get-ChildItem -Path (Join-Path $env:LOCALAPPDATA 'mygo\nsis-*') -Directory -ErrorAction SilentlyContinue | Select-Object -ExpandProperty FullName)
    foreach ($directory in $directories) {
        foreach ($name in @('makensis.exe', 'Bin\makensis.exe')) {
            $tool = Join-Path $directory $name
            if (Test-Path -LiteralPath $tool) { return $tool }
        }
    }
    if (-not $Bootstrap) { throw 'NSIS is missing. Run scripts/build-release.ps1 -BootstrapTools or set MAKENSIS.' }
    New-Item -ItemType Directory -Path $cache -Force | Out-Null
    $archive = Join-Path $cache 'nsis-3.13.zip'
    if (-not (Test-Path -LiteralPath $archive)) {
        $urls = @('https://github.com/egoist/mygo/releases/download/nsis-3.13/nsis-3.13.zip',
            'https://downloads.sourceforge.net/project/nsis/NSIS%203/3.13/nsis-3.13.zip')
        foreach ($url in $urls) {
            try { Invoke-WebRequest -Uri $url -OutFile $archive -UseBasicParsing; break }
            catch { if ($url -eq $urls[-1]) { throw } }
        }
    }
    $expected = 'ba63dffc4410ee89193e1cb5a41989991bd77c61068da17e3156d136b7b0b3d8'
    if ((Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant() -ne $expected) {
        throw 'NSIS archive checksum mismatch.'
    }
    Expand-Archive -LiteralPath $archive -DestinationPath $cache -Force
    foreach ($name in @('makensis.exe', 'Bin\makensis.exe')) {
        $tool = Join-Path (Join-Path $cache 'nsis-3.13') $name
        if (Test-Path -LiteralPath $tool) { return $tool }
    }
    throw 'The verified NSIS archive contains no compiler.'
}
