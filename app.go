package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	x365core "github.com/365vpn/x365/core"

	"github.com/365vpn/x365/desktop/internal/logbus"
	"github.com/365vpn/x365/desktop/internal/nodestore"
	"github.com/365vpn/x365/desktop/internal/probe"
	"github.com/365vpn/x365/desktop/internal/sysproxy"
	"github.com/365vpn/x365/desktop/internal/tun"
)

// App is the main application struct bound to the Wails frontend.
type App struct {
	ctx         context.Context
	proxy       *x365core.ProxyManager
	tray        *Tray
	store       *nodestore.Store
	sysProxy    sysproxy.Manager
	tun         *tun.Manager
	probe       *probe.Probe
	quitReq     atomic.Bool
	trayHidden  atomic.Bool
	connectedID string
	connectedMu sync.Mutex
}

// StatusDTO is returned to the frontend.
type StatusDTO struct {
	Running      bool   `json:"running"`
	ListenAddr   string `json:"listenAddr"`
	CurrentLabel string `json:"currentLabel"`
	CurrentPath  string `json:"currentPath"`
	CurrentServer string `json:"currentServer"`
	ConnectedID  string `json:"connectedId"`
	SysProxyOn   bool   `json:"sysProxyOn"`
	TunMode      bool   `json:"tunMode"`
	TunRunning   bool   `json:"tunRunning"`
}

// ExitInfoDTO is the exit-node network identity returned to the frontend.
type ExitInfoDTO struct {
	IP          string `json:"ip"`
	Country     string `json:"country"`
	CountryCode string `json:"countryCode"`
	ASN         string `json:"asn"`
	Org         string `json:"org"`
}

// TrafficDTO holds cumulative byte counters.
type TrafficDTO struct {
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
	UpBps    int64 `json:"upBps"`
	DownBps  int64 `json:"downBps"`
}

// NewApp creates the App instance.
func NewApp() *App {
	return &App{
		proxy: x365core.NewProxyManager(),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	store, err := nodestore.New()
	if err != nil {
		log.Printf("nodestore init: %v", err)
	}
	a.store = store
	a.sysProxy = sysproxy.New()
	a.tun = tun.New(tun.Config{
		Name:    "Open365VPN",
		Addr:    "172.16.0.1",
		Gateway: "172.16.0.1",
		Mask:    "255.255.255.0",
		DNS:     "8.8.8.8",
		MTU:     1500,
	})

	// Wire x365-core logs into our logbus
	x365core.SetLogger(func(format string, args ...interface{}) {
		logbus.PostGlobalf("[Go] "+format, args...)
	})

	// Forward logbus entries to the frontend via Wails events
	go a.forwardLogs()

	listenAddr := "127.0.0.1:10808"
	if store != nil {
		listenAddr = store.GetSettings().ListenAddr
	}
	a.probe = probe.New(listenAddr)

	a.tray = NewTray(a.trayIconBytes(), TrayCallbacks{
		OnShowWindow: func() {
			a.trayHidden.Store(false)
			wailsRuntime.WindowShow(a.ctx)
		},
		OnConnect: func(nodeID string) {
			if nodeID == "" {
				settings := a.store.GetSettings()
				nodeID = settings.LastNodeID
			}
			a.Connect(nodeID)
		},
		OnDisconnect: func() {
			a.Disconnect()
		},
		OnQuit: func() {
			a.quitReq.Store(true)
			wailsRuntime.Quit(a.ctx)
		},
	})

	if store != nil {
		a.refreshTrayNodes()
	}
	a.tray.Start()

	logbus.PostGlobal("[App] Open365VPN 启动完成")

	// Auto-connect if configured
	if store != nil {
		settings := store.GetSettings()
		if settings.AutoConnect && settings.LastNodeID != "" {
			go func() {
				if err := a.Connect(settings.LastNodeID); err != nil {
					logbus.PostGlobalf("[App] auto-connect failed: %v", err)
				}
			}()
		}
	}
}

func (a *App) shutdown(ctx context.Context) {
	if a.tun != nil {
		a.tun.Stop()
	}
	if a.sysProxy != nil {
		a.sysProxy.Clear()
	}
	if a.proxy != nil {
		a.proxy.Stop()
	}
	if a.tray != nil {
		a.tray.Stop()
	}
	logbus.PostGlobal("[App] 应用已关闭")
}

func (a *App) beforeClose(ctx context.Context) bool {
	if a.quitReq.Load() {
		return false
	}
	a.trayHidden.Store(true)
	wailsRuntime.WindowHide(ctx)
	return true
}

func (a *App) trayIconBytes() []byte {
	return appIcon
}

func (a *App) refreshTrayNodes() {
	if a.store == nil || a.tray == nil {
		return
	}
	nodes := a.store.GetNodes()
	infos := make([]nodeInfo, len(nodes))
	for i, n := range nodes {
		infos[i] = nodeInfo{ID: n.ID, Label: n.Label}
	}
	a.tray.SetNodes(infos)
}

// --- Frontend-bound methods ---

// GetNodes returns all stored nodes.
func (a *App) GetNodes() []nodestore.Node {
	if a.store == nil {
		return nil
	}
	return a.store.GetNodes()
}

// AddNode adds a node from an x365:// URI.
func (a *App) AddNode(uri string) (*nodestore.Node, error) {
	if a.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	node, err := a.store.AddNode(uri)
	if err != nil {
		return nil, err
	}
	a.store.Save()
	a.refreshTrayNodes()
	return node, nil
}

// ImportFromText imports multiple nodes from text (one URI per line).
func (a *App) ImportFromText(text string) (int, error) {
	if a.store == nil {
		return 0, fmt.Errorf("store not initialized")
	}
	count := 0
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "x365://") {
			continue
		}
		if _, err := a.store.AddNode(line); err != nil {
			log.Printf("skip %q: %v", line, err)
			continue
		}
		count++
	}
	a.store.Save()
	a.refreshTrayNodes()
	return count, nil
}

// RemoveNode removes a node by ID.
func (a *App) RemoveNode(id string) error {
	if a.store == nil {
		return fmt.Errorf("store not initialized")
	}
	if err := a.store.RemoveNode(id); err != nil {
		return err
	}
	a.store.Save()
	a.refreshTrayNodes()
	return nil
}

// RenameNode updates a node's label.
func (a *App) RenameNode(id string, newLabel string) error {
	if a.store == nil {
		return fmt.Errorf("store not initialized")
	}
	if err := a.store.RenameNode(id, newLabel); err != nil {
		return err
	}
	a.store.Save()
	a.refreshTrayNodes()
	return nil
}

// Connect starts the SOCKS5 proxy (and TUN if enabled) through the specified node.
func (a *App) Connect(nodeID string) error {
	if a.store == nil {
		return fmt.Errorf("store not initialized")
	}
	node, err := a.store.GetNode(nodeID)
	if err != nil {
		return err
	}
	cfg, err := x365core.ParseURI(node.URI)
	if err != nil {
		return err
	}
	settings := a.store.GetSettings()

	logbus.PostGlobalf("[App] 连接节点: %s (%s:%d%s)", node.Label, cfg.Server, cfg.Port, cfg.Path)

	// Stop existing connection first
	a.stopConnected()

	// Start SOCKS5 proxy
	if err := a.proxy.Start(cfg, settings.ListenAddr); err != nil {
		logbus.PostGlobalf("[Proxy] SOCKS5 启动失败: %v", err)
		return err
	}
	logbus.PostGlobalf("[Proxy] SOCKS5 监听于 %s", settings.ListenAddr)

	a.connectedMu.Lock()
	a.connectedID = nodeID
	a.connectedMu.Unlock()

	settings.LastNodeID = nodeID
	a.store.SetSettings(settings)

	// TUN mode: route all traffic through Wintun → SOCKS5
	if settings.TunMode {
		if err := a.tun.Start(settings.ListenAddr); err != nil {
			logbus.PostGlobalf("[TUN] TUN 模式启动失败，回退系统代理: %v", err)
			// Fallback to system proxy
			if a.sysProxy != nil && settings.AutoSysProxy {
				host, port := parseListenAddr(settings.ListenAddr)
				a.sysProxy.SetSOCKS5(host, port)
			}
		}
	} else if a.sysProxy != nil && settings.AutoSysProxy {
		host, port := parseListenAddr(settings.ListenAddr)
		a.sysProxy.SetSOCKS5(host, port)
	}

	if a.tray != nil {
		a.tray.SetConnected(node.Label)
	}
	wailsRuntime.EventsEmit(a.ctx, "proxy:connected", node)
	return nil
}

// Disconnect stops the SOCKS5 proxy and TUN device.
func (a *App) Disconnect() error {
	logbus.PostGlobal("[App] 断开连接")
	a.stopConnected()

	if a.tray != nil {
		a.tray.SetDisconnected()
	}
	wailsRuntime.EventsEmit(a.ctx, "proxy:disconnected", nil)
	return nil
}

func (a *App) stopConnected() {
	if a.tun != nil {
		a.tun.Stop()
	}
	if a.sysProxy != nil {
		a.sysProxy.Clear()
	}
	a.proxy.Stop()

	a.connectedMu.Lock()
	a.connectedID = ""
	a.connectedMu.Unlock()
}

// GetStatus returns the current proxy status.
func (a *App) GetStatus() StatusDTO {
	settings := nodestore.Settings{}
	if a.store != nil {
		settings = a.store.GetSettings()
	}
	dto := StatusDTO{
		Running:    a.proxy.IsRunning(),
		ListenAddr: settings.ListenAddr,
		TunMode:    settings.TunMode,
		TunRunning: a.tun.IsRunning(),
		SysProxyOn: false,
	}
	if a.sysProxy != nil {
		dto.SysProxyOn = a.sysProxy.IsEnabled()
	}
	a.connectedMu.Lock()
	dto.ConnectedID = a.connectedID
	a.connectedMu.Unlock()
	if dto.ConnectedID != "" && a.store != nil {
		if node, err := a.store.GetNode(dto.ConnectedID); err == nil {
			dto.CurrentLabel = node.Label
			dto.CurrentPath = node.Path
			dto.CurrentServer = node.Server
		}
	}
	return dto
}

// GetSettings returns the current settings.
func (a *App) GetSettings() nodestore.Settings {
	if a.store == nil {
		return nodestore.Settings{ListenAddr: "127.0.0.1:10808", TunMode: true}
	}
	return a.store.GetSettings()
}

// SaveSettings updates and persists settings.
func (a *App) SaveSettings(s nodestore.Settings) {
	if a.store == nil {
		return
	}
	a.store.SetSettings(s)
	a.probe.SetSocksAddr(s.ListenAddr)
}

// SetSystemProxy enables or disables system proxy pointing at the SOCKS5 listener.
func (a *App) SetSystemProxy(enabled bool) error {
	if a.sysProxy == nil {
		return fmt.Errorf("sysproxy not available")
	}
	if enabled {
		settings := a.store.GetSettings()
		host, port := parseListenAddr(settings.ListenAddr)
		return a.sysProxy.SetSOCKS5(host, port)
	}
	return a.sysProxy.Clear()
}

// GetExitInfo queries the exit node's IP/ASN through the local SOCKS5.
func (a *App) GetExitInfo() (*ExitInfoDTO, error) {
	if !a.proxy.IsRunning() {
		return nil, fmt.Errorf("proxy not running")
	}
	info, err := a.probe.QueryExitInfo(true)
	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, nil
	}
	return &ExitInfoDTO{
		IP:          info.IP,
		Country:     info.Country,
		CountryCode: info.CountryCode,
		ASN:         info.ASN,
		Org:         info.Org,
	}, nil
}

// TestLatency measures the latency to a node in milliseconds.
func (a *App) TestLatency(nodeID string) (int64, error) {
	if a.store == nil {
		return 0, fmt.Errorf("store not initialized")
	}
	node, err := a.store.GetNode(nodeID)
	if err != nil {
		return 0, err
	}
	logbus.PostGlobalf("[Probe] 测速: %s", node.Label)
	ms, err := probe.MeasureLatency(node.URI)
	if err != nil {
		logbus.PostGlobalf("[Probe] 测速失败: %v", err)
		return 0, err
	}
	logbus.PostGlobalf("[Probe] %s 延迟 %dms", node.Label, ms)
	return ms, nil
}

// GetLogs returns the current log buffer.
func (a *App) GetLogs() []string {
	return logbus.LinesGlobal()
}

// ClearLogs empties the log buffer.
func (a *App) ClearLogs() {
	logbus.ClearGlobal()
}

// GetTraffic returns current traffic statistics.
func (a *App) GetTraffic() TrafficDTO {
	if a.tun == nil || !a.tun.IsRunning() {
		return TrafficDTO{}
	}
	up, down := a.tun.Traffic()
	upBps, downBps := a.tun.Rate()
	return TrafficDTO{
		Upload:   up,
		Download: down,
		UpBps:    upBps,
		DownBps:  downBps,
	}
}

// startTrafficPusher periodically emits traffic updates to the frontend while connected.
func (a *App) startTrafficPusher() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			if a.proxy.IsRunning() {
				dto := a.GetTraffic()
				wailsRuntime.EventsEmit(a.ctx, "proxy:traffic", dto)
			}
		}
	}
}

// forwardLogs subscribes to the logbus and emits each new line as a Wails event.
func (a *App) forwardLogs() {
	ch := logbus.Default.Subscribe()
	defer logbus.Default.Unsubscribe(ch)
	for {
		select {
		case <-a.ctx.Done():
			return
		case line, ok := <-ch:
			if !ok {
				return
			}
			wailsRuntime.EventsEmit(a.ctx, "proxy:log", line)
		}
	}
}

// OnDomReady is called when the frontend DOM is ready. We start the traffic
// pusher goroutine here.
func (a *App) OnDomReady(ctx context.Context) {
	go a.startTrafficPusher()
}

func parseListenAddr(addr string) (string, int) {
	host, port := addr, 10808
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			host = addr[:i]
			fmt.Sscanf(addr[i+1:], "%d", &port)
			break
		}
	}
	return host, port
}