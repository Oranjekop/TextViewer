param(
    [ValidateSet('Install', 'Remove', 'Status', 'Trust', 'Prepare', 'Locate')]
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
    Update-TextViewerShell
    Write-Output 'REMOVED'
    exit 0
}
$source = (Resolve-Path -LiteralPath $AppDirectory).Path
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
    if (-not $trusted) { throw 'TEXTVIEWER_CERTIFICATE_TRUST_REQUIRED' }
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
$installRoot = Join-Path $env:LOCALAPPDATA 'Programs\TextViewer'
$destination = Join-Path $installRoot "app-$digest"
if ($Action -eq 'Locate') {
    Write-Output "PREPARED $destination"
    exit 0
}
New-Item -ItemType Directory -Path $destination -Force | Out-Null
foreach ($name in $required) {
    $from = Join-Path $source $name
    $to = Join-Path $destination $name
    if (-not [string]::Equals($from, $to, [StringComparison]::OrdinalIgnoreCase)) {
        if (-not (Test-Path -LiteralPath $to)) {
            Copy-Item -LiteralPath $from -Destination $to
        } elseif ((Get-FileHash -LiteralPath $from).Hash -ne (Get-FileHash -LiteralPath $to).Hash) {
            throw 'This immutable install directory contains different files. Rebuild with a new application version.'
        }
    }
}
if ($Action -eq 'Prepare') {
    Write-Output "PREPARED $destination"
    exit 0
}
$packagePath = Join-Path $destination 'TextViewer.ContextMenu.msix'
$statePath = Join-Path $installRoot 'context-menu.json'
$previous = $null
if (Test-Path -LiteralPath $statePath) {
    $previous = Get-Content -LiteralPath $statePath -Raw | ConvertFrom-Json
}
if ($existing -and $previous -and $previous.Directory -eq $destination) {
    Enable-TextViewerModernMenu -PackageFamilyName $existing.PackageFamilyName
    Update-TextViewerShell
    Write-Output "INSTALLED $destination"
    exit 0
}
try {
    if ($existing) { $existing | Remove-AppxPackage -ErrorAction Stop }
    Add-AppxPackage -Path $packagePath -ExternalLocation $destination -ErrorAction Stop
} catch {
    $installError = $_
    if ($existing -and $previous) {
        $oldPackage = Join-Path $previous.Directory 'TextViewer.ContextMenu.msix'
        if (Test-Path -LiteralPath $oldPackage) {
            try { Add-AppxPackage -Path $oldPackage -ExternalLocation $previous.Directory -ErrorAction Stop } catch { }
        }
    }
    throw $installError
}
$installed = Get-AppxPackage -Name $packageName
if (-not $installed) { throw 'Windows did not register the context menu package.' }
[IO.File]::WriteAllText($statePath, (@{ Directory = $destination } | ConvertTo-Json), [Text.UTF8Encoding]::new($false))
Enable-TextViewerModernMenu -PackageFamilyName $installed.PackageFamilyName
Update-TextViewerShell
Write-Output "INSTALLED $destination"
