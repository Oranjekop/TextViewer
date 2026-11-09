param(
    [ValidateSet('Install', 'Remove', 'Status', 'Trust', 'Prepare', 'Locate', 'Cleanup')]
    [string]$Action = 'Install',
    [string]$AppDirectory = $PSScriptRoot,
    [string]$ExpectedThumbprint
)

function Get-TextViewerMenuMode {
    param([int]$Build = [Environment]::OSVersion.Version.Build)
    if ($Build -ge 22000) { return 'Modern' }
    return 'Classic'
}

function Set-TextViewerClassicMenu {
    param([string]$Directory, [string]$ClassesPath = 'HKCU:\Software\Classes')
    $command = Join-Path $ClassesPath '*\shell\TextViewer'
    $server = Join-Path $ClassesPath 'CLSID\{9DFEEF63-8B31-4BD2-85A0-A53687CE7C96}'
    foreach ($path in @($command, $server, "$server\InprocServer32")) {
        New-Item -Path $path -Force | Out-Null
    }
    $title = [string][char]0x5728 + ' TextViewer ' + [char]0x4e2d + [char]0x6253 + [char]0x5f00
    New-ItemProperty -LiteralPath $command -Name 'MUIVerb' -Value $title -PropertyType String -Force | Out-Null
    New-ItemProperty -LiteralPath $command -Name 'ExplorerCommandHandler' -Value '{9DFEEF63-8B31-4BD2-85A0-A53687CE7C96}' -PropertyType String -Force | Out-Null
    New-ItemProperty -LiteralPath $command -Name 'Icon' -Value ('"{0}",0' -f (Join-Path $Directory 'TextViewer.exe')) -PropertyType String -Force | Out-Null
    New-ItemProperty -LiteralPath $command -Name 'MultiSelectModel' -Value 'Player' -PropertyType String -Force | Out-Null
    foreach ($path in @($command, $server)) {
        New-ItemProperty -LiteralPath $path -Name 'TextViewerOwner' -Value 'TextViewer' -PropertyType String -Force | Out-Null
    }
    Set-Item -LiteralPath "$server\InprocServer32" -Value (Join-Path $Directory 'TextViewerShell.dll')
    New-ItemProperty -LiteralPath "$server\InprocServer32" -Name 'ThreadingModel' -Value 'Apartment' -PropertyType String -Force | Out-Null
}

function Test-TextViewerClassicMenu {
    param([string]$ClassesPath = 'HKCU:\Software\Classes')
    $command = Join-Path $ClassesPath '*\shell\TextViewer'
    if (-not (Test-Path -LiteralPath $command)) { return $false }
    $entry = Get-ItemProperty -LiteralPath $command
    return $entry.TextViewerOwner -eq 'TextViewer' -and $entry.ExplorerCommandHandler -eq '{9DFEEF63-8B31-4BD2-85A0-A53687CE7C96}'
}

function Remove-TextViewerClassicMenu {
    param([string]$ClassesPath = 'HKCU:\Software\Classes')
    foreach ($suffix in @('*\shell\TextViewer', 'CLSID\{9DFEEF63-8B31-4BD2-85A0-A53687CE7C96}')) {
        $path = Join-Path $ClassesPath $suffix
        if ((Test-Path -LiteralPath $path) -and (Get-ItemProperty -LiteralPath $path).TextViewerOwner -eq 'TextViewer') {
            Remove-Item -LiteralPath $path -Recurse -Force
        }
    }
}

function Enable-TextViewerModernMenu {
    param(
        [Parameter(Mandatory = $true)]
        [ValidatePattern('^[A-Za-z0-9.-]+_[a-z0-9]{13}$')]
        [string]$PackageFamilyName,
        [string]$PreferencesPath = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\ContextMenu\Preferences'
    )
    # With the 26H2 extensions submenu hidden, only individually enabled
    # packages appear. Opt in this package without changing global choices.
    if (-not (Test-Path -LiteralPath $PreferencesPath)) { return }
    $entry = Join-Path $PreferencesPath "extensions\$PackageFamilyName"
    if (-not (Test-Path -LiteralPath $entry)) {
        New-Item -Path $entry -Force | Out-Null
    }
    New-ItemProperty -LiteralPath $entry -Name 'Enabled' -Value 1 -PropertyType DWord -Force | Out-Null
}

function Update-TextViewerShell {
    if (-not ('TextViewer.ShellNotification' -as [type])) {
        Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
namespace TextViewer {
    public static class ShellNotification {
        [DllImport("shell32.dll")]
        public static extern void SHChangeNotify(uint eventId, uint flags, IntPtr item1, IntPtr item2);
    }
}
'@
    }
    [TextViewer.ShellNotification]::SHChangeNotify(0x08000000, 0, [IntPtr]::Zero, [IntPtr]::Zero)
}

function Read-TextViewerIdentity {
    param([string]$PackagePath)
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $archive = [IO.Compression.ZipFile]::OpenRead($PackagePath)
    try {
        $entry = $archive.GetEntry('AppxManifest.xml')
        if (-not $entry -or $entry.Length -gt 1MB) { throw 'Invalid identity manifest.' }
        $stream = $entry.Open()
        $settings = New-Object Xml.XmlReaderSettings
        $settings.DtdProcessing = [Xml.DtdProcessing]::Prohibit
        $reader = [Xml.XmlReader]::Create($stream, $settings)
        try {
            $document = New-Object Xml.XmlDocument
            $document.Load($reader)
            return $document.Package.Identity
        } finally { $reader.Dispose(); $stream.Dispose() }
    } finally { $archive.Dispose() }
}

function Get-TextViewerExistingIdentity {
    param($Existing, $Previous)
    if (-not $Existing -or -not $Previous -or -not $Previous.Directory) { return $null }
    $path = $Previous.PackagePath
    if (-not $path) { $path = Join-Path $Previous.Directory 'TextViewer.ContextMenu.msix' }
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { return $null }
    $identity = Read-TextViewerIdentity -PackagePath $path
    if ($identity.Name -ne $Existing.Name -or $identity.Publisher -ne $Existing.Publisher -or
        $identity.Version -ne [string]$Existing.Version -or
        $identity.ProcessorArchitecture -ne [string]$Existing.Architecture) {
        return $null
    }
    return [IO.Path]::GetFullPath($path)
}

function Test-TextViewerReusableIdentity {
    param([string]$PackagePath)
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $archive = [IO.Compression.ZipFile]::OpenRead($PackagePath)
    try {
        $entry = $archive.GetEntry('AppxManifest.xml')
        if (-not $entry -or $entry.Length -gt 1MB -or -not $archive.GetEntry('resources.pri')) { return $false }
        $settings = New-Object Xml.XmlReaderSettings
        $settings.DtdProcessing = [Xml.DtdProcessing]::Prohibit
        $stream = $entry.Open()
        $reader = [Xml.XmlReader]::Create($stream, $settings)
        try {
            $document = New-Object Xml.XmlDocument
            $document.Load($reader)
            $application = $document.SelectSingleNode('/*[local-name()="Package"]/*[local-name()="Applications"]/*[local-name()="Application"]')
            return $application -and $application.GetAttribute('RuntimeBehavior', 'http://schemas.microsoft.com/appx/manifest/uap/windows10/10') -eq 'win32App'
        } finally { $reader.Dispose(); $stream.Dispose() }
    } finally { $archive.Dispose() }
}

function Get-TextViewerExternalDirectory {
    param($Package)
    if (-not $Package) { return $null }
    [void][Windows.Management.Deployment.PackageManager,Windows.Management.Deployment,ContentType=WindowsRuntime]
    $manager = New-Object Windows.Management.Deployment.PackageManager
    $registered = $manager.FindPackageForUser('', $Package.PackageFullName)
    if (-not $registered -or -not $registered.EffectiveExternalPath) {
        throw 'Windows did not return the registered external application directory.'
    }
    return [IO.Path]::GetFullPath($registered.EffectiveExternalPath).TrimEnd('\')
}

function Set-TextViewerIdentity {
    param([string]$PackagePath, [string]$Directory, $Existing)
    $previousDirectory = Get-TextViewerExternalDirectory -Package $Existing
    $removed = $false
    try {
        try {
            Add-AppxPackage -Path $PackagePath -ExternalLocation $Directory -ErrorAction Stop
        } catch {
            $exception = $_.Exception
            $alreadyInstalled = $false
            while ($exception) {
                if ($exception.HResult -in @(0x80073CFB, 0x80073D0B) -or $exception.Message -match '\b0x80073(CFB|D0B)\b') {
                    $alreadyInstalled = $true
                }
                $exception = $exception.InnerException
            }
            if (-not $alreadyInstalled -or -not $Existing -or
                -not (Get-TextViewerExistingIdentity -Existing $Existing -Previous @{ Directory=$Directory; PackagePath=$PackagePath })) { throw }
            if ((Get-AuthenticodeSignature -LiteralPath $PackagePath -ErrorAction Stop).Status -ne 'Valid') {
                throw 'The existing signed identity archive could not be verified; the working registration was retained.'
            }
            # Windows rejects changing a registered sparse external location,
            # including with -Register. Reinstall only the same signed identity
            # and keep its valid archive/old directory available for rollback.
            $Existing | Remove-AppxPackage -ErrorAction Stop
            $removed = $true
            Add-AppxPackage -Path $PackagePath -ExternalLocation $Directory -ErrorAction Stop
        }
        $installed = Get-AppxPackage -Name 'Oranjekop.TextViewer.ContextMenu' -ErrorAction Stop
        $actual = Get-TextViewerExternalDirectory -Package $installed
        if (-not $installed -or -not [string]::Equals($actual, $Directory.TrimEnd('\'), [StringComparison]::OrdinalIgnoreCase)) {
            throw 'Windows did not bind the context menu to the current application directory.'
        }
        return $installed
    } catch {
        $installError = $_
        if ($removed) {
            try {
                $current = Get-AppxPackage -Name 'Oranjekop.TextViewer.ContextMenu' -ErrorAction Stop
                if ($current) {
                    if ($current.PackageFullName -ne $Existing.PackageFullName) {
                        throw 'Another context menu identity was registered during recovery.'
                    }
                    $current | Remove-AppxPackage -ErrorAction Stop
                }
                Add-AppxPackage -Path $PackagePath -ExternalLocation $previousDirectory -ErrorAction Stop
                $restored = Get-AppxPackage -Name 'Oranjekop.TextViewer.ContextMenu' -ErrorAction Stop
                $actual = Get-TextViewerExternalDirectory -Package $restored
                if (-not $restored -or -not [string]::Equals($actual, $previousDirectory, [StringComparison]::OrdinalIgnoreCase)) {
                    throw 'Windows did not restore the original external directory.'
                }
            } catch {
                throw "Context menu registration failed: $installError; restoring the previous directory also failed: $_"
            }
        }
        throw $installError
    }
}

function Copy-TextViewerImmutableFile {
    param([string]$Source, [string]$Destination)
    if ([string]::Equals($Source, $Destination, [StringComparison]::OrdinalIgnoreCase)) { return }
    if (Test-Path -LiteralPath $Destination) {
        if ((Get-Item -LiteralPath $Destination).Attributes -band [IO.FileAttributes]::ReparsePoint) {
            throw 'An immutable file must not be a link.'
        }
        if ((Get-FileHash -LiteralPath $Source).Hash -ne (Get-FileHash -LiteralPath $Destination).Hash) {
            throw 'This immutable install directory contains different files. Rebuild with a new application version.'
        }
    } else {
        Copy-Item -LiteralPath $Source -Destination $Destination
    }
}

function Expand-TextViewerVisualAssets {
    param([string]$PackagePath, [string]$Directory)
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $assets = Join-Path $Directory 'Assets'
    New-Item -ItemType Directory -Path $assets -Force | Out-Null
    if ((Get-Item -LiteralPath $assets).Attributes -band [IO.FileAttributes]::ReparsePoint) {
        throw 'The application asset directory must not be a link.'
    }
    $archive = [IO.Compression.ZipFile]::OpenRead($PackagePath)
    try {
        $names = @('Logo.png', 'Logo44.png', 'FileTypeText.png', 'resources.pri')
        foreach ($entry in $archive.Entries) {
            if ($entry.FullName -cmatch '^Assets[/\\](Logo44\.targetsize-(16|20|24|30|32|36|40|48|60|64|72|80|96|256)(_altform-(lightunplated|unplated))?\.png)$') {
                $names += $Matches[1]
            }
        }
        foreach ($name in $names) {
            $entry = $archive.GetEntry("Assets/$name")
            if (-not $entry) { $entry = $archive.GetEntry("Assets\$name") }
            if ($name -eq 'resources.pri') { $entry = $archive.GetEntry($name) }
            if (-not $entry) {
                if ($name -in @('FileTypeText.png', 'resources.pri')) { continue } # Older identities.
                throw "Missing visual asset: $name"
            }
            if ($entry.Length -gt 2MB) { throw 'An application visual asset is too large.' }
            $stream = $entry.Open()
            $memory = New-Object IO.MemoryStream
            try { $stream.CopyTo($memory); $bytes = $memory.ToArray() }
            finally { $stream.Dispose(); $memory.Dispose() }
            $target = Join-Path $assets $name
            if ($name -eq 'resources.pri') { $target = Join-Path $Directory $name }
            if (Test-Path -LiteralPath $target) {
                if ((Get-Item -LiteralPath $target).Attributes -band [IO.FileAttributes]::ReparsePoint -or
                    [Convert]::ToBase64String([IO.File]::ReadAllBytes($target)) -cne [Convert]::ToBase64String($bytes)) {
                    throw 'This immutable visual asset has changed.'
                }
            } else { [IO.File]::WriteAllBytes($target, $bytes) }
        }
    } finally { $archive.Dispose() }
}

function Remove-TextViewerOldBundles {
    param(
        [string]$InstallRoot,
        [string[]]$ProtectedPaths = @(),
        [string[]]$RunningPaths = @()
    )
    if (-not (Test-Path -LiteralPath $InstallRoot -PathType Container)) { return }
    $root = [IO.Path]::GetFullPath($InstallRoot).TrimEnd('\')
    if ((Get-Item -LiteralPath $root).Attributes -band [IO.FileAttributes]::ReparsePoint) {
        throw 'The cleanup root must not be a link.'
    }
    $known = @('TextViewer.exe', 'TextViewerShell.dll', 'TextViewer.ContextMenu.msix',
        'TextViewer.ContextMenu.cer', 'ContextMenu.ps1', 'resources.pri',
        'FileIcon-text.ico', 'FileIcon-markdown.ico', 'FileIcon-code.ico',
        'FileIcon-data.ico', 'FileIcon-config.ico', 'FileIcon-script.ico')
    foreach ($directory in Get-ChildItem -LiteralPath $root -Directory) {
        if ($directory.Name -cnotmatch '^app-[a-f0-9]{16}$' -or
            $directory.Attributes -band [IO.FileAttributes]::ReparsePoint) { continue }
        $target = [IO.Path]::GetFullPath($directory.FullName).TrimEnd('\')
        if (-not [string]::Equals([IO.Path]::GetDirectoryName($target), $root, [StringComparison]::OrdinalIgnoreCase)) {
            throw 'Cleanup target escaped the installation root.'
        }
        $keep = $false
        foreach ($path in @($ProtectedPaths) + @($RunningPaths)) {
            if (-not $path) { continue }
            $full = [IO.Path]::GetFullPath($path).TrimEnd('\')
            if ($full.Equals($target, [StringComparison]::OrdinalIgnoreCase) -or
                $full.StartsWith("$target\", [StringComparison]::OrdinalIgnoreCase)) { $keep = $true; break }
        }
        if ($keep -or -not (Test-Path -LiteralPath (Join-Path $target 'TextViewer.exe') -PathType Leaf)) { continue }
        $files = @()
        foreach ($item in Get-ChildItem -LiteralPath $target -Force) {
            if ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) { $keep = $true; break }
            if ($item.PSIsContainer) {
                if ($item.Name -ne 'Assets') { $keep = $true; break }
                foreach ($asset in Get-ChildItem -LiteralPath $item.FullName -Force) {
                    if ($asset.PSIsContainer -or $asset.Attributes -band [IO.FileAttributes]::ReparsePoint -or
                        ($asset.Name -notin @('Logo.png', 'Logo44.png', 'FileTypeText.png') -and
                        $asset.Name -cnotmatch '^Logo44\.targetsize-(16|20|24|30|32|36|40|48|60|64|72|80|96|256)(_altform-(lightunplated|unplated))?\.png$')) { $keep = $true; break }
                    $files += $asset.FullName
                }
            } elseif ($item.Name -in $known -or $item.Name -cmatch '^TrustedIdentity-[a-f0-9]{16}\.msix$') {
                $files += $item.FullName
            } else { $keep = $true; break }
        }
        if ($keep) { continue } # Keep a directory containing any user-created file.
        $locks = @()
        try {
            foreach ($file in $files) {
                $locks += [IO.File]::Open($file, [IO.FileMode]::Open, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
            }
        } catch { $keep = $true }
        finally { foreach ($lock in $locks) { $lock.Dispose() } }
        if ($keep) { continue } # A running editor or loaded shell DLL keeps its bundle.
        try {
            foreach ($file in $files) { Remove-Item -LiteralPath $file -Force -ErrorAction Stop }
            $assets = Join-Path $target 'Assets'
            if (Test-Path -LiteralPath $assets) { [IO.Directory]::Delete($assets, $false) }
            [IO.Directory]::Delete($target, $false)
            Write-Output "CLEANED $target"
        } catch { Write-Verbose "Retained busy bundle: $target" }
    }
}

if ($MyInvocation.InvocationName -eq '.') { return }

$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)
$packageName = 'Oranjekop.TextViewer.ContextMenu'
$modern = (Get-TextViewerMenuMode) -eq 'Modern'
$stateRoot = Join-Path $env:LOCALAPPDATA 'TextViewer\Integration'
$statePath = Join-Path $stateRoot 'context-menu.json'
$legacyState = Join-Path $env:LOCALAPPDATA 'Programs\TextViewer\context-menu.json'
$existing = $null
if ($modern -and $Action -notin @('Prepare', 'Locate')) {
    $existing = Get-AppxPackage -Name $packageName
}
if ($Action -eq 'Status') {
    if ($existing -or (Test-TextViewerClassicMenu)) { Write-Output 'INSTALLED' } else { Write-Output 'ABSENT' }
    exit 0
}
if ($Action -eq 'Remove') {
    if ($existing) { $existing | Remove-AppxPackage -ErrorAction Stop }
    Remove-TextViewerClassicMenu
    foreach ($path in @($statePath, $legacyState)) {
        if (Test-Path -LiteralPath $path) { Remove-Item -LiteralPath $path }
    }
    Update-TextViewerShell
    Write-Output 'REMOVED'
    exit 0
}
$source = (Resolve-Path -LiteralPath $AppDirectory).Path
$installRoot = Join-Path $stateRoot 'bundles'
$previous = $null
if (Test-Path -LiteralPath $statePath) {
    $previous = Get-Content -LiteralPath $statePath -Raw | ConvertFrom-Json
} elseif (Test-Path -LiteralPath $legacyState) {
    $previous = Get-Content -LiteralPath $legacyState -Raw | ConvertFrom-Json
}
if ($Action -eq 'Cleanup') {
    $protected = @($source)
    if ($existing) { $protected += Get-TextViewerExternalDirectory -Package $existing }
    if ($previous) { $protected += @($previous.Directory, $previous.PackagePath) }
    $associations = 'HKCU:\Software\Oranjekop\TextViewer\FileAssociations'
    if (Test-Path -LiteralPath $associations) {
        $registered = Get-ItemProperty -LiteralPath $associations
        $protected += @($registered.Directory, $registered.Source)
    }
    $runningPaths = @(foreach ($process in @(Get-Process -Name TextViewer -ErrorAction SilentlyContinue)) {
        try {
            $path = $process.Path
            if ($path) { $path }
            elseif (Get-Process -Id $process.Id -ErrorAction SilentlyContinue) {
                throw 'Unable to identify a running TextViewer; old versions were retained.'
            }
        } finally { $process.Dispose() }
    })
    foreach ($root in @($installRoot, (Join-Path $env:LOCALAPPDATA 'Programs\TextViewer'))) {
        Remove-TextViewerOldBundles -InstallRoot $root -ProtectedPaths $protected -RunningPaths $runningPaths
    }
    exit 0
}
$previousPackage = $null
$reuseIdentity = $false
# File associations also use the immutable bundle, independently of MSIX trust.
if ($modern -and $Action -in @('Install', 'Trust')) {
    $certificatePath = Join-Path $source 'TextViewer.ContextMenu.cer'
    $certificate = New-Object Security.Cryptography.X509Certificates.X509Certificate2($certificatePath)
    if ($Action -eq 'Trust') {
        if (-not $ExpectedThumbprint -or $certificate.Thumbprint -ne $ExpectedThumbprint) {
            throw 'The package certificate changed. Cancel and retry from TextViewer.'
        }
        $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
        $principal = New-Object Security.Principal.WindowsPrincipal($identity)
        if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
            $powerShell = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
            $arguments = '-NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "{0}" -Action Trust -AppDirectory "{1}" -ExpectedThumbprint "{2}"' -f $PSCommandPath, $source, $ExpectedThumbprint
            $process = Start-Process -FilePath $powerShell -ArgumentList $arguments -Verb RunAs -WindowStyle Hidden -PassThru -Wait
            if ($process.ExitCode -ne 0) { throw 'Windows did not trust the TextViewer certificate.' }
            exit 0
        }
        # Trust this end-entity signer only after the app's explicit confirmation
        # and the Windows administrator prompt. Never install a root CA.
        $constraints = $certificate.Extensions | Where-Object {
            $_ -is [Security.Cryptography.X509Certificates.X509BasicConstraintsExtension]
        }
        if ($constraints -and $constraints.CertificateAuthority) { throw 'A root CA certificate is not permitted.' }
        Import-Certificate -FilePath $certificatePath -CertStoreLocation 'Cert:\LocalMachine\TrustedPeople' | Out-Null
        Write-Output 'TRUSTED'
        exit 0
    }
    $chain = New-Object Security.Cryptography.X509Certificates.X509Chain
    try {
        $chain.ChainPolicy.RevocationMode = [Security.Cryptography.X509Certificates.X509RevocationMode]::NoCheck
        $trusted = (Test-Path -LiteralPath "Cert:\LocalMachine\TrustedPeople\$($certificate.Thumbprint)") -or $chain.Build($certificate)
    } finally {
        $chain.Dispose()
    }
    if ($existing) {
        $actualDirectory = Get-TextViewerExternalDirectory -Package $existing
        if (-not $previous) { $previous = @{ Directory = $actualDirectory } }
        $previousPackage = Get-TextViewerExistingIdentity -Existing $existing -Previous $previous
        if (-not $previousPackage) {
            $previousPackage = Get-TextViewerExistingIdentity -Existing $existing -Previous @{ Directory = $actualDirectory }
        }
    }
    if (-not $trusted) {
        if (-not $previousPackage -or -not (Test-TextViewerReusableIdentity -PackagePath $previousPackage)) {
            throw 'TEXTVIEWER_CERTIFICATE_TRUST_REQUIRED'
        }
        # Sparse identities authorize external content updates. Retain the
        # already registered signed identity instead of leaving its old EXE active.
        $reuseIdentity = $true
    }
}
$required = @('TextViewer.exe', 'TextViewerShell.dll', 'TextViewer.ContextMenu.msix', 'TextViewer.ContextMenu.cer', 'ContextMenu.ps1')
foreach ($name in $required) {
    if (-not (Test-Path -LiteralPath (Join-Path $source $name) -PathType Leaf)) {
        throw "Missing build file: $name"
    }
}
# An immutable install directory keeps the command valid when the original
# portable folder is moved and avoids replacing a DLL currently in dllhost.
# The single EXE contains the other four files, so its digest identifies all
# integration resources and agrees with the native extraction code.
$digest = (Get-FileHash -LiteralPath (Join-Path $source 'TextViewer.exe') -Algorithm SHA256).Hash.Substring(0, 16).ToLowerInvariant()
$destination = Join-Path $installRoot "app-$digest"
if ($Action -eq 'Locate') {
    Write-Output "PREPARED $destination"
    exit 0
}
New-Item -ItemType Directory -Path $destination -Force | Out-Null
if ((Get-Item -LiteralPath $destination).Attributes -band [IO.FileAttributes]::ReparsePoint) {
    throw 'The immutable application directory must not be a link.'
}
foreach ($name in $required) {
    $from = Join-Path $source $name
    $to = Join-Path $destination $name
    Copy-TextViewerImmutableFile -Source $from -Destination $to
}
# Sparse package visual resources are resolved against the external directory.
Expand-TextViewerVisualAssets -PackagePath (Join-Path $destination 'TextViewer.ContextMenu.msix') -Directory $destination
if ($Action -eq 'Prepare') {
    Write-Output "PREPARED $destination"
    exit 0
}
if (-not $modern) {
    Set-TextViewerClassicMenu -Directory $destination
    $state = @{ Directory = $destination; Mode = 'Classic' }
    [IO.File]::WriteAllText($statePath, ($state | ConvertTo-Json), [Text.UTF8Encoding]::new($false))
    if (Test-Path -LiteralPath $legacyState) { Remove-Item -LiteralPath $legacyState }
    Update-TextViewerShell
    Write-Output "INSTALLED $destination"
    exit 0
}
$packagePath = Join-Path $destination 'TextViewer.ContextMenu.msix'
if ($reuseIdentity) {
    $identityDigest = (Get-FileHash -LiteralPath $previousPackage).Hash.Substring(0, 16).ToLowerInvariant()
    $packagePath = Join-Path $destination "TrustedIdentity-$identityDigest.msix"
    Copy-TextViewerImmutableFile -Source $previousPackage -Destination $packagePath
}
if ($existing -and $actualDirectory -eq $destination -and
    (Get-TextViewerExistingIdentity -Existing $existing -Previous @{Directory=$destination;PackagePath=$packagePath})) {
    $installed = $existing
} else {
    $installed = Set-TextViewerIdentity -PackagePath $packagePath -Directory $destination -Existing $existing
}
$state = @{ Directory = $destination; PackagePath = $packagePath; PackageVersion = [string]$installed.Version }
$temporaryState = "$statePath.$([Guid]::NewGuid().ToString('N')).tmp"
try {
    [IO.File]::WriteAllText($temporaryState, ($state | ConvertTo-Json), [Text.UTF8Encoding]::new($false))
    Move-Item -LiteralPath $temporaryState -Destination $statePath -Force
} finally { if (Test-Path -LiteralPath $temporaryState) { Remove-Item -LiteralPath $temporaryState } }
Enable-TextViewerModernMenu -PackageFamilyName $installed.PackageFamilyName
Remove-TextViewerClassicMenu
if (Test-Path -LiteralPath $legacyState) { Remove-Item -LiteralPath $legacyState }
Update-TextViewerShell
Write-Output "INSTALLED $destination"
