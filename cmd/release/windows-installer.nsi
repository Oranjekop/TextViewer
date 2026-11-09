Unicode true
ManifestDPIAware true
SetCompressor /SOLID lzma
Name @NAME@
OutFile @OUTPUT@
InstallDir "$LOCALAPPDATA\Programs\@DIRECTORY@"
InstallDirRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\@IDENTIFIER@" "InstallLocation"
RequestExecutionLevel user
BrandingText " "
VIProductVersion @VERSION4@
VIAddVersionKey /LANG=2052 "ProductName" @NAME@
VIAddVersionKey /LANG=2052 "ProductVersion" @VERSION@
VIAddVersionKey /LANG=2052 "FileDescription" "TextViewer 安装程序"
VIAddVersionKey /LANG=2052 "FileVersion" @VERSION@
VIAddVersionKey /LANG=2052 "LegalCopyright" "Copyright 2026 Oranjekop"
!define MUI_ICON @ICON@
!define MUI_UNICON @ICON@
!define MUI_FINISHPAGE_RUN "$INSTDIR\TextViewer.exe"
!define MUI_FINISHPAGE_SHOWREADME
!define MUI_FINISHPAGE_SHOWREADME_TEXT "创建桌面快捷方式"
!define MUI_FINISHPAGE_SHOWREADME_FUNCTION CreateDesktopShortcut
!include "MUI2.nsh"
!include "x64.nsh"
!include "LogicLib.nsh"
@TEST@
!include @HOOKS@
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "SimpChinese"

Function .onInit
  ${IfNot} ${RunningX64}
    MessageBox MB_ICONSTOP "TextViewer 需要 64 位 Windows。" /SD IDOK
    SetErrorLevel 2
    Abort
  ${EndIf}
FunctionEnd

; Only ask processes in the chosen install directory to exit naturally.
; Never force-stop an editor that may still contain unsaved changes.
!macro CheckRunning PREFIX
Function ${PREFIX}CheckRunning
  InitPluginsDir
  File /oname=$PLUGINSDIR\CheckRunning.ps1 @CHECK@
  System::Call 'kernel32::SetEnvironmentVariableW(w "TEXTVIEWER_INSTALL_DIR", w "$INSTDIR")'
  ; A 32-bit PowerShell cannot reliably read native editor module paths.
  StrCpy $2 "$WINDIR\Sysnative\WindowsPowerShell\v1.0\powershell.exe"
  IfFileExists "$2" +2
  StrCpy $2 "$SYSDIR\WindowsPowerShell\v1.0\powershell.exe"
  nsExec::ExecToStack /TIMEOUT=15000 '"$2" -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$PLUGINSDIR\CheckRunning.ps1"'
  Pop $0
  Pop $1
  ${If} $0 == 2
    MessageBox MB_ICONEXCLAMATION "请先关闭安装目录中的 TextViewer，再重新运行安装或卸载程序。未保存的文本不会被强制关闭。" /SD IDOK
    SetErrorLevel 2
    Abort
  ${ElseIf} $0 != 0
    MessageBox MB_ICONEXCLAMATION "无法确认 TextViewer 是否已退出。请关闭 TextViewer 后重试。" /SD IDOK
    SetErrorLevel 2
    Abort
  ${EndIf}
FunctionEnd
!macroend
!insertmacro CheckRunning ""
!insertmacro CheckRunning "un."

Section
  Call CheckRunning
  SetShellVarContext current
  SetOutPath "$INSTDIR"
  ${If} ${IsNativeARM64}
@ARM64@
  ${Else}
@AMD64@
  ${EndIf}
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  CreateShortCut "$SMPROGRAMS\@DIRECTORY@.lnk" "$INSTDIR\TextViewer.exe"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\@IDENTIFIER@" "DisplayName" @NAME@
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\@IDENTIFIER@" "DisplayVersion" @VERSION@
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\@IDENTIFIER@" "DisplayIcon" "$INSTDIR\TextViewer.exe"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\@IDENTIFIER@" "Publisher" "Oranjekop"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\@IDENTIFIER@" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\@IDENTIFIER@" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\@IDENTIFIER@" "QuietUninstallString" '"$INSTDIR\Uninstall.exe" /S'
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\@IDENTIFIER@" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\@IDENTIFIER@" "NoRepair" 1
  !ifndef TEXTVIEWER_ISOLATED_TEST
    !insertmacro customInstall
  !endif
SectionEnd

Function CreateDesktopShortcut
  SetShellVarContext current
  CreateShortCut "$DESKTOP\@DIRECTORY@.lnk" "$INSTDIR\TextViewer.exe"
FunctionEnd

Function un.DeleteShortcut
  Exch $0
  Push $1
  StrCpy $1 50
  retry:
    ClearErrors
    Delete $0
    IfErrors 0 done
    IntOp $1 $1 - 1
    IntCmp $1 0 done
    Sleep 100
    Goto retry
  done:
  Pop $1
  Pop $0
FunctionEnd

Section "Uninstall"
  Call un.CheckRunning
  SetShellVarContext current
  !ifndef TEXTVIEWER_ISOLATED_TEST
    !insertmacro customUnInstall
  !endif
  Push "$SMPROGRAMS\@DIRECTORY@.lnk"
  Call un.DeleteShortcut
  Push "$DESKTOP\@DIRECTORY@.lnk"
  Call un.DeleteShortcut
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\@IDENTIFIER@"
  Delete "$INSTDIR\TextViewer.exe"
  Delete "$INSTDIR\TextViewerShell.dll"
  Delete "$INSTDIR\TextViewer.ContextMenu.msix"
  Delete "$INSTDIR\TextViewer.ContextMenu.cer"
  Delete "$INSTDIR\ContextMenu.ps1"
  Delete "$INSTDIR\Uninstall.exe"
  ; Nonrecursive: never delete user-created files in the install directory.
  RMDir "$INSTDIR"
SectionEnd
