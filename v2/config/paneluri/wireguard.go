package paneluri

import (
	"errors"
	"net/netip"
	"strconv"
	"strings"
)

func wireguard(text string) (Result, error) {
	section := ""
	iface := map[string]string{}
	peers := []map[string]string{}
	name := "AmneziaWG"
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# Name = ") {
			name = strings.TrimPrefix(line, "# Name = ")
		}
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.Trim(line, "[]"))
			if section == "peer" {
				peers = append(peers, map[string]string{})
			}
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return Result{}, errors.New("invalid WireGuard configuration line")
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		switch section {
		case "interface":
			iface[key] = value
		case "peer":
			peers[len(peers)-1][key] = value
		default:
			return Result{}, errors.New("invalid WireGuard configuration section")
		}
	}
	validKey := func(key string) bool { data, err := DecodeBase64(key); return err == nil && len(data) == 32 }
	if !validKey(iface["PrivateKey"]) {
		return Result{}, errors.New("invalid WireGuard private key")
	}
	addresses, err := prefixes(iface["Address"])
	if err != nil || len(addresses) == 0 {
		return Result{}, errors.New("invalid WireGuard interface address")
	}
	// Android's VPN service already owns the OS TUN; use the userspace stack.
	endpoint := map[string]any{"type": "awg", "tag": name, "private_key": iface["PrivateKey"], "address": addresses, "useIntegratedTun": false}
	if value := iface["MTU"]; value != "" {
		mtu, err := strconv.Atoi(value)
		if err != nil || mtu < 576 || mtu > 65535 {
			return Result{}, errors.New("invalid WireGuard MTU")
		}
		endpoint["mtu"] = mtu
	}
	if value := iface["ListenPort"]; value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 || n > 65535 {
			return Result{}, errors.New("invalid WireGuard listen port")
		}
		endpoint["listen_port"] = n
	}
	awg := map[string]any{}
	if value := iface["HeaderProtectionKey"]; value != "" {
		if !validKey(value) {
			return Result{}, errors.New("invalid AWG header protection key")
		}
		awg["header_protection_key"] = value
	}
	for _, field := range []struct{ source, target string }{
		{"ContentPaddingAddition", "content_padding_addition"}, {"RekeyAfterTime", "rekey_after_time"}, {"RekeyTimeout", "rekey_timeout"},
		{"RejectAfterTime", "reject_after_time"}, {"KeepaliveTimeout", "keepalive_timeout"}, {"MaxHandshakeAttempts", "max_handshake_attempts"},
	} {
		if value := iface[field.source]; value != "" {
			parts := strings.Split(value, "-")
			if len(parts) > 2 {
				return Result{}, errors.New("invalid AWG option range")
			}
			previous := uint64(0)
			for _, part := range parts {
				n, err := strconv.ParseUint(part, 10, 32)
				if err != nil || n < previous {
					return Result{}, errors.New("invalid AWG option range")
				}
				previous = n
			}
			awg[field.target] = value
		}
	}
	for _, field := range []struct{ source, target string }{{"RandomTrailers", "random_trailers"}, {"DisableCookies", "disable_cookies"}} {
		if value := strings.ToLower(iface[field.source]); value != "" {
			switch value {
			case "on", "true", "1":
				awg[field.target] = true
			case "off", "false", "0":
				awg[field.target] = false
			default:
				return Result{}, errors.New("invalid AWG boolean option")
			}
		}
	}
	for _, key := range []string{"Jc", "Jmin", "Jmax", "S1", "S2", "S3", "S4"} {
		if value := iface[key]; value != "" {
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 {
				return Result{}, errors.New("invalid AmneziaWG numeric option")
			}
			awg[strings.ToLower(key)] = n
		}
	}
	for _, key := range []string{"H1", "H2", "H3", "H4", "I1", "I2", "I3", "I4", "I5"} {
		if value := iface[key]; value != "" {
			awg[strings.ToLower(key)] = value
		}
	}
	endpoint["awg"] = awg
	entries := []map[string]any{}
	for _, peer := range peers {
		if !validKey(peer["PublicKey"]) {
			return Result{}, errors.New("invalid WireGuard peer public key")
		}
		host, port, err := address(peer["Endpoint"])
		if err != nil {
			return Result{}, err
		}
		allowed, err := prefixes(peer["AllowedIPs"])
		if err != nil {
			return Result{}, err
		}
		if len(allowed) == 0 {
			return Result{}, errors.New("WireGuard peer allowed IPs are missing")
		}
		entry := map[string]any{"address": host, "port": port, "public_key": peer["PublicKey"], "allowed_ips": allowed}
		if value := peer["PresharedKey"]; value != "" {
			if !validKey(value) {
				return Result{}, errors.New("invalid WireGuard preshared key")
			}
			entry["preshared_key"] = value
		}
		if value := peer["PersistentKeepalive"]; value != "" {
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 || n > 65535 {
				return Result{}, errors.New("invalid WireGuard keepalive")
			}
			entry["persistent_keepalive_interval"] = n
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return Result{}, errors.New("WireGuard peers are missing")
	}
	endpoint["peers"] = entries
	return Result{Endpoints: []map[string]any{endpoint}}, nil
}

func prefixes(value string) ([]string, error) {
	result := []string{}
	if value == "" {
		return result, nil
	}
	for _, item := range strings.Split(value, ",") {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(item))
		if err != nil {
			return nil, errors.New("invalid WireGuard IP prefix")
		}
		result = append(result, prefix.String())
	}
	return result, nil
}
