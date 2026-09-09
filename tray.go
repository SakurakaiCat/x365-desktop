package main

import (
	"log"
	"sync"

	"github.com/energye/systray"
)

// TrayCallbacks holds callbacks for tray menu actions.
type TrayCallbacks struct {
	OnShowWindow   func()
	OnConnect      func(nodeID string)
	OnDisconnect   func()
	OnQuit         func()
}

// Tray wraps the systray functionality.
type Tray struct {
	mu          sync.Mutex
	icon        []byte
	cb          TrayCallbacks
	running     bool
	exited      chan struct{}
	stopReq     bool
	miShow      *systray.MenuItem
	miConn      *systray.MenuItem
	miDisc      *systray.MenuItem
	miQuit      *systray.MenuItem
	miNodes     *systray.MenuItem
	nodeItems   map[string]*systray.MenuItem
	nodes       []nodeInfo
}

type nodeInfo struct {
	ID    string
	Label string
}

// NewTray creates a new system tray wrapper.
func NewTray(icon []byte, cb TrayCallbacks) *Tray {
	return &Tray{
		icon:      icon,
		cb:        cb,
		exited:    make(chan struct{}),
		nodeItems: make(map[string]*systray.MenuItem),
	}
}

// Start runs the system tray on its own goroutine.
func (t *Tray) Start() {
	go systray.Run(t.onReady, t.onExit)
}

// Stop requests the tray to shut down.
func (t *Tray) Stop() {
	t.mu.Lock()
	t.stopReq = true
	running := t.running
	t.mu.Unlock()
	if running {
		systray.Quit()
		select {
		case <-t.exited:
		default:
		}
	}
}

func (t *Tray) onReady() {
	t.mu.Lock()
	t.running = true
	t.mu.Unlock()

	systray.SetIcon(t.icon)
	systray.SetTitle("X365")
	systray.SetTooltip("X365 Client — Disconnected")

	t.miShow = systray.AddMenuItem("Show Window", "Show the main window")
	t.miShow.Click(func() { go t.safeCall(t.cb.OnShowWindow) })

	systray.AddSeparator()

	t.miNodes = systray.AddMenuItem("Nodes", "Select a node")
	for _, n := range t.nodes {
		item := t.miNodes.AddSubMenuItemCheckbox(n.Label, "", false)
		item.Click(func() { go t.safeCall(func() { t.cb.OnConnect(n.ID) }) })
		t.nodeItems[n.ID] = item
	}

	systray.AddSeparator()

	t.miConn = systray.AddMenuItem("Connect", "Connect to current node")
	t.miConn.Click(func() { go t.safeCall(func() { t.cb.OnConnect("") }) })

	t.miDisc = systray.AddMenuItem("Disconnect", "Disconnect")
	t.miDisc.Click(func() { go t.safeCall(t.cb.OnDisconnect) })
	t.miDisc.Disable()

	systray.AddSeparator()

	t.miQuit = systray.AddMenuItem("Quit", "Quit the application")
	t.miQuit.Click(func() { go t.safeCall(t.cb.OnQuit) })
}

func (t *Tray) onExit() {
	t.mu.Lock()
	t.running = false
	close(t.exited)
	t.mu.Unlock()
}

func (t *Tray) safeCall(fn func()) {
	if fn == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[tray] panic: %v", r)
		}
	}()
	fn()
}

// SetConnected updates the tray to show connected state.
func (t *Tray) SetConnected(label string) {
	systray.SetTooltip("X365 Client — " + label)
	if t.miConn != nil {
		t.miConn.Disable()
	}
	if t.miDisc != nil {
		t.miDisc.Enable()
	}
}

// SetDisconnected updates the tray to show disconnected state.
func (t *Tray) SetDisconnected() {
	systray.SetTooltip("X365 Client — Disconnected")
	if t.miConn != nil {
		t.miConn.Enable()
	}
	if t.miDisc != nil {
		t.miDisc.Disable()
	}
}

// SetNodes updates the node list in the tray menu.
func (t *Tray) SetNodes(nodes []nodeInfo) {
	t.mu.Lock()
	t.nodes = nodes
	t.mu.Unlock()
	// If tray already ready, rebuild submenu items
	if t.miNodes != nil {
		for _, item := range t.nodeItems {
			item.Hide()
		}
		t.nodeItems = make(map[string]*systray.MenuItem)
		for _, n := range nodes {
			item := t.miNodes.AddSubMenuItemCheckbox(n.Label, "", false)
			item.Click(func() { go t.safeCall(func() { t.cb.OnConnect(n.ID) }) })
			t.nodeItems[n.ID] = item
		}
	}
}
