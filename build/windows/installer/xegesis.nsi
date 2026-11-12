; Xegesis setup (NSIS 3, Modern UI 2). Per-user: no administrator prompt, installs under
; %LOCALAPPDATA%\Programs\Xegesis. Built by scripts\package-nsis.ps1, which passes:
;   /DVERSION=1.2.3          shown in the installer and in Add or Remove Programs
;   /DVERSION_NUM=1.2.3.0    four-part number for the file properties
;   /DEXE=<path>             the Xegesis.exe to install
;   /DOUTFILE=<path>         the setup .exe to write
;   /DWEBVIEW2=<path>        optional: Microsoft's WebView2 bootstrapper, embedded so a PC without
;                            the runtime gets it with no download step of our own
; The setup is not code-signed; the app's updates carry our own Ed25519 signature instead
; (docs/releasing.md). Silent use, as the updater does:  Setup.exe /S /UPDATE

Unicode true
SetCompressor /SOLID lzma
!include "MUI2.nsh"
!include "nsDialogs.nsh"
!include "LogicLib.nsh"
!include "FileFunc.nsh"

!ifndef VERSION
  !define VERSION "0.0.0"
!endif
!ifndef VERSION_NUM
  !define VERSION_NUM "0.0.0.0"
!endif
!ifndef EXE
  !error "Pass /DEXE=<path to Xegesis.exe>"
!endif
!ifndef OUTFILE
  !define OUTFILE "Xegesis-setup.exe"
!endif
!ifndef ROOT
  !define ROOT "..\..\.."
!endif

!define UNINSTKEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\Xegesis"
!define WEBVIEW2_CLIENT "Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}"

Name "Xegesis"
OutFile "${OUTFILE}"
InstallDir "$LOCALAPPDATA\Programs\Xegesis"
InstallDirRegKey HKCU "${UNINSTKEY}" "InstallLocation"
RequestExecutionLevel user
ShowInstDetails nevershow
ShowUninstDetails nevershow

VIProductVersion "${VERSION_NUM}"
VIAddVersionKey "ProductName" "Xegesis"
VIAddVersionKey "FileDescription" "Xegesis Bible study setup"
VIAddVersionKey "FileVersion" "${VERSION}"
VIAddVersionKey "ProductVersion" "${VERSION}"
VIAddVersionKey "LegalCopyright" "MIT OR Apache-2.0"

!define MUI_ICON "${ROOT}\build\windows\icon.ico"
!define MUI_UNICON "${ROOT}\build\windows\icon.ico"
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_RUN "$INSTDIR\Xegesis.exe"
!define MUI_FINISHPAGE_RUN_TEXT "Open Xegesis"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
UninstPage custom un.LibraryPage un.LibraryLeave
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"

Var IsUpdate
Var RemoveLibrary
Var RemoveCheck

Function .onInit
  StrCpy $IsUpdate 0
  ${GetParameters} $0
  ${GetOptions} $0 "/UPDATE" $1
  ${IfNot} ${Errors}
    StrCpy $IsUpdate 1
    ; The app that started this update is closing; give it a moment to let go of its files.
    Sleep 2500
  ${EndIf}
FunctionEnd

; WebView2 is built into current Windows 11 and Edge. Install it only when it is missing.
Function EnsureWebView2
  ReadRegStr $0 HKLM "SOFTWARE\WOW6432Node\${WEBVIEW2_CLIENT}" "pv"
  ${If} $0 == ""
    ReadRegStr $0 HKLM "SOFTWARE\${WEBVIEW2_CLIENT}" "pv"
  ${EndIf}
  ${If} $0 == ""
    ReadRegStr $0 HKCU "SOFTWARE\${WEBVIEW2_CLIENT}" "pv"
  ${EndIf}
  ${If} $0 == ""
  ${OrIf} $0 == "0.0.0.0"
!ifdef WEBVIEW2
    DetailPrint "Installing the Microsoft WebView2 runtime..."
    InitPluginsDir
    SetOutPath "$PLUGINSDIR"
    File "/oname=MicrosoftEdgeWebview2Setup.exe" "${WEBVIEW2}"
    ExecWait '"$PLUGINSDIR\MicrosoftEdgeWebview2Setup.exe" /silent /install'
!else
    MessageBox MB_OK|MB_ICONEXCLAMATION "Xegesis needs the Microsoft WebView2 runtime, which was not found. Install it from https://developer.microsoft.com/microsoft-edge/webview2/ and run this setup again."
!endif
  ${EndIf}
FunctionEnd

Section "Xegesis" SecMain
  Call EnsureWebView2
  SetOutPath "$INSTDIR"
  File "${EXE}"
  File "${ROOT}\LICENSE-MIT"
  File "${ROOT}\LICENSE-APACHE"
  File "${ROOT}\build\windows\icon.ico"
  WriteUninstaller "$INSTDIR\Uninstall.exe"

  CreateShortcut "$SMPROGRAMS\Xegesis.lnk" "$INSTDIR\Xegesis.exe" "" "$INSTDIR\icon.ico"

  WriteRegStr HKCU "${UNINSTKEY}" "DisplayName" "Xegesis"
  WriteRegStr HKCU "${UNINSTKEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINSTKEY}" "Publisher" "Xegesis contributors"
  WriteRegStr HKCU "${UNINSTKEY}" "DisplayIcon" "$INSTDIR\icon.ico"
  WriteRegStr HKCU "${UNINSTKEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTKEY}" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegStr HKCU "${UNINSTKEY}" "QuietUninstallString" '"$INSTDIR\Uninstall.exe" /S'
  WriteRegDWORD HKCU "${UNINSTKEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINSTKEY}" "NoRepair" 1
  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  IntFmt $0 "0x%08X" $0
  WriteRegDWORD HKCU "${UNINSTKEY}" "EstimatedSize" "$0"
SectionEnd

; After a silent update, start the app again.
Function .onInstSuccess
  ${If} $IsUpdate == 1
    Exec '"$INSTDIR\Xegesis.exe"'
  ${EndIf}
FunctionEnd

; Uninstall --------------------------------------------------------------------------------------

Function un.onInit
  StrCpy $RemoveLibrary 0
FunctionEnd

Function un.LibraryPage
  !insertmacro MUI_HEADER_TEXT "Your library" "Your notes, highlights, books and settings are kept by default."
  nsDialogs::Create 1018
  Pop $0
  ${NSD_CreateLabel} 0 0 100% 36u "Your library (notes, highlights, documents, Bible modules and settings) is stored in $APPDATA\Xegesis. It is not removed unless you tick the box below. This cannot be undone."
  Pop $0
  ${NSD_CreateCheckbox} 0 44u 100% 12u "Also remove my library"
  Pop $RemoveCheck
  nsDialogs::Show
FunctionEnd

Function un.LibraryLeave
  ${NSD_GetState} $RemoveCheck $RemoveLibrary
FunctionEnd

Section "Uninstall"
  Delete "$INSTDIR\Xegesis.exe"
  Delete "$INSTDIR\LICENSE-MIT"
  Delete "$INSTDIR\LICENSE-APACHE"
  Delete "$INSTDIR\icon.ico"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"
  Delete "$SMPROGRAMS\Xegesis.lnk"
  DeleteRegKey HKCU "${UNINSTKEY}"
  ${If} $RemoveLibrary == ${BST_CHECKED}
    RMDir /r "$APPDATA\Xegesis"
  ${EndIf}
SectionEnd
