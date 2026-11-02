$ErrorActionPreference = 'Stop'
$directory = [IO.Path]::GetFullPath($env:TEXTVIEWER_INSTALL_DIR).TrimEnd('\')
$deadline = (Get-Date).AddSeconds(8)
do {
    $running = @(Get-CimInstance Win32_Process -Filter "Name = 'TextViewer.exe'" | Where-Object {
        $_.ExecutablePath -and [string]::Equals(
            [IO.Path]::GetDirectoryName($_.ExecutablePath), $directory, [StringComparison]::OrdinalIgnoreCase)
    })
    if ($running.Count -eq 0) { exit 0 }
    Start-Sleep -Milliseconds 250
} while ((Get-Date) -lt $deadline)
exit 2
