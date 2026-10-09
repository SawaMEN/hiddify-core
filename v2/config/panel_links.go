package config

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/hiddify/hiddify-core/v2/config/paneluri"
	"github.com/hiddify/ray2sing/ray2sing"
	"github.com/sagernet/sing-box/option"
)

// Unlike ray2sing's permissive batch parser this path never drops a failed
// entry, and never writes a credential-bearing URI to stderr.
func parsePanelLinks(ctx context.Context, content string) (options *option.Options, handled bool, err error) {
	text := strings.TrimSpace(content)
	if !strings.Contains(text, "://") {
		if decoded, e := paneluri.DecodeBase64(strings.Join(strings.Fields(text), "")); e == nil {
			text = strings.TrimSpace(string(decoded))
		}
	}
	first := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "//") {
			first = line
			break
		}
	}
	if !strings.Contains(first, "://") && !strings.HasPrefix(first, "fptn:") {
		return nil, false, nil
	}
	defer func() {
		if recover() != nil {
			options = nil
			handled = true
			err = fmt.Errorf("invalid proxy share configuration")
		}
	}()
	options = &option.Options{}
	count := 0
	for number, line := range strings.Split(text, "\n") {
		if err := ctx.Err(); err != nil {
			return nil, true, err
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		chain := strings.Split(line, " -> ")
		detour := ""
		for i := len(chain) - 1; i >= 0; i-- {
			link := strings.TrimSpace(chain[i])
			scheme, _, _ := strings.Cut(link, ":")
			scheme = strings.ToLower(scheme)
			if len(scheme) > 32 || strings.IndexFunc(scheme, func(r rune) bool {
				return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '+' || r == '-' || r == '.')
			}) >= 0 {
				return nil, true, fmt.Errorf("share line %d: invalid proxy scheme", number+1)
			}
			var parsed *option.Options
			if paneluri.Handles(scheme) {
				result, e := paneluri.Parse(link)
				if e != nil {
					return nil, true, fmt.Errorf("share line %d (%s): %w", number+1, scheme, e)
				}
				if len(chain) > 1 && len(result.Outbounds)+len(result.Endpoints) != 1 {
					return nil, true, fmt.Errorf("share line %d: multi-server profiles cannot form a proxy chain", number+1)
				}
				data, e := json.Marshal(result)
				if e != nil {
					return nil, true, fmt.Errorf("cannot encode proxy configuration")
				}
				parsed = &option.Options{}
				if parsed.UnmarshalJSONContext(ctx, data) != nil {
					return nil, true, fmt.Errorf("share line %d (%s): unsupported or invalid native options", number+1, scheme)
				}
			} else {
				var e error
				parsed, e = parseKnownLink(link, scheme)
				if e != nil {
					return nil, true, fmt.Errorf("share line %d (%s): invalid or unsupported proxy configuration", number+1, scheme)
				}
			}

			mask, maskErr := parseFinalMask(link)
			if maskErr != nil {
				return nil, true, fmt.Errorf("share line %d: invalid final mask", number+1)
			}
			if mask != nil {
				apply := func(value any) {
					if wrapper, ok := value.(option.DialerOptionsWrapper); ok {
						d := wrapper.TakeDialerOptions()
						d.FinalMask = mask
						wrapper.ReplaceDialerOptions(d)
					}
				}
				for _, out := range parsed.Outbounds {
					apply(out.Options)
				}
				for _, end := range parsed.Endpoints {
					apply(end.Options)
				}
			}
			for _, base := range parsed.Outbounds {
				if base.Tag == "" {
					base.Tag = base.Type
				}
				base.Tag += fmt.Sprintf(" § %d", count)
				count++
				if dialer, ok := base.Options.(option.DialerOptionsWrapper); ok && detour != "" {
					d := dialer.TakeDialerOptions()
					d.Detour = detour
					dialer.ReplaceDialerOptions(d)
				}
				detour = base.Tag
				options.Outbounds = append(options.Outbounds, base)
			}
			for _, base := range parsed.Endpoints {
				if base.Tag == "" {
					base.Tag = base.Type
				}
				base.Tag += fmt.Sprintf(" § %d", count)
				count++
				if dialer, ok := base.Options.(option.DialerOptionsWrapper); ok && detour != "" {
					d := dialer.TakeDialerOptions()
					d.Detour = detour
					dialer.ReplaceDialerOptions(d)
				}
				detour = base.Tag
				options.Endpoints = append(options.Endpoints, base)
			}
		}
	}
	if count == 0 {
		return nil, true, fmt.Errorf("no proxy share configurations found")
	}
	return options, true, nil
}

var panelStandardParsers = map[string]ray2sing.ParserFunc{
	"vmess": ray2sing.VmessSingbox, "svmess": ray2sing.VmessSingbox,
	"vless": ray2sing.VlessSingbox, "svless": ray2sing.VlessSingbox,
	"trojan": ray2sing.TrojanSingbox, "strojan": ray2sing.TrojanSingbox,
	"ss": ray2sing.ShadowsocksSingbox, "tuic": ray2sing.TuicSingbox,
	"hysteria": ray2sing.HysteriaSingbox, "hy": ray2sing.HysteriaSingbox,
	"hysteria2": ray2sing.Hysteria2Singbox, "hy2": ray2sing.Hysteria2Singbox,
	"ssh": ray2sing.SSHSingbox, "naive": ray2sing.NaiveSingbox,
	"naive+https": ray2sing.NaiveSingbox, "naive+quic": ray2sing.NaiveSingbox,
	"anytls": ray2sing.AnyTLSSingbox, "ssconf": ray2sing.BeepassSingbox,
	"direct": ray2sing.DirectSingbox, "socks": ray2sing.SocksSingbox,
	"socks5": ray2sing.SocksSingbox, "phttp": ray2sing.HttpSingbox,
	"phttps": ray2sing.HttpsSingbox, "http": ray2sing.HttpSingbox, "https": ray2sing.HttpsSingbox,
	"xvmess": ray2sing.VmessXray, "xvless": ray2sing.VlessXray,
	"xtrojan": ray2sing.TrojanXray, "xdirect": ray2sing.DirectXray,
	"psiphon": ray2sing.PsiphonSingbox, "dnstt": ray2sing.DnsttSingbox,
}

func parseKnownLink(link, scheme string) (*option.Options, error) {
	if parser, ok := panelStandardParsers[scheme]; ok {
		out, err := parser(link)
		if err != nil || out == nil {
			return nil, fmt.Errorf("invalid proxy")
		}
		if vless, ok := out.Options.(*option.VLESSOutboundOptions); ok {
			u, err := url.Parse(link)
			if err != nil {
				return nil, fmt.Errorf("invalid VLESS URL")
			}
			// ray2sing predates VLESS ML-KEM encryption and otherwise discards it.
			vless.Encryption = u.Query().Get("encryption")
		}
		if naive, ok := out.Options.(*option.NaiveOutboundOptions); ok {
			u, err := url.Parse(link)
			if err != nil {
				return nil, fmt.Errorf("invalid Naive URL")
			}
			q := u.Query()
			naive.QUIC = scheme == "naive+quic" || q.Get("quic") == "1" || q.Get("quic") == "true"
			naive.QUICCongestionControl = q.Get("quic_congestion_control")
		}
		return &option.Options{Outbounds: []option.Outbound{*out}}, nil
	}
	var parser ray2sing.EndpointParserFunc
	switch scheme {
	case "wg", "wireguard", "awg":
		parser = ray2sing.AWGSingbox
	case "warp":
		parser = ray2sing.WarpSingbox
	default:
		return nil, fmt.Errorf("unsupported protocol")
	}
	endpoint, err := parser(link)
	if err != nil || endpoint == nil {
		return nil, fmt.Errorf("invalid endpoint")
	}
	return &option.Options{Endpoints: []option.Endpoint{*endpoint}}, nil
}
