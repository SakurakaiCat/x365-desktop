package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"sync"
	"time"

	x365core "github.com/SakurakaiCat/x365-core"

	"github.com/SakurakaiCat/x365-desktop/internal/logbus"
)

// ExitInfo holds the exit-node network identity, queried through the local SOCKS5.
type ExitInfo struct {
	IP          string `json:"ip"`
	Country     string `json:"country"`
	CountryCode string `json:"countryCode"`
	ASN         string `json:"asn"`
	Org         string `json:"org"`
}

// Probe handles network diagnostics: exit-IP lookup and per-node latency.
type Probe struct {
	mu             sync.Mutex
	lastQuery      time.Time
	minInterval    time.Duration
	socksAddr      string
}

// New creates a Probe that targets the given local SOCKS5 address.
func New(socksAddr string) *Probe {
	return &Probe{
		socksAddr:   socksAddr,
		minInterval: 15 * time.Second,
	}
}

// SetSocksAddr updates the SOCKS5 address used for exit-info queries.
func (p *Probe) SetSocksAddr(addr string) {
	p.mu.Lock()
	p.socksAddr = addr
	p.mu.Unlock()
}

// QueryExitInfo fetches exit IP/ASN via ip-api.com through the local SOCKS5.
// When force is false, results are rate-limited to minInterval.
func (p *Probe) QueryExitInfo(force bool) (*ExitInfo, error) {
	p.mu.Lock()
	now := time.Now()
	if !force && now.Sub(p.lastQuery) < p.minInterval {
		p.mu.Unlock()
		return nil, nil
	}
	p.lastQuery = now
	addr := p.socksAddr
	p.mu.Unlock()

	client := &httpClient{
		socksAddr: addr,
		timeout:   8 * time.Second,
	}

	body, err := client.get("http://ip-api.com/json/?fields=status,country,countryCode,query,as,asname")
	if err != nil {
		logbus.PostGlobalf("[Probe] exit info query failed: %v", err)
		return nil, err
	}

	var raw struct {
		Status      string `json:"status"`
		Query       string `json:"query"`
		Country     string `json:"country"`
		CountryCode string `json:"countryCode"`
		AS          string `json:"as"`
		ASName      string `json:"asname"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	if raw.Status != "success" {
		return nil, fmt.Errorf("ip-api returned: %s", raw.Status)
	}

	info := &ExitInfo{
		IP:          raw.Query,
		Country:     raw.Country,
		CountryCode: raw.CountryCode,
		ASN:         raw.AS,
		Org:         raw.ASName,
	}
	return info, nil
}

// MeasureLatency dials example.com:80 through the X365 tunnel for the given
// node URI and returns the elapsed milliseconds. Returns error on failure.
func MeasureLatency(nodeURI string) (int64, error) {
	cfg, err := x365core.ParseURI(nodeURI)
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	start := time.Now()
	conn, err := x365core.Dial(ctx, cfg, "example.com", 80)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("GET / HTTP/1.0\r\nHost: example.com\r\n\r\n")); err != nil {
		return 0, err
	}
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	buf := make([]byte, 128)
	if _, err := conn.Read(buf); err != nil {
		return 0, err
	}
	return time.Since(start).Milliseconds(), nil
}

// --- minimal SOCKS5 HTTP client ---

type httpClient struct {
	socksAddr string
	timeout   time.Duration
}

func (c *httpClient) get(rawURL string) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	host := u.Host
	if u.Port() == "" {
		host = net.JoinHostPort(host, "80")
	}

	// Dial through SOCKS5
	socksConn, err := socks5Dial(c.socksAddr, host, c.timeout)
	if err != nil {
		return nil, err
	}
	defer socksConn.Close()

	req := fmt.Sprintf("GET %s HTTP/1.0\r\nHost: %s\r\nConnection: close\r\n\r\n", u.RequestURI(), u.Hostname())
	if _, err := socksConn.Write([]byte(req)); err != nil {
		return nil, err
	}

	resp, err := io.ReadAll(socksConn)
	if err != nil {
		return nil, err
	}
	// Strip HTTP headers
	idx := indexByteSlice(resp, "\r\n\r\n")
	if idx >= 0 {
		resp = resp[idx+4:]
	}
	return resp, nil
}

func indexByteSlice(s []byte, sep string) int {
	for i := 0; i <= len(s)-len(sep); i++ {
		match := true
		for j := 0; j < len(sep); j++ {
			if s[i+j] != sep[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// socks5Dial connects to target through a SOCKS5 proxy.
func socks5Dial(proxyAddr, targetAddr string, timeout time.Duration) (net.Conn, error) {
	proxy, err := net.DialTimeout("tcp", proxyAddr, timeout)
	if err != nil {
		return nil, err
	}
	if err := proxy.SetDeadline(time.Now().Add(timeout)); err != nil {
		proxy.Close()
		return nil, err
	}

	// Greeting: no auth
	if _, err := proxy.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		proxy.Close()
		return nil, err
	}
	buf := make([]byte, 2)
	if _, err := io.ReadFull(proxy, buf); err != nil {
		proxy.Close()
		return nil, err
	}
	if buf[0] != 0x05 || buf[1] != 0x00 {
		proxy.Close()
		return nil, fmt.Errorf("socks5 auth rejected: %v", buf)
	}

	host, portStr, err := net.SplitHostPort(targetAddr)
	if err != nil {
		proxy.Close()
		return nil, err
	}
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	// Connect request
	req := []byte{0x05, 0x01, 0x00}
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			req = append(req, 0x01)
			req = append(req, ip4...)
		} else {
			req = append(req, 0x04)
			req = append(req, ip.To16()...)
		}
	} else {
		req = append(req, 0x03, byte(len(host)))
		req = append(req, []byte(host)...)
	}
	req = append(req, byte(port>>8), byte(port&0xff))

	if _, err := proxy.Write(req); err != nil {
		proxy.Close()
		return nil, err
	}
	resp := make([]byte, 4)
	if _, err := io.ReadFull(proxy, resp); err != nil {
		proxy.Close()
		return nil, err
	}
	if resp[1] != 0x00 {
		proxy.Close()
		return nil, fmt.Errorf("socks5 connect rejected: %d", resp[1])
	}

	// Read bind address
	var skip int
	switch resp[3] {
	case 0x01:
		skip = 4
	case 0x03:
		l := make([]byte, 1)
		if _, err := io.ReadFull(proxy, l); err != nil {
			proxy.Close()
			return nil, err
		}
		skip = int(l[0])
	case 0x04:
		skip = 16
	default:
		proxy.Close()
		return nil, fmt.Errorf("socks5 unknown bind atyp: %d", resp[3])
	}
	skipBuf := make([]byte, skip+2)
	if _, err := io.ReadFull(proxy, skipBuf); err != nil {
		proxy.Close()
		return nil, err
	}

	// Clear deadline for the caller
	proxy.SetDeadline(time.Time{})
	return proxy, nil
}