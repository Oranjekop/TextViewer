param(
    [ValidateSet('Install', 'Remove', 'Status', 'Trust', 'Prepare', 'Locate', 'Cleanup')]
    [string]$Action = 'Install',
    [string]$AppDirectory = $PSScriptRoot,
    [string]$ExpectedThumbprint
)

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
        foreach ($name in @('Logo.png', 'Logo44.png', 'FileTypeText.png')) {
            $entry = $archive.GetEntry("Assets/$name")
            if (-not $entry) { $entry = $archive.GetEntry("Assets\$name") }
            if (-not $entry) {
                if ($name -eq 'FileTypeText.png') { continue } # Older signed identities use Logo44.
                throw "Missing visual asset: $name"
            }
            if ($entry.Length -gt 2MB) { throw 'An application visual asset is too large.' }
            $stream = $entry.Open()
            $memory = New-Object IO.MemoryStream
            try { $stream.CopyTo($memory); $bytes = $memory.ToArray() }
            finally { $stream.Dispose(); $memory.Dispose() }
            $target = Join-Path $assets $name
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
        'TextViewer.ContextMenu.cer', 'ContextMenu.ps1',
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
                        $asset.Name -notin @('Logo.png', 'Logo44.png', 'FileTypeText.png')) { $keep = $true; break }
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
$existing = $null
if ($Action -notin @('Prepare', 'Locate')) {
    $existing = Get-AppxPackage -Name $packageName
}
if ($Action -eq 'Status') {
    if ($existing) { Write-Output 'INSTALLED' } else { Write-Output 'ABSENT' }
    exit 0
}
if ($Action -eq 'Remove') {
    if ($existing) { $existing | Remove-AppxPackage -ErrorAction Stop }
    $oldState = Join-Path $env:LOCALAPPDATA 'Programs\TextViewer\context-menu.json'
    if (Test-Path -LiteralPath $oldState) { Remove-Item -LiteralPath $oldState }
    Update-TextViewerShell
    Write-Output 'REMOVED'
    exit 0
}
$source = (Resolve-Path -LiteralPath $AppDirectory).Path
$installRoot = Join-Path $env:LOCALAPPDATA 'Programs\TextViewer'
$statePath = Join-Path $installRoot 'context-menu.json'
$previous = $null
if (Test-Path -LiteralPath $statePath) {
    $previous = Get-Content -LiteralPath $statePath -Raw | ConvertFrom-Json
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
    $running = @(Get-CimInstance Win32_Process -Filter "Name = 'TextViewer.exe'")
    if (@($running | Where-Object { -not $_.ExecutablePath }).Count -gt 0) {
        throw 'Unable to identify a running TextViewer; old versions were retained.'
    }
    Remove-TextViewerOldBundles -InstallRoot $installRoot -ProtectedPaths $protected -RunningPaths @($running.ExecutablePath)
    exit 0
}
$previousPackage = $null
$reuseIdentity = $false
# File associations also use the immutable bundle, independently of MSIX trust.
if ($Action -in @('Install', 'Trust')) {
    if ([Environment]::OSVersion.Version.Build -lt 22000) {
        throw 'The modern context menu requires Windows 11.'
    }
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
        if (-not $previousPackage) { throw 'TEXTVIEWER_CERTIFICATE_TRUST_REQUIRED' }
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
$hashes = foreach ($name in $required) {
    (Get-FileHash -LiteralPath (Join-Path $source $name) -Algorithm SHA256).Hash
}
$sha = [Security.Cryptography.SHA256]::Create()
try {
    $hash = $sha.ComputeHash([Text.Encoding]::UTF8.GetBytes(($hashes -join '|')))
    $digest = [BitConverter]::ToString($hash).Replace('-', '').Substring(0, 16).ToLowerInvariant()
} finally {
    $sha.Dispose()
}
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
Update-TextViewerShell
Write-Output "INSTALLED $destination"
