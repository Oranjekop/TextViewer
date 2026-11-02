param(
    [string]$BuildDirectory = 'dist\TextViewer\windows-amd64'
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
$dll = [IO.Path]::GetFullPath((Join-Path $root "$BuildDirectory\TextViewerShell.dll"))
& $testExe $dll $file $testDirectory
Assert-TextViewerExit 'Explorer command tests'
