; Identity registration never changes trust or the system's default editor.
; A local development signer is trusted only from the app's confirmation UI.
!macro customInstall
  nsExec::ExecToStack '"$INSTDIR\TextViewer.exe" --configure-windows-integration'
  Pop $0
  Pop $1
  ${If} $0 != 0
    MessageBox MB_ICONEXCLAMATION "文件关联或右键菜单设置未完成。请打开 TextViewer 的设置，确认勾选后重新保存。" /SD IDOK
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
