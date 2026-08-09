; AIProxy Windows 安装程序脚本
; 编译: makensis -DVERSION=x.y.z [-DFILEVERSION=x.y.z.w] installer.nsi
; 依赖: build/aiproxy-windows-amd64.exe 已构建
;
; 功能：
;   - 向导式安装，可选择安装路径
;   - 附加任务：开机自启动（HKCU Run 注册表）、桌面快捷方式（默认勾选）
;   - 创建开始菜单快捷方式与卸载器
;   - 卸载时询问是否保留用户数据（aiproxy.db）

Unicode true

; ---------- 基础信息 ----------
!define APP_NAME "AIProxy"
!define APP_EXE "aiproxy.exe"
!define APP_PUBLISHER "AIProxy"
!define APP_URL "https://github.com/aiproxy"
!define APP_ICON "..\..\assets\aiproxy.ico"
!define UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\AIProxy"
!define RUN_KEY "Software\Microsoft\Windows\CurrentVersion\Run"

; 版本号（由 makensis -DVERSION=x.y.z 传入；未传入时默认 1.0.0）
!ifndef VERSION
  !define VERSION "1.0.0"
!endif

; 规范化的文件版本（x.y.z.w，VIProductVersion 要求纯数字四段式）
; 由构建脚本传入；未传入时回退 1.0.0.0
!ifndef FILEVERSION
  !define FILEVERSION "1.0.0.0"
!endif

; 输出安装包文件名
OutFile "aiproxy-Setup-${VERSION}.exe"

Name "${APP_NAME} ${VERSION}"
; 默认安装到用户级目录（无需管理员权限，程序可正常写自身目录 aiproxy.db）
InstallDir "$LOCALAPPDATA\Programs\${APP_NAME}"
InstallDirRegKey HKCU "${UNINST_KEY}" "InstallLocation"
; 用户级权限，避免 UAC 弹窗
RequestExecutionLevel user

; ---------- MUI2 现代向导 ----------
!include "MUI2.nsh"
!include "LogicLib.nsh"

!define MUI_ABORTWARNING
!define MUI_ICON "${APP_ICON}"
!define MUI_UNICON "${APP_ICON}"
!define MUI_FINISHPAGE_RUN "$INSTDIR\${APP_EXE}"
!define MUI_FINISHPAGE_RUN_TEXT "立即运行 ${APP_NAME}"
!define MUI_FINISHPAGE_NOAUTOCLOSE

; ---------- 页面 ----------
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
; 组件选择页：自动列出「开机自启动」「创建桌面快捷方式」供用户勾选（默认选中）
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "SimpChinese"
!insertmacro MUI_LANGUAGE "English"

; ---------- 区段 ----------
Section "主程序（必需）" SecMain
  SectionIn RO
  SetOutPath "$INSTDIR"

  ; 安装主程序（编译时目标文件名不同，用 /oname 重命名）
  File "/oname=${APP_EXE}" "..\..\build\aiproxy-windows-amd64.exe"

  ; 写入卸载注册表项
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayName" "${APP_NAME}"
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayVersion" "${FILEVERSION}"
  WriteRegStr HKCU "${UNINST_KEY}" "Publisher" "${APP_PUBLISHER}"
  WriteRegStr HKCU "${UNINST_KEY}" "URLInfoAbout" "${APP_URL}"
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayIcon" "$INSTDIR\${APP_EXE}"
  WriteRegStr HKCU "${UNINST_KEY}" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegStr HKCU "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegDWORD HKCU "${UNINST_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINST_KEY}" "NoRepair" 1
  WriteRegDWORD HKCU "${UNINST_KEY}" "EstimatedSize" 12288

  ; 开始菜单快捷方式
  CreateDirectory "$SMPROGRAMS\${APP_NAME}"
  CreateShortcut "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk" "$INSTDIR\${APP_EXE}"
  CreateShortcut "$SMPROGRAMS\${APP_NAME}\卸载 ${APP_NAME}.lnk" "$INSTDIR\Uninstall.exe"
SectionEnd

; 附加任务：开机自启动（写 HKCU Run 注册表，用户级无需管理员权限）
Section "开机自启动" SecAutoStart
  WriteRegStr HKCU "${RUN_KEY}" "${APP_NAME}" '"$INSTDIR\${APP_EXE}"'
SectionEnd

; 附加任务：桌面快捷方式
Section "创建桌面快捷方式" SecDesktop
  CreateShortcut "$DESKTOP\${APP_NAME}.lnk" "$INSTDIR\${APP_EXE}"
SectionEnd

; ---------- 区段描述（组件页显示） ----------
!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${SecMain} "AIProxy 主程序（必需）。"
  !insertmacro MUI_DESCRIPTION_TEXT ${SecAutoStart} "开机时自动启动 AIProxy，代理服务随系统登录自动运行。"
  !insertmacro MUI_DESCRIPTION_TEXT ${SecDesktop} "在桌面创建 AIProxy 快捷方式。"
!insertmacro MUI_FUNCTION_DESCRIPTION_END

; ---------- 卸载区段 ----------
Section "Uninstall"
  ; 清理注册表（自启动键 + 卸载信息）
  DeleteRegValue HKCU "${RUN_KEY}" "${APP_NAME}"
  DeleteRegKey HKCU "${UNINST_KEY}"

  ; 删除快捷方式
  Delete "$DESKTOP\${APP_NAME}.lnk"
  Delete "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk"
  Delete "$SMPROGRAMS\${APP_NAME}\卸载 ${APP_NAME}.lnk"
  RMDir "$SMPROGRAMS\${APP_NAME}"

  ; 询问是否保留用户数据（aiproxy.db：渠道配置、用量统计等）
  MessageBox MB_YESNO|MB_ICONQUESTION "是否保留用户数据（渠道配置、用量统计等保存在 aiproxy.db 中）？$\n$\n选择「是」将保留数据，选择「否」将删除全部数据。" IDYES keepData
  Delete "$INSTDIR\aiproxy.db"
  keepData:
  Delete "$INSTDIR\aiproxy.db-journal"
  Delete "$INSTDIR\aiproxy.db-wal"
  Delete "$INSTDIR\aiproxy.db-shm"

  ; 删除程序文件与安装目录
  Delete "$INSTDIR\${APP_EXE}"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"
SectionEnd

; ---------- 安装包版本信息 ----------
VIProductVersion "${FILEVERSION}"
VIAddVersionKey "ProductName" "${APP_NAME}"
VIAddVersionKey "CompanyName" "${APP_PUBLISHER}"
VIAddVersionKey "FileDescription" "${APP_NAME} Windows 安装程序"
VIAddVersionKey "FileVersion" "${FILEVERSION}"
VIAddVersionKey "ProductVersion" "${FILEVERSION}"
VIAddVersionKey "LegalCopyright" "Copyright (c) ${APP_PUBLISHER}"