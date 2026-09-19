Unicode true

####
## Agentry Desktop's Windows installer (agentry-desktop#83; plan
## agentry-notes/plans/desktop-installers.md §4, §5, §8; D1, D8, D11).
##
## A file-copy program and nothing more. It installs per user, under the
## person's own programs directory, without elevation (D11: no UAC prompt;
## the app's self-update reinstalls the same way, silently, M7). It writes a
## Start Menu shortcut and an Apps & features entry, and offers to launch the
## app on its last page. It asks nothing about Docker, sign-in, models or
## agents: those are the app's first-run wizard's questions (D1).
##
## Installing over an existing installation replaces only the files under
## $INSTDIR. Configuration lives outside it (%USERPROFILE%\.config\agentry,
## %USERPROFILE%\.agentry\runner, Windows Credential Manager) and nothing in
## the install section reaches there. The uninstaller removes the application
## and leaves those alone unless the person ticks "Also remove my Agentry
## settings and sign-in" (off by default, off under /S), which hands the
## removal to the app itself (`agentry-desktop.exe --remove-my-settings`,
## internal/uninstall): the app knows the paths, the relocation variables and
## the keychain entries it wrote; this script does not.
##
## `wails build -platform windows/amd64 -nsis` resolves wails_tools.nsh from
## its own template (DO NOT EDIT that file: the checked-in copy is what the
## build writes), drops the WebView2 bootstrapper into tmp/ and runs
## makensis on this file with -DARG_WAILS_AMD64_BINARY. The two defines
## below are what `-installscope user` would pass; they live here so the
## script is the record and a build without the flag builds the same
## installer (passing the flag as well would redefine them and fail).
## scripts/check-windows-installer.sh reads the built installer back.
####
!define REQUEST_EXECUTION_LEVEL "user"
!define WAILS_INSTALL_SCOPE "user"

!include "wails_tools.nsh"
!include "LogicLib.nsh"

# The version information for this two must consist of 4 parts
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

# Enable HiDPI support. https://nsis.sourceforge.io/Reference/ManifestDPIAware
ManifestDPIAware true

!include "MUI.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_FINISHPAGE_NOAUTOCLOSE # Wait on the INSTFILES page so the user can take a look into the details of the installation steps
!define MUI_ABORTWARNING # This will warn the user if they exit from the installer.
# The last page offers to start the app; the first-run wizard takes it from
# there (D1). Ticked by default, as MUI does.
!define MUI_FINISHPAGE_RUN "$INSTDIR\${PRODUCT_EXECUTABLE}"
!define MUI_FINISHPAGE_RUN_TEXT "Launch ${INFO_PRODUCTNAME}"

!insertmacro MUI_PAGE_WELCOME # Welcome to the installer page.
# No EULA page: the licence is the repository's; the installer asks only installer-shaped questions (plan §4).
!insertmacro MUI_PAGE_DIRECTORY # In which folder install page.
!insertmacro MUI_PAGE_INSTFILES # Installing page.
!insertmacro MUI_PAGE_FINISH # Finished installation page.

# The uninstaller's one optional page (plan §8): the application is always
# removed; the settings and sign-in only when the box is ticked. The
# description box names the paths before anything is hovered, spelled with
# %USERPROFILE% so the text does not depend on variable expansion inside a
# language string.
!define MUI_COMPONENTSPAGE_TEXT_TOP "${INFO_PRODUCTNAME} will be removed. Your settings and sign-in stay unless you tick the second box."
!define MUI_COMPONENTSPAGE_TEXT_COMPLIST "Remove:"
!define MUI_COMPONENTSPAGE_TEXT_DESCRIPTION_TITLE "What stays"
!define MUI_COMPONENTSPAGE_TEXT_DESCRIPTION_INFO "By default your settings (%USERPROFILE%\.config\agentry), runner configuration (%USERPROFILE%\.agentry\runner) and sign-in (the agentry entries in Windows Credential Manager) are kept, so a reinstall finds them. The agentry CLI uses the same files."
!insertmacro MUI_UNPAGE_COMPONENTS
!insertmacro MUI_UNPAGE_INSTFILES # Uinstalling page

!insertmacro MUI_LANGUAGE "English" # Set the Language of the installer

## The following two statements can be used to sign the installer and the uninstaller. The path to the binaries are provided in %1
## Unsigned today (agentry-desktop#54).
#!uninstfinalize 'signtool --file "%1"'
#!finalize 'signtool --file "%1"'

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe" # Name of the installer's file; the Makefile renames it to the manifest's grammar.
# Per user (WAILS_INSTALL_SCOPE above): the person's own programs directory,
# writable without elevation. An upgrade defaults to where the previous
# release went (InstallLocation is written below), so installing N over N-1
# replaces the files in place rather than leaving two copies.
InstallDir "$LOCALAPPDATA\Programs\${INFO_PRODUCTNAME}"
InstallDirRegKey HKCU "${UNINST_KEY}" "InstallLocation"
ShowInstDetails show # This will always show the installation details.

Function .onInit
   !insertmacro wails.checkArchitecture
FunctionEnd

# Everything this section touches: $INSTDIR, the per-user Start Menu, the
# per-user uninstall key (HKCU), and the WebView2 runtime if it is missing.
# Nothing under the person's profile beyond those.
Section
    !insertmacro wails.setShellContext

    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR

    !insertmacro wails.files

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols

    !insertmacro wails.writeUninstaller
    WriteRegStr HKCU "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
    WriteRegStr HKCU "${UNINST_KEY}" "URLInfoAbout" "https://secureagentry.net"
SectionEnd

# The uninstaller. Sections run in the order they are declared. The first
# visible section is the page's mandatory row and does nothing itself; the
# optional second runs the app's own removal while the binary is still
# there; the hidden third, always run, removes the application afterwards.
Section "un.${INFO_PRODUCTNAME}" SEC_UN_APP
    SectionIn RO
SectionEnd

# Off by default (/o), and so off under /S: a silent uninstall never touches
# the person's configuration (plan §3's invariant).
Section /o "un.Also remove my Agentry settings and sign-in" SEC_UN_SETTINGS
    Push $0
    DetailPrint "Removing your Agentry settings and sign-in"
    ClearErrors
    ExecWait '"$INSTDIR\${PRODUCT_EXECUTABLE}" --remove-my-settings' $0
    ${If} ${Errors}
    ${OrIf} $0 <> 0
        DetailPrint "Some settings could not be removed (exit code $0)"
        ${IfNot} ${Silent}
            MessageBox MB_OK|MB_ICONEXCLAMATION "Some of your Agentry settings could not be removed. Look in %USERPROFILE%\.config\agentry, %USERPROFILE%\.agentry\runner and Windows Credential Manager for what is left."
        ${EndIf}
    ${EndIf}
    Pop $0
SectionEnd

Section "-un.RemoveApplication"
    !insertmacro wails.setShellContext

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" # Remove the WebView2 DataPath

    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
SectionEnd

!insertmacro MUI_UNFUNCTION_DESCRIPTION_BEGIN
    !insertmacro MUI_DESCRIPTION_TEXT ${SEC_UN_APP} "Removes the application from $INSTDIR, its Start Menu shortcut, its Apps & features entry and its WebView2 cache. Your settings and sign-in stay."
    !insertmacro MUI_DESCRIPTION_TEXT ${SEC_UN_SETTINGS} "Also deletes %USERPROFILE%\.config\agentry (settings, installed agents, pinned keys), %USERPROFILE%\.agentry\runner (runner policy and profiles) — or where AGENTRY_CONFIG_DIR and AGENTRY_RUNNER_HOME point — and every agentry entry in Windows Credential Manager (registry sign-ins and the model key). The agentry CLI shares all of these."
!insertmacro MUI_UNFUNCTION_DESCRIPTION_END
