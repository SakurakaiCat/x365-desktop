//go:build !windows

// Package tun provides a no-op TUN manager on non-Windows platforms.
// The real Wintun implementation lives in tun_windows.go.
package tun

import (
	"errors"

	"github.com/365vpn/x365/desktop/internal/logbus"
)

type Manager struct{}

type Config struct {
	Name    string
	Addr    string
	Gateway string
	Mask    string
	DNS     string
	MTU     int
}

func New(cfg Config) *Manager { return &Manager{} }

func (m *Manager) Start(socksAddr string) error {
	logbus.PostGlobal("[TUN] TUN mode not supported on this platform, using system proxy fallback")
	return errors.New("TUN mode is only available on Windows")
}

func (m *Manager) Stop() error             { return nil }
func (m *Manager) IsRunning() bool         { return false }
func (m *Manager) Traffic() (int64, int64) { return 0, 0 }
func (m *Manager) Rate() (int64, int64)    { return 0, 0 }