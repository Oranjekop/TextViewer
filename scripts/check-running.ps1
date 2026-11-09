$ErrorActionPreference = 'Stop'
$directory = [IO.Path]::GetFullPath($env:TEXTVIEWER_INSTALL_DIR).TrimEnd('\')
$timer = [Diagnostics.Stopwatch]::StartNew()
do {
    $running = $false
    foreach ($process in @(Get-Process -Name TextViewer -ErrorAction SilentlyContinue)) {
        try {
            $path = $process.Path
            if (-not $path) {
                if (-not (Get-Process -Id $process.Id -ErrorAction SilentlyContinue)) { continue }
                exit 3 # Cannot safely rule out an editor in the target directory.
            }
            if ([string]::Equals([IO.Path]::GetDirectoryName($path), $directory, [StringComparison]::OrdinalIgnoreCase)) {
                $running = $true
                break
            }
        } finally { $process.Dispose() }
    }
    if (-not $running) { exit 0 }
    Start-Sleep -Milliseconds 250
} while ($timer.Elapsed.TotalSeconds -lt 8)
exit 2
