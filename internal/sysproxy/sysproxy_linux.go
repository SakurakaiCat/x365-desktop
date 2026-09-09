//go:build !windows && !darwin

package sysproxy

import (
	"fmt"
	"os/exec"
	"strings"
)

type linuxManager struct {
	enabled bool
}

func newManager() Manager {
	return &linuxManager{}
}

func (l *linuxManager) SetSOCKS5(host string, port int) error {
	addr := fmt.Sprintf("'%s'", host)
	if strings.Contains(host, ":") {
		addr = fmt.Sprintf("'%s'", host)
	}
	_ = exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "manual").Run()
	_ = exec.Command("gsettings", "set", "org.gnome.system.proxy.socks", "host", addr).Run()
	_ = exec.Command("gsettings", "set", "org.gnome.system.proxy.socks", "port", fmt.Sprintf("%d", port)).Run()
	l.enabled = true
	return nil
}

func (l *linuxManager) Clear() error {
	_ = exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "none").Run()
	l.enabled = false
	return nil
}

func (l *linuxManager) IsEnabled() bool {
	return l.enabled
}
