package paneluri

import (
	"bytes"
	"compress/flate"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
)

const maxAdditionalPayload = 64 * 1024

func shareJSON(encoded string, compressed bool, value any) error {
	raw, e := DecodeBase64(encoded)
	if e != nil {
		return e
	}
	if len(raw) > maxAdditionalPayload {
		return errors.New("share payload exceeds 64 KiB")
	}
	if compressed {
		r := flate.NewReader(bytes.NewReader(raw))
		defer r.Close()
		raw, e = io.ReadAll(io.LimitReader(r, maxAdditionalPayload+1))
		if e != nil {
			return errors.New("invalid compressed share payload")
		}
		if len(raw) > maxAdditionalPayload {
			return errors.New("decompressed share exceeds 64 KiB")
		}
	}
	if json.Unmarshal(raw, value) != nil {
		return errors.New("invalid share JSON")
	}
	return nil
}
func validHostPort(host string, port int) bool {
	_, _, e := address(net.JoinHostPort(host, strconv.Itoa(port)))
	return e == nil && !strings.ContainsAny(host, "\r\n\x00 /?#@")
}
func sudokuLink(link string) (Result, error) {
	var p struct {
		Host     string   `json:"h"`
		Port     int      `json:"p"`
		Key      string   `json:"k"`
		ASCII    string   `json:"a"`
		AEAD     string   `json:"e"`
		Packed   bool     `json:"x"`
		Table    string   `json:"t"`
		Tables   []string `json:"ts"`
		Disable  bool     `json:"hd"`
		Mode     string   `json:"hm"`
		TLS      bool     `json:"ht"`
		HTTPHost string   `json:"hh"`
		Path     string   `json:"hy"`
		Mux      string   `json:"hx"`
	}
	if !strings.HasPrefix(link, "sudoku://") {
		return Result{}, errors.New("invalid Sudoku link")
	}
	if e := shareJSON(strings.TrimPrefix(link, "sudoku://"), false, &p); e != nil {
		return Result{}, e
	}
	if !validHostPort(p.Host, p.Port) || p.Key == "" {
		return Result{}, errors.New("Sudoku endpoint and key are required")
	}
	if p.ASCII == "" {
		p.ASCII = "entropy"
	}
	switch p.ASCII {
	case "ascii", "entropy", "prefer_ascii", "prefer_entropy", "up_ascii_down_entropy", "up_entropy_down_ascii":
	default:
		return Result{}, errors.New("invalid Sudoku appearance mode")
	}
	if p.AEAD == "" {
		p.AEAD = "none"
	}
	switch p.AEAD {
	case "none", "chacha20-poly1305", "aes-128-gcm":
	default:
		return Result{}, errors.New("unsupported Sudoku AEAD")
	}
	switch p.Mode {
	case "", "legacy", "stream", "poll", "auto", "ws":
	default:
		return Result{}, errors.New("invalid Sudoku HTTPMask mode")
	}
	switch p.Mux {
	case "", "off", "auto", "on":
	default:
		return Result{}, errors.New("invalid Sudoku multiplex mode")
	}
	if len(p.Tables) == 0 && p.Table != "" {
		p.Tables = []string{p.Table}
	}
	out := map[string]any{"type": "sudoku", "tag": "Sudoku", "server": p.Host, "server_port": p.Port, "key": p.Key, "ascii": p.ASCII, "aead": p.AEAD, "enable_pure_downlink": !p.Packed, "padding_min": 5, "padding_max": 15, "httpmask": map[string]any{"disable": p.Disable, "mode": p.Mode, "tls": p.TLS, "host": p.HTTPHost, "path_root": p.Path, "multiplex": p.Mux}}
	if len(p.Tables) > 0 {
		out["custom_tables"] = p.Tables
	}
	return Result{Outbounds: []map[string]any{out}}, nil
}
func openFluxLink(link string) (Result, error) {
	var p struct {
		Name       string `json:"name"`
		Secret     string `json:"secret"`
		Context    string `json:"context"`
		Codec      string `json:"codec"`
		Negotiate  bool   `json:"negotiate"`
		Transports []struct {
			Type     string `json:"type"`
			URL      string `json:"url"`
			Dial     string `json:"dial"`
			Priority int    `json:"priority"`
		} `json:"transports"`
	}
	if !strings.HasPrefix(link, "openflux://v1/") {
		return Result{}, errors.New("unsupported OpenFlux share version")
	}
	if e := shareJSON(strings.TrimPrefix(link, "openflux://v1/"), true, &p); e != nil {
		return Result{}, e
	}
	if len([]rune(p.Secret)) < 32 || len(p.Transports) == 0 || len(p.Transports) > 16 {
		return Result{}, errors.New("invalid OpenFlux secret or transport count")
	}
	if p.Codec == "" {
		p.Codec = "batched"
	}
	if p.Codec != "batched" {
		return Result{}, errors.New("unsupported OpenFlux codec")
	}
	seen := map[string]bool{}
	transports := []map[string]any{}
	for _, t := range p.Transports {
		if seen[t.Type] || t.Priority < 0 || t.Priority > 1000 {
			return Result{}, errors.New("duplicate OpenFlux carrier or invalid priority")
		}
		seen[t.Type] = true
		v := map[string]any{"type": t.Type, "priority": t.Priority}
		switch t.Type {
		case "direct":
			if _, _, e := address(t.Dial); e != nil {
				return Result{}, e
			}
			v["dial"] = t.Dial
		case "yandex", "vyandex", "boards", "mailru":
			u, e := url.Parse(t.URL)
			if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
				return Result{}, errors.New("invalid OpenFlux channel URL")
			}
			v["url"] = t.URL
		default:
			return Result{}, errors.New("unsupported OpenFlux carrier")
		}
		transports = append(transports, v)
	}
	if p.Name == "" {
		p.Name = "OpenFlux"
	}
	return Result{Outbounds: []map[string]any{{"type": "openflux", "tag": p.Name, "secret": p.Secret, "context": p.Context, "codec": p.Codec, "negotiate": p.Negotiate, "transports": transports}}}, nil
}
func fptnLink(link string) (Result, error) {
	var p struct {
		Version  int          `json:"version"`
		Name     string       `json:"service_name"`
		Username string       `json:"username"`
		Password string       `json:"password"`
		Servers  []fptnServer `json:"servers"`
		Censored []fptnServer `json:"censored_zone_servers"`
	}
	if e := shareJSON(strings.TrimPrefix(link, "fptn:"), false, &p); e != nil {
		return Result{}, e
	}
	if p.Version != 1 || p.Username == "" || p.Password == "" {
		return Result{}, errors.New("invalid FPTN version or credentials")
	}
	servers := append(p.Servers, p.Censored...)
	if len(servers) == 0 || len(servers) > 64 {
		return Result{}, errors.New("invalid FPTN server count")
	}
	r := Result{}
	seen := map[string]bool{}
	for _, s := range servers {
		pin, e := hex.DecodeString(s.Fingerprint)
		if !validHostPort(s.Host, s.Port) || e != nil || len(pin) != 16 {
			return Result{}, errors.New("invalid FPTN endpoint or certificate pin")
		}
		id := net.JoinHostPort(s.Host, strconv.Itoa(s.Port)) + strings.ToLower(s.Fingerprint)
		if seen[id] {
			continue
		}
		seen[id] = true
		name := s.Name
		if name == "" {
			name = p.Name
		}
		if name == "" {
			name = "FPTN"
		}
		r.Outbounds = append(r.Outbounds, map[string]any{"type": "fptn", "tag": name, "server": s.Host, "server_port": s.Port, "username": p.Username, "password": p.Password, "md5_fingerprint": strings.ToLower(s.Fingerprint), "bypass": "obfuscation", "mtu": 1500})
	}
	return r, nil
}

type fptnServer struct {
	Name        string `json:"name"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Fingerprint string `json:"md5_fingerprint"`
}

// The panel does not export PingTunnel share links; this URI is the client's
// interchange format. ICMP has no transport port.
func pingTunnelLink(link string) (Result, error) {
	u, e := url.Parse(link)
	if e != nil || u.Hostname() == "" || u.Port() != "" || u.Path != "" && u.Path != "/" {
		return Result{}, errors.New("invalid PingTunnel URI")
	}
	host := u.Hostname()
	if !validHostPort(host, 1) {
		return Result{}, errors.New("invalid PingTunnel host")
	}
	key := u.Query().Get("key")
	if key == "" && u.User != nil {
		key = u.User.Username()
	}
	if key == "" {
		key = "0"
	}
	n, e := strconv.ParseInt(key, 10, 32)
	if e != nil || n < 0 {
		return Result{}, errors.New("invalid PingTunnel key")
	}
	name := u.Fragment
	if name == "" {
		name = "PingTunnel"
	}
	out := map[string]any{"type": "pingtunnel", "tag": name, "server": host, "key": n}
	enc := u.Query().Get("encrypt")
	if enc != "" {
		switch enc {
		case "none", "aes128", "aes256", "chacha20", "chacha20-poly1305":
		default:
			return Result{}, errors.New("unsupported PingTunnel encryption")
		}
		out["encryption"] = enc
		out["encryption_key"] = u.Query().Get("encrypt_key")
		if enc != "none" && out["encryption_key"] == "" {
			return Result{}, errors.New("PingTunnel encryption key is required")
		}
	}
	return Result{Outbounds: []map[string]any{out}}, nil
}
