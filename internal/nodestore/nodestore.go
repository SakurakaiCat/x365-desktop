package nodestore

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	x365core "github.com/SakurakaiCat/x365-core"
)

// Node represents a stored proxy node.
type Node struct {
	ID          string `json:"id"`
	URI         string `json:"uri"`
	Label       string `json:"label"`
	Server      string `json:"server"`
	Port        uint16 `json:"port"`
	Path        string `json:"path"`
	CountryCode string `json:"countryCode,omitempty"`
	UUID        string `json:"uuid,omitempty"`
	SNI         string `json:"sni,omitempty"`
	PublicKey   string `json:"pbk,omitempty"`
	ShortID     string `json:"sid,omitempty"`
}

// Settings stores user preferences.
type Settings struct {
	ListenAddr   string `json:"listenAddr"`
	AutoConnect  bool   `json:"autoConnect"`
	AutoSysProxy bool   `json:"autoSysProxy"`
	TunMode      bool   `json:"tunMode"`
	LastNodeID   string `json:"lastNodeId"`
}

// Store manages nodes and settings in a JSON file.
type Store struct {
	mu       sync.Mutex
	filePath string
	Nodes    []Node   `json:"nodes"`
	Settings Settings `json:"settings"`
}

func configDir() string {
	if dir := os.Getenv("APPDATA"); dir != "" {
		return filepath.Join(dir, "X365Client")
	}
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "x365-client")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config", "x365-client")
	}
	return "."
}

// DefaultNodes is empty: the open-source release bundles no server
// credentials. Users import their own x365:// URIs.
var DefaultNodes = []string{}

// labelToISO maps Chinese node labels to ISO 3166-1 alpha-2 country codes
// (for flag emoji display), mirroring the Android client.
var labelToISO = map[string]string{
	"独立IP":       "US",
	"香港":          "HK",
	"日本":          "JP",
	"新加坡":        "SG",
	"台湾":          "TW",
	"美国":          "US",
	"韩国":          "KR",
	"英国":          "GB",
	"德国":          "DE",
	"加拿大":        "CA",
	"澳大利亚":      "AU",
	"泰国":          "TH",
	"马来西亚":      "MY",
	"印度尼西亚":    "ID",
	"柬埔寨":        "KH",
	"阿联酋":        "AE",
	"俄罗斯":        "RU",
	"土耳其":        "TR",
	"西班牙":        "ES",
	"意大利":        "IT",
	"荷兰":          "NL",
	"芬兰":          "FI",
	"波兰":          "PL",
	"瑞士":          "CH",
	"奥地利":        "AT",
	"巴西":          "BR",
	"保加利亚":      "BG",
	"立陶宛":        "LT",
	"罗马尼亚":      "RO",
	"葡萄牙":        "PT",
}

// isoFromPathOrLabel infers the ISO country code from the URI path (e.g. "/hk")
// or the node label (e.g. "香港").
func isoFromPathOrLabel(path, label string) string {
	code := strings.ToUpper(strings.TrimPrefix(path, "/"))
	if len(code) == 2 && strings.IndexFunc(code, func(r rune) bool { return !(r >= 'A' && r <= 'Z') }) == -1 {
		return code
	}
	if iso, ok := labelToISO[strings.TrimSpace(label)]; ok {
		return iso
	}
	return ""
}

// New creates a Store, loading from the default config path.
// If no saved nodes exist, the default node list is imported automatically.
func New() (*Store, error) {
	dir := configDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}
	s := &Store{
		filePath: filepath.Join(dir, "nodes.json"),
		Settings: Settings{
			ListenAddr: "127.0.0.1:10808",
			TunMode:    true,
		},
	}
	if err := s.Load(); err != nil {
		return nil, err
	}
	// First run: seed default nodes
	if len(s.Nodes) == 0 {
		for _, uri := range DefaultNodes {
			if node, err := nodeFromURI(uri); err == nil {
				s.Nodes = append(s.Nodes, *node)
			}
		}
		s.Save()
	}
	return s, nil
}

// Load reads the JSON config file into the store.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var loaded Store
	if err := json.Unmarshal(data, &loaded); err != nil {
		return err
	}
	s.Nodes = loaded.Nodes
	if loaded.Settings.ListenAddr != "" {
		s.Settings = loaded.Settings
	}
	return nil
}

// Save writes the store to the JSON config file.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0644)
}

// nodeFromURI parses an x365:// URI into a Node (without assigning an ID).
func nodeFromURI(uri string) (*Node, error) {
	uri = strings.TrimSpace(uri)
	cfg, err := x365core.ParseURI(uri)
	if err != nil {
		return nil, err
	}
	label := x365core.Label(uri)
	if label == "" {
		label = strings.TrimPrefix(cfg.Path, "/")
	}
	path := cfg.Path
	if path == "" {
		path = "/"
	}
	uuidStr := ""
	if atIdx := strings.Index(uri, "@"); atIdx > 0 {
		uuidStr = strings.TrimPrefix(uri[:atIdx], "x365://")
	}
	return &Node{
		URI:         uri,
		Label:       label,
		Server:      cfg.Server,
		Port:        cfg.Port,
		Path:        path,
		CountryCode: isoFromPathOrLabel(path, label),
		UUID:        uuidStr,
		SNI:         cfg.SNI,
		PublicKey:   cfg.PublicKey,
		ShortID:     cfg.ShortID,
	}, nil
}

// AddNode parses and adds a node from a URI.
func (s *Store) AddNode(uri string) (*Node, error) {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return nil, fmt.Errorf("empty URI")
	}
	node, err := nodeFromURI(uri)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, n := range s.Nodes {
		if n.URI == uri {
			return &n, nil
		}
	}
	node.ID = randomID()
	s.Nodes = append(s.Nodes, *node)
	return node, nil
}

// RemoveNode removes a node by ID.
func (s *Store) RemoveNode(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, n := range s.Nodes {
		if n.ID == id {
			s.Nodes = append(s.Nodes[:i], s.Nodes[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("node not found: %s", id)
}

// RenameNode updates a node's label and rebuilds its URI fragment.
func (s *Store) RenameNode(id, newLabel string) error {
	newLabel = strings.TrimSpace(newLabel)
	if newLabel == "" {
		return fmt.Errorf("label cannot be empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.Nodes {
		if s.Nodes[i].ID == id {
			base := s.Nodes[i].URI
			if idx := strings.Index(base, "#"); idx >= 0 {
				base = base[:idx]
			}
			s.Nodes[i].URI = base + "#" + newLabel
			s.Nodes[i].Label = newLabel
			return nil
		}
	}
	return fmt.Errorf("node not found: %s", id)
}

// GetNodes returns all nodes sorted by label.
func (s *Store) GetNodes() []Node {
	s.mu.Lock()
	defer s.mu.Unlock()
	nodes := make([]Node, len(s.Nodes))
	copy(nodes, s.Nodes)
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Label < nodes[j].Label
	})
	return nodes
}

// GetNode returns a node by ID.
func (s *Store) GetNode(id string) (*Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.Nodes {
		if s.Nodes[i].ID == id {
			return &s.Nodes[i], nil
		}
	}
	return nil, fmt.Errorf("node not found: %s", id)
}

// GetSettings returns the current settings.
func (s *Store) GetSettings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Settings
}

// SetSettings updates and persists settings.
func (s *Store) SetSettings(settings Settings) {
	s.mu.Lock()
	s.Settings = settings
	s.mu.Unlock()
	s.Save()
}

func randomID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}