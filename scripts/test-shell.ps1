param(
    [string]$BuildDirectory = 'dist\TextViewer\windows-amd64',
    [switch]$Registered
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'tools.ps1')
$tools = Get-TextViewerTools
$root = Split-Path -Parent $PSScriptRoot
$testDirectory = Join-Path $root '.cache\shell-tests'
New-Item -ItemType Directory -Path $testDirectory -Force | Out-Null
$testExe = Join-Path $testDirectory 'shell-tests.exe'
& $tools.Clang -std=c++17 -O2 -static -municode -DUNICODE -D_UNICODE `
    (Join-Path $root 'shell\TextViewerShell_test.cpp') -o $testExe -lole32 -lshell32 -luuid
Assert-TextViewerExit 'Shell test compilation'
$file = Join-Path $testDirectory 'sample & spaces.txt'
[IO.File]::WriteAllText($file, 'TextViewer shell extension test')
$dll = '--registered'
if (-not $Registered) {
    $payload = Join-Path $testDirectory ("embedded-" + [guid]::NewGuid().ToString('N'))
    go run ./cmd/release extract -executable (Join-Path $root "$BuildDirectory\TextViewer.exe") -output $payload
    Assert-TextViewerExit 'Extract embedded integration for isolated tests'
    Copy-Item -LiteralPath (Join-Path $root "$BuildDirectory\TextViewer.exe") -Destination (Join-Path $payload 'TextViewer.exe')
    $dll = Join-Path $payload 'TextViewerShell.dll'
}
& $testExe $dll $file $testDirectory
Assert-TextViewerExit 'Explorer command tests'
if (-not $Registered) {
    $invokeDirectory = Join-Path $testDirectory ("invoke-" + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $invokeDirectory | Out-Null
    $isolatedDll = Join-Path $invokeDirectory 'TextViewerShell.dll'
    Copy-Item -LiteralPath $dll -Destination $isolatedDll
    & $tools.Clang -std=c++17 -O2 -static -municode -mwindows -DUNICODE -D_UNICODE `
        (Join-Path $root 'shell\TextViewerShell_recorder_test.cpp') `
        -o (Join-Path $invokeDirectory 'TextViewer.exe') -lshell32
    Assert-TextViewerExit 'Isolated argument recorder compilation'
    $unicodeFile = Join-Path $invokeDirectory ('sample & ' + [char]0x4e2d + [char]0x6587 + '.txt')
    [IO.File]::WriteAllText($unicodeFile, 'TextViewer multi-file invocation test')
    $previousCapture = $env:TEXTVIEWER_SHELL_TEST_CAPTURE
    try {
        $env:TEXTVIEWER_SHELL_TEST_CAPTURE = $invokeDirectory
        & $testExe $isolatedDll $unicodeFile $invokeDirectory --invoke $invokeDirectory
        Assert-TextViewerExit 'Multi-file invocation tests'
    } finally {
        if ($null -eq $previousCapture) { Remove-Item Env:TEXTVIEWER_SHELL_TEST_CAPTURE }
        else { $env:TEXTVIEWER_SHELL_TEST_CAPTURE = $previousCapture }
    }
    $windowTest = Join-Path $testDirectory 'window-tests.exe'
    & $tools.Clang -std=c++17 -O2 -static -municode -DUNICODE -D_UNICODE `
        (Join-Path $root 'shell\TextViewerWindow_test.cpp') -o $windowTest -lgdi32
    Assert-TextViewerExit 'Native window icon test compilation'
    $application = (Resolve-Path -LiteralPath (Join-Path $root "$BuildDirectory\TextViewer.exe")).Path
    & $windowTest $application (Join-Path $root 'assets\icon.png')
    Assert-TextViewerExit 'Native window icon tests'
}
