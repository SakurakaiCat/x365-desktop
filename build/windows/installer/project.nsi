Unicode true
####
## Please note: Template overrides are for Wails v2 only.
## As of v3, templates are built-in to the framework. See: https://v3alpha.wails.io/guides/custom-templates/
####

!macro NSIS_INSTALL_PROFILES
  ; Copy wintun.dll alongside the executable
  File "${BINARYDIR}\wintun.dll"
!macroend

!macro NSIS_UNINSTALL_PROFILES
  Delete "$INSTDIR\wintun.dll"
!macroend