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
