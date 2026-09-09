//go:build windows

package sysproxy

import (
	"fmt"
	"syscall"
	"unsafe"
)

type windowsManager struct {
	enabled bool
}

var (
	modWininet                = syscall.NewLazyDLL("wininet.dll")
	procInternetSetOption     = modWininet.NewProc("InternetSetOptionW")
	modAdvapi32               = syscall.NewLazyDLL("advapi32.dll")
	procRegOpenKey            = modAdvapi32.NewProc("RegOpenKeyExW")
	procRegSetValue           = modAdvapi32.NewProc("RegSetValueExW")
	procRegCloseKey           = modAdvapi32.NewProc("RegCloseKey")
)

const (
	HKEY_CURRENT_USER = 0x80000001
	KEY_SET_VALUE      = 0x0002
	REG_DWORD         = 4
	REG_SZ            = 1
	INTERNET_OPTION_SETTINGS_CHANGED = 39
	INTERNET_OPTION_REFRESH          = 37
)

func newManager() Manager {
	return &windowsManager{}
}

func (w *windowsManager) SetSOCKS5(host string, port int) error {
	keyPath, _ := syscall.UTF16PtrFromString(`Software\Microsoft\Windows\CurrentVersion\Internet Settings`)
	var handle syscall.Handle
	ret, _, _ := procRegOpenKey.Call(
		uintptr(HKEY_CURRENT_USER),
		uintptr(unsafe.Pointer(keyPath)),
		0,
		uintptr(KEY_SET_VALUE),
		uintptr(unsafe.Pointer(&handle)),
	)
	if ret != 0 {
		return fmt.Errorf("RegOpenKeyEx failed: %d", ret)
	}
	defer procRegCloseKey.Call(uintptr(handle))

	proxyEnable, _ := syscall.UTF16PtrFromString("ProxyEnable")
	enableVal := uint32(1)
	procRegSetValue.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(proxyEnable)),
		0,
		uintptr(REG_DWORD),
		uintptr(unsafe.Pointer(&enableVal)),
		unsafe.Sizeof(enableVal),
	)

	proxyServer, _ := syscall.UTF16PtrFromString("ProxyServer")
	serverStr, _ := syscall.UTF16PtrFromString(fmt.Sprintf("socks=%s:%d", host, port))
	procRegSetValue.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(proxyServer)),
		0,
		uintptr(REG_SZ),
		uintptr(unsafe.Pointer(serverStr)),
		unsafe.Sizeof(uint16(0))*uintptr(len(fmt.Sprintf("socks=%s:%d", host, port))+1),
	)

	procInternetSetOption.Call(0, INTERNET_OPTION_SETTINGS_CHANGED, 0, 0)
	procInternetSetOption.Call(0, INTERNET_OPTION_REFRESH, 0, 0)

	w.enabled = true
	return nil
}

func (w *windowsManager) Clear() error {
	keyPath, _ := syscall.UTF16PtrFromString(`Software\Microsoft\Windows\CurrentVersion\Internet Settings`)
	var handle syscall.Handle
	ret, _, _ := procRegOpenKey.Call(
		uintptr(HKEY_CURRENT_USER),
		uintptr(unsafe.Pointer(keyPath)),
		0,
		uintptr(KEY_SET_VALUE),
		uintptr(unsafe.Pointer(&handle)),
	)
	if ret != 0 {
		return fmt.Errorf("RegOpenKeyEx failed: %d", ret)
	}
	defer procRegCloseKey.Call(uintptr(handle))

	proxyEnable, _ := syscall.UTF16PtrFromString("ProxyEnable")
	disableVal := uint32(0)
	procRegSetValue.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(proxyEnable)),
		0,
		uintptr(REG_DWORD),
		uintptr(unsafe.Pointer(&disableVal)),
		unsafe.Sizeof(disableVal),
	)

	procInternetSetOption.Call(0, INTERNET_OPTION_SETTINGS_CHANGED, 0, 0)
	procInternetSetOption.Call(0, INTERNET_OPTION_REFRESH, 0, 0)

	w.enabled = false
	return nil
}

func (w *windowsManager) IsEnabled() bool {
	return w.enabled
}
