package paneluri

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/url"
	"unicode/utf8"
)

// TLS/QUIC integers are big endian, with their size in the first two bits.
func varint(data []byte) (uint64, int, error) {
	if len(data) == 0 {
		return 0, 0, errors.New("truncated TrustTunnel integer")
	}
	n := 1 << (data[0] >> 6)
	if len(data) < n {
		return 0, 0, errors.New("truncated TrustTunnel integer")
	}
	value := uint64(data[0] & 63)
	for _, b := range data[1:n] {
		value = value<<8 | uint64(b)
	}
	return value, n, nil
}

func trustTunnel(link string) (Result, error) {
	const prefix = "tt://?"
	if len(link) < len(prefix) || link[:len(prefix)] != prefix {
		return Result{}, errors.New("invalid TrustTunnel deep link")
	}
	data, err := DecodeBase64(link[len(prefix):])
	if err != nil {
		return Result{}, err
	}
	fields := map[uint64][]byte{}
	addresses := []string{}
	for len(data) > 0 {
		tag, n, err := varint(data)
		if err != nil {
			return Result{}, err
		}
		data = data[n:]
		size, n, err := varint(data)
		if err != nil {
			return Result{}, err
		}
		data = data[n:]
		if size > uint64(len(data)) {
			return Result{}, errors.New("truncated TrustTunnel field")
		}
		value := data[:int(size)]
		data = data[int(size):]
		if tag == 2 {
			if !utf8.Valid(value) {
				return Result{}, errors.New("invalid TrustTunnel address")
			}
			addresses = append(addresses, string(value))
		} else {
			fields[tag] = value
		}
	}
	integer := func(tag uint64, fallback uint64) (uint64, error) {
		value, exists := fields[tag]
		if !exists {
			return fallback, nil
		}
		v, n, err := varint(value)
		if err != nil || n != len(value) {
			return 0, errors.New("invalid TrustTunnel numeric field")
		}
		return v, nil
	}
	version, err := integer(0, 0)
	if err != nil || version > 2 {
		return Result{}, errors.New("unsupported TrustTunnel deep-link version")
	}
	protocol, err := integer(9, 1)
	if err != nil || (protocol != 1 && protocol != 2) {
		return Result{}, errors.New("unsupported TrustTunnel upstream protocol")
	}
	for _, tag := range []uint64{4, 7, 10} {
		if value, exists := fields[tag]; exists && (len(value) != 1 || value[0] > 1) {
			return Result{}, errors.New("invalid TrustTunnel boolean")
		}
	}
	for _, tag := range []uint64{1, 3, 5, 6, 11, 12, 14} {
		if !utf8.Valid(fields[tag]) {
			return Result{}, errors.New("invalid TrustTunnel text field")
		}
	}
	if sub := string(fields[14]); sub != "" {
		u, err := url.Parse(sub)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" {
			return Result{}, errors.New("TrustTunnel subscription URL must use HTTPS")
		}
		// No network fetching inside configuration parsing or VPN startup.
		return Result{}, errors.New("TrustTunnel subscription deep links require a TrustTunnel subscription client; import static endpoint credentials instead")
	}
	if string(fields[10]) == "\x01" || len(fields[11]) > 0 {
		return Result{}, errors.New("TrustTunnel anti-DPI/client-random options are not supported by this core")
	}
	if len(fields[1]) == 0 || len(fields[5]) == 0 || len(fields[6]) == 0 || len(addresses) == 0 {
		return Result{}, errors.New("TrustTunnel required endpoint fields are missing")
	}
	tls := map[string]any{"enabled": true, "server_name": string(fields[1])}
	if len(fields[3]) > 0 {
		tls["server_name"] = string(fields[3])
	}
	if string(fields[7]) == "\x01" {
		tls["insecure"] = true
	}
	if len(fields[8]) > 0 {
		certificates, err := x509.ParseCertificates(fields[8])
		if err != nil {
			return Result{}, errors.New("invalid TrustTunnel DER certificate chain")
		}
		chain := ""
		for _, cert := range certificates {
			chain += string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}))
		}
		tls["certificate"] = []string{chain}
	}
	name := string(fields[12])
	if name == "" {
		name = "TrustTunnel"
	}
	result := Result{}
	for _, value := range addresses {
		host, port, err := address(value)
		if err != nil {
			return Result{}, err
		}
		out := map[string]any{"type": "trusttunnel", "tag": name, "server": host, "server_port": port,
			"username": string(fields[5]), "password": string(fields[6]), "quic": protocol == 2, "tls": tls}
		result.Outbounds = append(result.Outbounds, out)
	}
	return result, nil
}

func trustTunnelURL(link string) (Result, error) {
	u, out, err := serverURL(link)
	if err != nil {
		return Result{}, err
	}
	if u.User == nil {
		return Result{}, errors.New("TrustTunnel credentials are missing")
	}
	password, _ := u.User.Password()
	if u.User.Username() == "" || password == "" {
		return Result{}, errors.New("TrustTunnel credentials are missing")
	}
	q := u.Query()
	sni := q.Get("sni")
	if sni == "" {
		sni = u.Hostname()
	}
	tls := map[string]any{"enabled": true, "server_name": sni}
	if q.Get("insecure") == "1" || q.Get("insecure") == "true" {
		tls["insecure"] = true
	}
	out["type"], out["username"], out["password"], out["tls"] = "trusttunnel", u.User.Username(), password, tls
	out["quic"] = q.Get("quic") == "1" || q.Get("quic") == "true"
	return Result{Outbounds: []map[string]any{out}}, nil
}
