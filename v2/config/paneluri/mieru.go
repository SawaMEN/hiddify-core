package paneluri

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"

	pb "github.com/enfein/mieru/v3/pkg/appctl/appctlpb"
	"google.golang.org/protobuf/proto"
)

func mieru(link string) (Result, error) {
	if strings.HasPrefix(link, "mierus://") || strings.Contains(link, "@") {
		return mieruSimple(link)
	}
	data, err := DecodeBase64(strings.TrimPrefix(link, "mieru://"))
	if err != nil {
		return Result{}, err
	}
	var cfg pb.ClientConfig
	if proto.Unmarshal(data, &cfg) != nil {
		return Result{}, errors.New("invalid Mieru protobuf configuration")
	}
	var profile *pb.ClientProfile
	for _, p := range cfg.Profiles {
		if p.GetProfileName() == cfg.GetActiveProfile() {
			profile = p
			break
		}
	}
	if profile == nil || profile.User == nil || profile.User.GetName() == "" || profile.User.GetPassword() == "" {
		return Result{}, errors.New("Mieru active profile or credentials are missing")
	}
	result := Result{}
	for i, server := range profile.Servers {
		host := server.GetDomainName()
		if host == "" {
			host = server.GetIpAddress()
		}
		if host == "" {
			return Result{}, errors.New("Mieru server is missing")
		}
		bindings := make([]map[string]any, 0, len(server.PortBindings))
		for _, binding := range server.PortBindings {
			entry, err := mieruBinding(binding.GetProtocol().String(), binding.GetPort(), binding.GetPortRange())
			if err != nil {
				return Result{}, err
			}
			bindings = append(bindings, entry)
		}
		if len(bindings) == 0 {
			return Result{}, errors.New("Mieru port bindings are missing")
		}
		out := map[string]any{"type": "mieru", "tag": profile.GetProfileName() + "-" + strconv.Itoa(i+1), "server": host,
			"username": profile.User.GetName(), "password": profile.User.GetPassword(), "portBindings": bindings,
			"handshake_mode": profile.GetHandshakeMode().String(), "multiplexing": profile.GetMultiplexing().GetLevel().String()}
		if mtu := profile.GetMtu(); mtu != 0 {
			if mtu < 1280 || mtu > 1500 {
				return Result{}, errors.New("invalid Mieru MTU")
			}
			out["mtu"] = mtu
		}
		result.Outbounds = append(result.Outbounds, out)
	}
	if len(result.Outbounds) == 0 {
		return Result{}, errors.New("Mieru servers are missing")
	}
	return result, nil
}

func mieruSimple(link string) (Result, error) {
	u, err := url.Parse(link)
	if err != nil || u.Hostname() == "" || u.User == nil {
		return Result{}, errors.New("invalid Mieru profile URL")
	}
	password, _ := u.User.Password()
	if u.User.Username() == "" || password == "" {
		return Result{}, errors.New("Mieru credentials are missing")
	}
	q := u.Query()
	ports, protocols := q["port"], q["protocol"]
	if len(ports) == 1 {
		ports = strings.Split(ports[0], ",")
	}
	if len(protocols) == 1 {
		protocols = strings.Split(protocols[0], ",")
	}
	if port := u.Port(); port != "" && len(protocols) == len(ports)+1 {
		ports = append([]string{port}, ports...)
	}
	if len(ports) == 0 || len(ports) != len(protocols) {
		return Result{}, errors.New("Mieru ports and protocols do not match")
	}
	bindings := []map[string]any{}
	for i, port := range ports {
		number, portRange := int32(0), ""
		if strings.Contains(port, "-") {
			portRange = port
		} else {
			n, err := strconv.ParseInt(port, 10, 32)
			if err != nil {
				return Result{}, errors.New("invalid Mieru port")
			}
			number = int32(n)
		}
		entry, err := mieruBinding(protocols[i], number, portRange)
		if err != nil {
			return Result{}, err
		}
		bindings = append(bindings, entry)
	}
	name := u.Fragment
	if name == "" {
		name = q.Get("profile")
	}
	if name == "" {
		name = "Mieru"
	}
	handshake := q.Get("handshake-mode")
	if handshake == "" {
		handshake = q.Get("handshake_mode")
	}
	if handshake == "" {
		handshake = "HANDSHAKE_DEFAULT"
	}
	mux := q.Get("multiplexing")
	if mux == "" {
		mux = "MULTIPLEXING_DEFAULT"
	}
	if _, ok := pb.HandshakeMode_value[handshake]; !ok {
		return Result{}, errors.New("invalid Mieru handshake mode")
	}
	if _, ok := pb.MultiplexingLevel_value[mux]; !ok {
		return Result{}, errors.New("invalid Mieru multiplexing")
	}
	out := map[string]any{"type": "mieru", "tag": name, "server": strings.Trim(u.Hostname(), "[]"), "username": u.User.Username(), "password": password,
		"portBindings": bindings, "handshake_mode": handshake, "multiplexing": mux}
	if value := q.Get("mtu"); value != "" {
		mtu, err := strconv.Atoi(value)
		if err != nil || mtu < 1280 || mtu > 1500 {
			return Result{}, errors.New("invalid Mieru MTU")
		}
		out["mtu"] = mtu
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && ip.IsUnspecified() {
		return Result{}, errors.New("unspecified Mieru server")
	}
	return Result{Outbounds: []map[string]any{out}}, nil
}

func mieruBinding(protocol string, port int32, portRange string) (map[string]any, error) {
	if protocol != "TCP" && protocol != "UDP" {
		return nil, errors.New("invalid Mieru transport")
	}
	entry := map[string]any{"protocol": protocol}
	if portRange == "" {
		if port < 1 || port > 65535 {
			return nil, errors.New("invalid Mieru port")
		}
		entry["port"] = port
	} else {
		start, end, ok := strings.Cut(portRange, "-")
		a, e1 := strconv.Atoi(start)
		b, e2 := strconv.Atoi(end)
		if !ok || e1 != nil || e2 != nil || a < 1 || b > 65535 || a > b {
			return nil, errors.New("invalid Mieru port range")
		}
		entry["portRange"] = portRange
	}
	return entry, nil
}
