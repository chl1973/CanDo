; CanDo 可为 Windows 安装程序（NSIS 3，Unicode）
Unicode true
ManifestDPIAware true
SetCompressor /SOLID lzma

!define APPNAME "CanDo 可为"
!define APPEXE "CanDo.exe"
!define OLDEXE "KeyanWorkbench.exe"
!define OLDNAME "科研竞赛工作台"
!define APPID "KeyanWorkbench"
!ifndef VERSION
  !define VERSION "1.15.0"
!endif
!define UNINSTKEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APPID}"

Name "${APPNAME}"
OutFile "..\dist\CanDo可为_安装包_v${VERSION}.exe"
InstallDir "$PROGRAMFILES64\${APPID}"
InstallDirRegKey HKLM "${UNINSTKEY}" "InstallLocation"
RequestExecutionLevel admin
BrandingText "${APPNAME} v${VERSION}"

VIProductVersion "1.15.0.0"
VIAddVersionKey /LANG=2052 "ProductName" "${APPNAME}"
VIAddVersionKey /LANG=2052 "FileDescription" "${APPNAME} 安装程序"
VIAddVersionKey /LANG=2052 "FileVersion" "${VERSION}"
VIAddVersionKey /LANG=2052 "ProductVersion" "${VERSION}"
VIAddVersionKey /LANG=2052 "LegalCopyright" "${APPNAME}"

!include "MUI2.nsh"
!include "x64.nsh"
!include "LogicLib.nsh"

!define MUI_ICON "app.ico"
!define MUI_UNICON "app.ico"
!define MUI_ABORTWARNING
!define MUI_WELCOMEPAGE_TITLE "欢迎安装 ${APPNAME}"
!define MUI_WELCOMEPAGE_TEXT "CanDo 可为帮助课题组把科研和竞赛做成规范的流程：阶段任务与审核、项目资料库、有依据的问答、论文引用核验和往届经验库。$\r$\n$\r$\n所有数据只保存在这台电脑上。安装后，同一 Wi-Fi 下的手机可以扫码使用。$\r$\n$\r$\n点击“下一步”继续。"
!define MUI_FINISHPAGE_RUN
!define MUI_FINISHPAGE_RUN_TEXT "立即打开 CanDo 可为"
!define MUI_FINISHPAGE_RUN_FUNCTION LaunchAsUser
!define MUI_FINISHPAGE_TEXT "安装完成。桌面上已创建“${APPNAME}”图标。$\r$\n$\r$\n第一次打开时请创建管理员账号（通常是指导老师）。"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "SimpChinese"

Function .onInit
  ${IfNot} ${RunningX64}
    MessageBox MB_ICONSTOP "${APPNAME} 需要 64 位 Windows 10 或更高版本。"
    Abort
  ${EndIf}
  SetRegView 64
FunctionEnd

; 以当前登录用户（非管理员）身份启动程序，避免程序以管理员权限运行
Function LaunchAsUser
  Exec '"$WINDIR\explorer.exe" "$INSTDIR\${APPEXE}"'
FunctionEnd

Section "主程序" SecMain
  SectionIn RO
  ; 升级安装时先结束正在运行的旧版本
  nsExec::Exec 'taskkill /F /IM "${APPEXE}"'
  Pop $0
  nsExec::Exec 'taskkill /F /IM "${OLDEXE}"'
  Pop $0
  Sleep 500
  ; 1.12 起改名为 CanDo 可为：清理旧版的程序文件和快捷方式（数据目录不变，升级后数据照常使用）
  Delete "$INSTDIR\${OLDEXE}"
  SetShellVarContext all
  Delete "$DESKTOP\${OLDNAME}.lnk"
  RMDir /r "$SMPROGRAMS\${OLDNAME}"
  SetShellVarContext current
  Delete "$DESKTOP\${OLDNAME}.lnk"
  SetOutPath "$INSTDIR"
  File "..\dist\${APPEXE}"
  File "app.ico"
  File "..\dist\使用说明.txt"
  WriteUninstaller "$INSTDIR\uninstall.exe"

  ; 快捷方式（所有用户）
  SetShellVarContext all
  CreateShortcut "$DESKTOP\${APPNAME}.lnk" "$INSTDIR\${APPEXE}" "" "$INSTDIR\app.ico" 0
  CreateDirectory "$SMPROGRAMS\${APPNAME}"
  CreateShortcut "$SMPROGRAMS\${APPNAME}\${APPNAME}.lnk" "$INSTDIR\${APPEXE}" "" "$INSTDIR\app.ico" 0
  CreateShortcut "$SMPROGRAMS\${APPNAME}\使用说明.lnk" "$INSTDIR\使用说明.txt"
  CreateShortcut "$SMPROGRAMS\${APPNAME}\忘记管理员密码.lnk" "$INSTDIR\${APPEXE}" "-reset-admin" "$INSTDIR\app.ico" 0
  CreateShortcut "$SMPROGRAMS\${APPNAME}\卸载 ${APPNAME}.lnk" "$INSTDIR\uninstall.exe"

  ; 防火墙：允许同一局域网的手机访问（程序内默认关闭，需老师在“设置”中开启）
  nsExec::Exec 'netsh advfirewall firewall delete rule name="${APPID}"'
  Pop $0
  nsExec::Exec 'netsh advfirewall firewall add rule name="${APPID}" dir=in action=allow program="$INSTDIR\${APPEXE}" enable=yes profile=private,domain,public protocol=TCP'
  Pop $0

  ; 控制面板“程序和功能”
  WriteRegStr HKLM "${UNINSTKEY}" "DisplayName" "${APPNAME}"
  WriteRegStr HKLM "${UNINSTKEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKLM "${UNINSTKEY}" "Publisher" "${APPNAME}"
  WriteRegStr HKLM "${UNINSTKEY}" "DisplayIcon" "$INSTDIR\app.ico"
  WriteRegStr HKLM "${UNINSTKEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKLM "${UNINSTKEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegStr HKLM "${UNINSTKEY}" "QuietUninstallString" '"$INSTDIR\uninstall.exe" /S'
  WriteRegDWORD HKLM "${UNINSTKEY}" "NoModify" 1
  WriteRegDWORD HKLM "${UNINSTKEY}" "NoRepair" 1
  WriteRegDWORD HKLM "${UNINSTKEY}" "EstimatedSize" 12000
SectionEnd

Function un.onInit
  SetRegView 64
FunctionEnd

Section "Uninstall"
  nsExec::Exec 'taskkill /F /IM "${APPEXE}"'
  Pop $0
  Sleep 500
  nsExec::Exec 'netsh advfirewall firewall delete rule name="${APPID}"'
  Pop $0
  Delete "$INSTDIR\${APPEXE}"
  Delete "$INSTDIR\app.ico"
  Delete "$INSTDIR\使用说明.txt"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
  SetShellVarContext all
  Delete "$DESKTOP\${APPNAME}.lnk"
  RMDir /r "$SMPROGRAMS\${APPNAME}"
  DeleteRegKey HKLM "${UNINSTKEY}"
  ; 数据默认保留，询问是否一并删除（静默卸载时保留）
  SetShellVarContext current
  IfFileExists "$LOCALAPPDATA\${APPID}\data\data.json" 0 done
  MessageBox MB_YESNO|MB_ICONQUESTION|MB_DEFBUTTON2 "是否同时删除工作台的所有数据（账号、项目、上传的资料）？$\r$\n$\r$\n选择“否”将保留数据，重新安装后可继续使用。$\r$\n数据位置：$LOCALAPPDATA\${APPID}" /SD IDNO IDNO done
  RMDir /r "$LOCALAPPDATA\${APPID}"
  done:
SectionEnd
