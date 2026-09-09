//go:build darwin

package sysproxy

import (
	"fmt"
	"os/exec"
	"strings"
)

type darwinManager struct {
	enabled bool
	iface   string
}

func newManager() Manager {
	return &darwinManager{iface: "Wi-Fi"}
}

func (d *darwinManager) SetSOCKS5(host string, port int) error {
	_ = exec.Command("networksetup", "-setsocksfirewallproxy", d.iface, host, fmt.Sprintf("%d", port)).Run()
	d.enabled = true
	return nil
}

func (d *darwinManager) Clear() error {
	_ = exec.Command("networksetup", "-setsocksfirewallproxystate", d.iface, "off").Run()
	d.enabled = false
	return nil
}

func (d *darwinManager) IsEnabled() bool {
	out, err := exec.Command("networksetup", "-getsocksfirewallproxy", d.iface).Output()
	if err != nil {
		return d.enabled
	}
	return strings.Contains(string(out), "Enabled: Yes")
}
