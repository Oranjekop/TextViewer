; Identity registration never changes trust or the system's default editor.
; A local development signer is trusted only from the app's confirmation UI.
!macro customInstall
  nsExec::ExecToStack '"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$INSTDIR\ContextMenu.ps1" -Action Install -AppDirectory "$INSTDIR"'
  Pop $0
  Pop $1
  ${If} $0 != 0
    DetailPrint "Context menu not registered; use TextViewer Settings to complete registration."
  ${EndIf}
!macroend

!macro customUnInstall
  nsExec::ExecToStack '"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$INSTDIR\ContextMenu.ps1" -Action Remove -AppDirectory "$INSTDIR"'
  Pop $0
  Pop $1
  ${If} $0 != 0
    MessageBox MB_ICONSTOP "Unable to remove the TextViewer context menu. Close TextViewer and try uninstalling again." /SD IDOK
    Abort
  ${EndIf}
!macroend
