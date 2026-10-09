// Package paneluri implements the native share formats exported by SawaMEN/3x-ui.
// Decoders never include the input (which contains credentials) in errors.
package paneluri

import (
	"encoding/base64"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

const MaxPayload = 8 * 1024 * 1024

type Result struct {
	Outbounds []map[string]any `json:"outbounds,omitempty"`
	Endpoints []map[string]any `json:"endpoints,omitempty"`
}

func DecodeBase64(text string) ([]byte, error) {
	if len(text) > MaxPayload {
		return nil, errors.New("share payload exceeds 8 MiB")
	}
	for _, encoding := range []*base64.Encoding{base64.RawURLEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.StdEncoding} {
		if data, err := encoding.DecodeString(text); err == nil {
			return data, nil
		}
	}
	return nil, errors.New("invalid base64 share payload")
}

func Handles(scheme string) bool {
	switch scheme {
	case "snell", "mieru", "mierus", "vpn", "tt", "trusttunnel", "sudoku", "fptn", "openflux", "pingtunnel":
		return true
	}
	return false
}

func Parse(link string) (Result, error) {
	if len(link) > MaxPayload {
		return Result{}, errors.New("share payload exceeds 8 MiB")
	}
	scheme, _, _ := strings.Cut(link, ":")
	switch strings.ToLower(scheme) {
	case "sudoku":
		return sudokuLink(link)
	case "fptn":
		return fptnLink(link)
	case "openflux":
		return openFluxLink(link)
	case "pingtunnel":
		return pingTunnelLink(link)
	case "snell":
		return snell(link)
	case "mieru", "mierus":
		return mieru(link)
	case "vpn":
		data, err := DecodeBase64(strings.TrimPrefix(link, "vpn://"))
		if err != nil {
			return Result{}, err
		}
		return wireguard(string(data))
	case "tt":
		return trustTunnel(link)
	case "trusttunnel":
		return trustTunnelURL(link)
	}
	return Result{}, errors.New("unsupported share scheme")
}

func address(value string) (string, int, error) {
	host, portText, err := net.SplitHostPort(value)
	if err != nil {
		return "", 0, errors.New("invalid server address")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || host == "" || port < 1 || port > 65535 {
		return "", 0, errors.New("invalid server address or port")
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		return "", 0, errors.New("unspecified server address")
	}
	return host, port, nil
}

func serverURL(link string) (*url.URL, map[string]any, error) {
	u, err := url.Parse(link)
	if err != nil {
		return nil, nil, errors.New("invalid share URL")
	}
	host, port, err := address(u.Host)
	if err != nil {
		return nil, nil, err
	}
	name := u.Fragment
	if name == "" {
		name = u.Scheme
	}
	return u, map[string]any{"server": host, "server_port": port, "tag": name}, nil
}

func snell(link string) (Result, error) {
	u, out, err := serverURL(link)
	if err != nil {
		return Result{}, err
	}
	q := u.Query()
	psk := q.Get("psk")
	if psk == "" && u.User != nil {
		psk = u.User.Username()
	}
	if psk == "" {
		return Result{}, errors.New("Snell PSK is missing")
	}
	version := 4
	if value := q.Get("version"); value != "" {
		version, err = strconv.Atoi(value)
		if err != nil {
			return Result{}, errors.New("invalid Snell version")
		}
	}
	if version != 4 && version != 6 {
		return Result{}, errors.New("supported Snell client versions are 4 and 6")
	}
	out["type"], out["psk"], out["version"] = "snell", psk, version
	if value := q.Get("userkey"); value != "" {
		out["userkey"] = value
	}
	if version == 6 {
		mode := q.Get("mode")
		if mode != "" && mode != "default" && mode != "unshaped" && mode != "unsafe-raw" {
			return Result{}, errors.New("invalid Snell v6 mode")
		}
		if mode != "" {
			out["mode"] = mode
		}
	} else {
		mode := q.Get("obfs")
		if mode != "" && mode != "none" && mode != "http" && mode != "tls" {
			return Result{}, errors.New("invalid Snell obfuscation")
		}
		if mode != "" {
			out["obfs_mode"] = mode
		}
		if host := q.Get("obfs-host"); host != "" {
			out["obfs_host"] = host
		}
	}
	return Result{Outbounds: []map[string]any{out}}, nil
}
