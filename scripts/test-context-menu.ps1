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
    # Older Windows versions have no customization preferences to update.
    Enable-TextViewerModernMenu -PackageFamilyName $family -PreferencesPath $preferences
    Assert-MenuPreference (-not (Test-Path -LiteralPath $testRoot)) 'Do not create preferences on older Windows.'

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
