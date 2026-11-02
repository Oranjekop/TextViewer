$ErrorActionPreference = 'Stop'
. (Join-Path (Split-Path -Parent $PSScriptRoot) 'shell\ContextMenu.ps1')

function Assert-MenuPreference {
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) { throw $Message }
}

$testRoot = "HKCU:\Software\TextViewer.ContextMenu.Tests.$([Guid]::NewGuid().ToString('N'))"
$preferences = Join-Path $testRoot 'Preferences'
$family = 'Oranjekop.TextViewer.ContextMenu_0123456789abc'
$ownEntry = Join-Path $preferences "extensions\$family"
$otherEntry = Join-Path $preferences 'extensions\Other.App_0123456789abc'
try {
    Assert-MenuPreference ((Get-TextViewerMenuMode -Build 10240) -eq 'Classic') 'Windows 10 uses its native context menu without MSIX trust.'
    Assert-MenuPreference ((Get-TextViewerMenuMode -Build 19045) -eq 'Classic') 'Windows 10 22H2 uses its native context menu.'
    Assert-MenuPreference ((Get-TextViewerMenuMode -Build 22000) -eq 'Modern') 'Windows 11 uses the modern Explorer menu.'
    $classes = Join-Path $testRoot 'Classes'
    $classicDirectory = 'C:\TextViewer fixture & ' + [char]0x4e2d + [char]0x6587
    Set-TextViewerClassicMenu -Directory $classicDirectory -ClassesPath $classes
    Assert-MenuPreference (Test-TextViewerClassicMenu -ClassesPath $classes) 'Register the Windows 10 Explorer command.'
    $server = Join-Path $classes 'CLSID\{9DFEEF63-8B31-4BD2-85A0-A53687CE7C96}\InprocServer32'
    Assert-MenuPreference ((Get-Item -LiteralPath $server).GetValue('') -eq "$classicDirectory\TextViewerShell.dll") 'Bind COM to the immutable DLL with Unicode and spaces.'
    Assert-MenuPreference ((Get-ItemProperty -LiteralPath $server).ThreadingModel -eq 'Apartment') 'Use the shell extension apartment model.'
    $other = Join-Path $classes '*\shell\OtherApp'
    New-Item -Path $other -Force | Out-Null
    Remove-TextViewerClassicMenu -ClassesPath $classes
    Assert-MenuPreference (-not (Test-TextViewerClassicMenu -ClassesPath $classes)) 'Remove the Windows 10 menu.'
    Assert-MenuPreference (Test-Path -LiteralPath $other) 'Preserve unrelated context menu entries.'
    Write-Output 'Windows 10 context menu registration tests passed.'

    # Older Windows versions have no customization preferences to update.
    Enable-TextViewerModernMenu -PackageFamilyName $family -PreferencesPath $preferences
    Assert-MenuPreference (-not (Test-Path -LiteralPath $preferences)) 'Do not create preferences on older Windows.'

    New-Item -Path $otherEntry -Force | Out-Null
    New-ItemProperty -LiteralPath $otherEntry -Name 'Enabled' -Value 0 -PropertyType DWord | Out-Null
    foreach ($name in @('ShowExtensions', 'ShowMore', 'DemoteTopBar')) {
        $key = Join-Path $preferences $name
        New-Item -Path $key -Force | Out-Null
        New-ItemProperty -LiteralPath $key -Name 'Enabled' -Value 0 -PropertyType DWord | Out-Null
    }

    Enable-TextViewerModernMenu -PackageFamilyName $family -PreferencesPath $preferences
    $own = Get-Item -LiteralPath $ownEntry
    Assert-MenuPreference ($own.GetValue('Enabled') -eq 1) 'The package must be individually enabled.'
    Assert-MenuPreference ($own.GetValueKind('Enabled') -eq [Microsoft.Win32.RegistryValueKind]::DWord) 'Enabled must be a DWORD.'

    # Explicitly adding an already registered but hidden command repairs it.
    Set-ItemProperty -LiteralPath $ownEntry -Name 'Enabled' -Value 0
    New-ItemProperty -LiteralPath $ownEntry -Name 'OtherPreference' -Value 7 -PropertyType DWord | Out-Null
    Enable-TextViewerModernMenu -PackageFamilyName $family -PreferencesPath $preferences
    Enable-TextViewerModernMenu -PackageFamilyName $family -PreferencesPath $preferences
    Assert-MenuPreference ((Get-ItemProperty -LiteralPath $ownEntry).Enabled -eq 1) 'Repeated additions must enable the package.'
    Assert-MenuPreference ((Get-ItemProperty -LiteralPath $ownEntry).OtherPreference -eq 7) 'Preserve other package preferences.'
    Assert-MenuPreference ((Get-ItemProperty -LiteralPath $otherEntry).Enabled -eq 0) 'Other applications must remain unchanged.'
    foreach ($name in @('ShowExtensions', 'ShowMore', 'DemoteTopBar')) {
        $key = Join-Path $preferences $name
        Assert-MenuPreference ((Get-ItemProperty -LiteralPath $key).Enabled -eq 0) "Preserve the global $name choice."
    }
    Write-Output 'Modern context menu preference tests passed.'
} finally {
    if (Test-Path -LiteralPath $testRoot) {
        Remove-Item -LiteralPath $testRoot -Recurse -Force
    }
}

function New-IdentityFixture {
    param([string]$Path, [string]$Version, [switch]$Modern)
    $archive = [IO.Compression.ZipFile]::Open($Path, [IO.Compression.ZipArchiveMode]::Create)
    try {
        $entry = $archive.CreateEntry('AppxManifest.xml')
        $writer = New-Object IO.StreamWriter($entry.Open())
        try {
            $application = ''
            if ($Modern) {
                $application = '<Applications><Application xmlns:uap10="http://schemas.microsoft.com/appx/manifest/uap/windows10/10" uap10:RuntimeBehavior="win32App" /></Applications>'
            }
            $writer.Write(('<Package><Identity Name="Oranjekop.TextViewer.ContextMenu" Publisher="CN=Fixture" Version="{0}" ProcessorArchitecture="x64" />{1}</Package>' -f $Version, $application))
        } finally { $writer.Dispose() }
        if ($Modern) {
            $entry = $archive.CreateEntry('resources.pri')
            $writer = New-Object IO.StreamWriter($entry.Open())
            try { $writer.Write('fixture index') } finally { $writer.Dispose() }
        }
        foreach ($name in @('Logo.png', 'Logo44.png', 'FileTypeText.png', 'Logo44.targetsize-48_altform-unplated.png', 'Logo44.targetsize-48_altform-lightunplated.png')) {
            $entry = $archive.CreateEntry("Assets/$name")
            $writer = New-Object IO.StreamWriter($entry.Open())
            try { $writer.Write("fixture $name") } finally { $writer.Dispose() }
        }
    } finally { $archive.Dispose() }
}

function Test-IdentityRegistration {
    param([string]$PackagePath, [string]$Directory)
    $script:registrationCalls = @()
    $script:registrationRemovals = 0
    $script:registrationMode = 'already-installed'
    $script:registrationSignature = 'Valid'
    $script:registeredDirectory = Join-Path $Directory 'old'
    $script:registeredPackage = [pscustomobject]@{
        Name='Oranjekop.TextViewer.ContextMenu'; Publisher='CN=Fixture'
        Version=[version]'0.0.1.0'; Architecture='X64'
        PackageFullName='fixture'; InstallLocation=$Directory
    }
    function Get-TextViewerExternalDirectory { param($Package) return $script:registeredDirectory }
    function Get-AuthenticodeSignature {
        param($LiteralPath, $ErrorAction)
        return [pscustomobject]@{Status=$script:registrationSignature}
    }
    function Get-AppxPackage {
        param($Name, $ErrorAction)
        if ($script:registeredDirectory) { return $script:registeredPackage }
    }
    function Remove-AppxPackage {
        param([Parameter(ValueFromPipeline=$true)]$Package)
        $script:registrationRemovals++
        $script:registeredDirectory = $null
    }
    function Add-AppxPackage {
        param($Path, $ExternalLocation, $ErrorAction)
        $script:registrationCalls += @{ Directory=$ExternalLocation }
        if ($script:registeredDirectory -and $script:registrationMode -eq 'different-location') {
            throw [Runtime.InteropServices.COMException]::new('Already installed at a different external location.', 0x80073D0B)
        }
        if ($script:registeredDirectory -and $script:registrationMode -in @('already-installed', 'rebind-failure')) {
            throw [Runtime.InteropServices.COMException]::new('Package is already installed.', -2147009285)
        }
        if ($script:registrationMode -eq 'unrelated-error') { throw 'Unrelated deployment failure.' }
        $script:registeredDirectory = $ExternalLocation
        if ($script:registrationMode -eq 'rebind-failure' -and $script:registrationCalls.Count -eq 2) {
            throw 'Failure after partial rebind.'
        }
    }
    $previousDirectory = $script:registeredDirectory
    $result = Set-TextViewerIdentity -PackagePath $PackagePath -Directory $Directory -Existing $script:registeredPackage
    Assert-MenuPreference ($result.PackageFullName -eq 'fixture' -and $script:registeredDirectory -eq $Directory) 'Rebind a trusted staged identity to the new application.'
    Assert-MenuPreference ($script:registrationCalls.Count -eq 2 -and $script:registrationRemovals -eq 1) 'Reinstall the same trusted identity only for the known installed-package error.'

    $script:registrationCalls = @()
    $script:registrationRemovals = 0
    $script:registeredDirectory = $previousDirectory
    $script:registrationMode = 'different-location'
    $result = Set-TextViewerIdentity -PackagePath $PackagePath -Directory $Directory -Existing $script:registeredPackage
    Assert-MenuPreference ($script:registrationCalls.Count -eq 2 -and $script:registeredDirectory -eq $Directory) 'Handle the external-location mismatch reported by Windows 11.'

    $script:registrationCalls = @()
    $script:registrationRemovals = 0
    $script:registeredDirectory = $previousDirectory
    $script:registrationMode = 'unrelated-error'
    $rejected = $false
    try { Set-TextViewerIdentity -PackagePath $PackagePath -Directory $Directory -Existing $script:registeredPackage }
    catch { $rejected = $true }
    Assert-MenuPreference ($rejected -and $script:registrationCalls.Count -eq 1 -and $script:registrationRemovals -eq 0 -and $script:registeredDirectory -eq $previousDirectory) 'Do not change the working registration after an unrelated deployment failure.'

    $script:registrationCalls = @()
    $script:registrationMode = 'already-installed'
    $script:registrationSignature = 'HashMismatch'
    $rejected = $false
    try { Set-TextViewerIdentity -PackagePath $PackagePath -Directory $Directory -Existing $script:registeredPackage }
    catch { $rejected = $true }
    Assert-MenuPreference ($rejected -and $script:registrationRemovals -eq 0 -and $script:registeredDirectory -eq $previousDirectory) 'Keep the working registration when its rollback archive has been tampered with.'

    $script:registrationCalls = @()
    $script:registrationRemovals = 0
    $script:registrationMode = 'rebind-failure'
    $script:registrationSignature = 'Valid'
    $rejected = $false
    try { Set-TextViewerIdentity -PackagePath $PackagePath -Directory $Directory -Existing $script:registeredPackage }
    catch { $rejected = $true }
    Assert-MenuPreference ($rejected -and $script:registrationCalls.Count -eq 3 -and $script:registeredDirectory -eq $previousDirectory) 'Restore the original external directory after a failed rebind.'

    $script:registrationCalls = @()
    $script:registrationRemovals = 0
    $script:registrationMode = 'success'
    $result = Set-TextViewerIdentity -PackagePath $PackagePath -Directory $Directory -Existing $script:registeredPackage
    Assert-MenuPreference ($script:registrationCalls.Count -eq 1 -and $script:registrationRemovals -eq 0 -and $script:registeredDirectory -eq $Directory) 'Register or upgrade directly when Windows accepts the signed package.'
}

Add-Type -AssemblyName System.IO.Compression.FileSystem
Add-Type -AssemblyName System.IO.Compression
$cacheRoot = [IO.Path]::GetFullPath((Join-Path (Split-Path -Parent $PSScriptRoot) '.cache'))
$work = Join-Path $cacheRoot "context-menu-regression-$([Guid]::NewGuid().ToString('N'))"
New-Item -ItemType Directory -Path $work -Force | Out-Null
try {
    $old = Join-Path $work 'old.msix'
    $new = Join-Path $work 'new.msix'
    New-IdentityFixture -Path $old -Version '0.0.1.0'
    New-IdentityFixture -Path $new -Version '0.0.4.0'
    $modern = Join-Path $work 'modern.msix'
    New-IdentityFixture -Path $modern -Version '0.0.8.0' -Modern
    Assert-MenuPreference (-not (Test-TextViewerReusableIdentity -PackagePath $old)) 'Do not reuse the old runtime model or an identity without an icon index.'
    Assert-MenuPreference (Test-TextViewerReusableIdentity -PackagePath $modern) 'Reuse a compatible trusted Win32 identity with indexed icons.'
    $existing = [pscustomobject]@{Name='Oranjekop.TextViewer.ContextMenu';Publisher='CN=Fixture';Version=[version]'0.0.1.0';Architecture='X64'}
    $previous = @{Directory=$work;PackagePath=$old}
    Assert-MenuPreference ((Get-TextViewerExistingIdentity $existing $previous) -eq $old) 'Reuse only the identity actually registered with Windows.'
    $previous.PackagePath = $new
    Assert-MenuPreference (-not (Get-TextViewerExistingIdentity $existing $previous)) 'Never reuse an archive with a different registered version.'
    $previous.PackagePath = $old
    $existing.Publisher = 'CN=Another signer'
    Assert-MenuPreference (-not (Get-TextViewerExistingIdentity $existing $previous)) 'Never reuse another publisher identity.'
    $existing.Publisher = 'CN=Fixture'
    $existing.Architecture = 'ARM64'
    Assert-MenuPreference (-not (Get-TextViewerExistingIdentity $existing $previous)) 'Never reuse another architecture identity.'
    Test-IdentityRegistration -PackagePath $old -Directory $work

    $visualRoot = Join-Path $work 'visuals'
    Expand-TextViewerVisualAssets -PackagePath $new -Directory $visualRoot
    Expand-TextViewerVisualAssets -PackagePath $new -Directory $visualRoot
    foreach ($name in @('Logo.png', 'Logo44.png', 'FileTypeText.png', 'Logo44.targetsize-48_altform-unplated.png', 'Logo44.targetsize-48_altform-lightunplated.png')) {
        Assert-MenuPreference (([IO.File]::ReadAllText((Join-Path $visualRoot "Assets\$name"))) -eq "fixture $name") 'Publish sparse visual resources to the external location.'
    }
    [IO.File]::WriteAllText((Join-Path $visualRoot 'Assets\Logo44.png'), 'tampered')
    $rejected = $false
    try { Expand-TextViewerVisualAssets -PackagePath $new -Directory $visualRoot } catch { $rejected = $true }
    Assert-MenuPreference $rejected 'Do not overwrite modified immutable visual resources.'

    $installRoot = Join-Path $work 'Programs\TextViewer'
    $directories = @{}
    foreach ($id in 1..7) {
        $directory = Join-Path $installRoot ('app-{0:x16}' -f $id)
        New-Item -ItemType Directory -Path $directory -Force | Out-Null
        foreach ($name in @('TextViewer.exe', 'TextViewerShell.dll', 'TextViewer.ContextMenu.msix', 'TextViewer.ContextMenu.cer', 'ContextMenu.ps1')) {
            [IO.File]::WriteAllText((Join-Path $directory $name), 'fixture')
        }
        $directories[$id] = $directory
    }
    [IO.File]::WriteAllText((Join-Path $directories[4] 'user-created.txt'), 'preserve')
    $outside = Join-Path $work 'outside'
    New-Item -ItemType Directory -Path $outside -Force | Out-Null
    [IO.File]::WriteAllText((Join-Path $outside 'user.txt'), 'preserve')
    $junction = Join-Path $directories[7] 'Assets'
    New-Item -ItemType Junction -Path $junction -Target $outside | Out-Null
    $lock = [IO.File]::Open((Join-Path $directories[5] 'TextViewer.exe'), [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::None)
    try {
        Remove-TextViewerOldBundles -InstallRoot $installRoot `
            -ProtectedPaths @($directories[2], (Join-Path $directories[6] 'TrustedIdentity-0123456789abcdef.msix')) `
            -RunningPaths @((Join-Path $directories[3] 'TextViewer.exe'))
        Assert-MenuPreference (-not (Test-Path -LiteralPath $directories[1])) 'Remove an unused old program bundle.'
        foreach ($id in 2..7) {
            Assert-MenuPreference (Test-Path -LiteralPath (Join-Path $directories[$id] 'TextViewer.exe')) 'Retain active, referenced, running, locked, linked and user-containing bundles.'
        }
        Assert-MenuPreference (([IO.File]::ReadAllText((Join-Path $outside 'user.txt'))) -eq 'preserve') 'Never follow a directory junction while cleaning old versions.'
    } finally { $lock.Dispose() }
    [IO.Directory]::Delete($junction, $false)
    Write-Output 'Context menu identity, external icons and safe old-version cleanup tests passed.'
} finally {
    $resolvedWork = [IO.Path]::GetFullPath($work)
    if (-not $resolvedWork.StartsWith("$cacheRoot\", [StringComparison]::OrdinalIgnoreCase) -or
        [IO.Path]::GetFileName($resolvedWork) -cnotmatch '^context-menu-regression-[a-f0-9]{32}$') {
        throw 'Invalid isolated test cleanup directory.'
    }
    if (Test-Path -LiteralPath $work) { Remove-Item -LiteralPath $work -Recurse -Force }
}
