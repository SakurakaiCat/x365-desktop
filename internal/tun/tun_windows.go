//go:build windows

// Package tun manages the Wintun TUN device + gVisor netstack tun2socks engine.
// On Windows, it creates a Wintun adapter, routes all traffic through it, and
// tunnels it to the local SOCKS5 proxy. On non-Windows platforms, a no-op stub
// is provided (see tun_other.go).
package tun

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/xjasonlyu/tun2socks/v2/engine"
	t2slog "github.com/xjasonlyu/tun2socks/v2/log"
	t2sstats "github.com/xjasonlyu/tun2socks/v2/tunnel/statistic"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/SakurakaiCat/x365-desktop/internal/logbus"
)

// Manager owns the tun2socks engine lifecycle for a single SOCKS5 upstream.
type Manager struct {
	mu        sync.Mutex
	running   bool
	tunName   string
	tunAddr   string
	tunGw     string
	tunMask   string
	tunDNS    string
	mtu       int
	postUpCmd string
}

// Config holds TUN device parameters.
type Config struct {
	Name    string // Wintun adapter name
	Addr    string // TUN interface IPv4 (e.g. "172.16.0.1")
	Gateway string // TUN gateway (e.g. "172.16.0.1")
	Mask    string // netmask (e.g. "255.255.255.0")
	DNS     string // comma-separated DNS (e.g. "8.8.8.8")
	MTU     int    // 1500
}

// New creates a Manager with the given TUN config.
func New(cfg Config) *Manager {
	mtu := cfg.MTU
	if mtu == 0 {
		mtu = 1500
	}
	return &Manager{
		tunName: cfg.Name,
		tunAddr: cfg.Addr,
		tunGw:   cfg.Gateway,
		tunMask: cfg.Mask,
		tunDNS:  cfg.DNS,
		mtu:     mtu,
	}
}

// logbusWriteSyncer forwards zap-encoded log lines to our internal logbus.
type logbusWriteSyncer struct{}

func (logbusWriteSyncer) Write(p []byte) (int, error) {
	msg := strings.TrimSpace(string(p))
	if msg != "" {
		logbus.PostGlobalf("[t2s] %s", msg)
	}
	return len(p), nil
}
func (logbusWriteSyncer) Sync() error { return nil }

// Start launches the tun2socks engine, connecting the Wintun TUN device to the
// SOCKS5 proxy at socksAddr. The engine handles the full netstack → SOCKS5 path.
func (m *Manager) Start(socksAddr string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		logbus.PostGlobal("[TUN] already running, stopping first")
		m.stopLocked()
	}

	gw := m.tunGw
	if gw == "" {
		gw = m.tunAddr
	}

	// Route all IPv4 traffic through the TUN adapter's gateway.
	postUp := fmt.Sprintf(`route add 0.0.0.0 mask 0.0.0.0 %s metric 5`, gw)

	key := &engine.Key{
		MTU:        m.mtu,
		Proxy:      "socks5://" + socksAddr,
		Device:     "tun://" + m.tunName,
		LogLevel:   "info",
		TUNPostUp:  postUp,
		UDPTimeout: 60 * time.Second,
	}

	// Redirect tun2socks' zap logs into our logbus.
	ws := logbusWriteSyncer{}
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()),
		zapcore.AddSync(ws),
		zapcore.InfoLevel,
	)
	t2slog.SetLogger(zap.New(core))

	logbus.PostGlobalf("[TUN] starting tun2socks: device=%s proxy=socks5://%s mtu=%d", m.tunName, socksAddr, m.mtu)
	logbus.PostGlobalf("[TUN] post-up: %s", postUp)

	engine.Insert(key)
	if err := engineStartSafe(); err != nil {
		logbus.PostGlobalf("[TUN] engine start failed: %v", err)
		return fmt.Errorf("tun2socks engine: %w", err)
	}

	m.running = true
	m.postUpCmd = postUp
	logbus.PostGlobalf("[TUN] Wintun adapter '%s' created, traffic routed through SOCKS5", m.tunName)
	return nil
}

// engineStartSafe wraps engine.Start() to recover from panics. Note: on fatal
// errors engine.Start() calls log.Fatalf → os.Exit, which is unrecoverable.
func engineStartSafe() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("engine panic: %v", r)
		}
	}()
	engine.Start()
	return nil
}

// Stop tears down the TUN device and removes routes.
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopLocked()
}

func (m *Manager) stopLocked() error {
	if !m.running {
		return nil
	}
	m.running = false

	logbus.PostGlobal("[TUN] stopping tun2socks engine")
	engine.Stop()

	if m.postUpCmd != "" {
		delCmd := strings.Replace(m.postUpCmd, "add", "delete", 1)
		logbus.PostGlobalf("[TUN] route cleanup: %s", delCmd)
		parts := strings.Fields(delCmd)
		if len(parts) > 0 {
			exec.Command(parts[0], parts[1:]...).Run()
		}
	}
	m.postUpCmd = ""
	logbus.PostGlobal("[TUN] Wintun adapter removed")
	return nil
}

// IsRunning returns whether the TUN is currently active.
func (m *Manager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// Traffic returns cumulative upload/download bytes through the netstack.
func (m *Manager) Traffic() (upload, download int64) {
	snap := t2sstats.DefaultManager.Snapshot()
	if snap == nil {
		return 0, 0
	}
	return snap.UploadTotal, snap.DownloadTotal
}

// Rate returns current upload/download bytes-per-second.
func (m *Manager) Rate() (upBps, downBps int64) {
	return t2sstats.DefaultManager.Now()
}