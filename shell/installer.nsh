; Identity registration never changes trust or the system's default editor.
; A local development signer is trusted only from the app's confirmation UI.
!macro customInstall
  nsExec::ExecToStack '"$INSTDIR\TextViewer.exe" --register-file-associations'
  Pop $0
  Pop $1
  ${If} $0 != 0
    MessageBox MB_ICONEXCLAMATION "文件关联注册未完成。可在 TextViewer 的设置中重新注册常用文件格式。" /SD IDOK
  ${EndIf}
  nsExec::ExecToStack '"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$INSTDIR\ContextMenu.ps1" -Action Install -AppDirectory "$INSTDIR"'
  Pop $0
  Pop $1
  ${If} $0 != 0
    DetailPrint "Context menu not registered; use TextViewer Settings to complete registration."
  ${EndIf}
!macroend

!macro customUnInstall
  nsExec::ExecToStack '"$INSTDIR\TextViewer.exe" --unregister-file-associations'
  Pop $0
  Pop $1
  ${If} $0 != 0
    MessageBox MB_ICONSTOP "文件关联移除失败。请关闭 TextViewer 后重试卸载。" /SD IDOK
    Abort
  ${EndIf}
  nsExec::ExecToStack '"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$INSTDIR\ContextMenu.ps1" -Action Remove -AppDirectory "$INSTDIR"'
  Pop $0
  Pop $1
  ${If} $0 != 0
    MessageBox MB_ICONSTOP "Unable to remove the TextViewer context menu. Close TextViewer and try uninstalling again." /SD IDOK
    Abort
  ${EndIf}
!macroend
