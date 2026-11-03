package config

import (
	"reflect"
	"time"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"
)

// Device policy is based on transport and TLS, never a subscription label.
// VLESS/VMess and opaque nested transports stay excluded in this conservative mode.
func modernOutboundAllowed(out option.Outbound, allowUDP bool) bool {
	tlsOptions, ok := out.Options.(option.OutboundTLSOptionsWrapper)
	if !ok {
		return false
	}
	tls := tlsOptions.TakeOutboundTLSOptions()
	if tls == nil || !tls.Enabled || tls.Insecure || tls.DisableSNI ||
		tls.MinVersion == "1.0" || tls.MinVersion == "1.1" ||
		tls.MaxVersion == "1.0" || tls.MaxVersion == "1.1" ||
		(tls.Reality != nil && tls.Reality.Enabled) {
		return false
	}
	dialer, ok := out.Options.(option.DialerOptionsWrapper)
	if !ok || dialer.TakeDialerOptions().Detour != "" {
		return false
	}
	switch typed := out.Options.(type) {
	case *option.AnyTLSOutboundOptions:
		return out.Type == C.TypeAnyTLS
	case *option.NaiveOutboundOptions:
		return out.Type == C.TypeNaive && (!typed.QUIC || allowUDP)
	case *option.Hysteria2OutboundOptions:
		return out.Type == C.TypeHysteria2 && allowUDP
	case *option.TUICOutboundOptions:
		return out.Type == C.TypeTUIC && allowUDP
	default:
		return false
	}
}

func adaptiveDialer(dialer option.DialerOptions) option.DialerOptions {
	if dialer.ConnectTimeout.Build() < 30*time.Second {
		dialer.ConnectTimeout = badoption.Duration(30 * time.Second)
	}
	return dialer
}

func adaptiveTLS(tls *option.OutboundTLSOptions, adaptive bool) *option.OutboundTLSOptions {
	if tls == nil {
		return nil
	}
	next := *tls
	if adaptive && next.HandshakeTimeout.Build() < 30*time.Second {
		next.HandshakeTimeout = badoption.Duration(30 * time.Second)
	}
	return &next
}

// Copy typed options before applying runtime policy; stored subscriptions are unchanged.
func applyAdaptiveOutbound(out option.Outbound, adaptive, modern bool) option.Outbound {
	if !adaptive && !modern {
		return out
	}
	switch typed := out.Options.(type) {
	case *option.AnyTLSOutboundOptions:
		next := *typed
		next.TLS = adaptiveTLS(typed.TLS, adaptive)
		if adaptive {
			next.DialerOptions = adaptiveDialer(next.DialerOptions)
		}
		if modern {
			next.TLS.UTLS = &option.OutboundUTLSOptions{Enabled: true, Fingerprint: "chrome"}
		}
		out.Options = &next
	case *option.NaiveOutboundOptions:
		next := *typed
		next.TLS = adaptiveTLS(typed.TLS, adaptive)
		if adaptive {
			next.DialerOptions = adaptiveDialer(next.DialerOptions)
		}
		// Chromium provides its own TLS fingerprint; do not inject uTLS here.
		if modern {
			next.InsecureConcurrency = 0
		}
		out.Options = &next
	case *option.Hysteria2OutboundOptions:
		next := *typed
		next.TLS = adaptiveTLS(typed.TLS, adaptive)
		if adaptive {
			next.DialerOptions = adaptiveDialer(next.DialerOptions)
			// BBR adapts to the actual link instead of a subscription's fixed Mbps.
			next.UpMbps, next.DownMbps = 0, 0
			next.BBRProfile = "conservative"
		}
		if modern {
			next.DisableChromeParrot = false
		}
		out.Options = &next
	case *option.TUICOutboundOptions:
		next := *typed
		next.TLS = adaptiveTLS(typed.TLS, adaptive)
		if adaptive {
			next.DialerOptions = adaptiveDialer(next.DialerOptions)
			next.CongestionControl = "bbr"
		}
		// Avoid replay-sensitive 0-RTT in either opt-in protection mode.
		next.ZeroRTTHandshake = false
		out.Options = &next
	default:
		if adaptive {
			// Other supported protocols keep their transport, with a longer dial budget.
			value := reflect.ValueOf(out.Options)
			if value.Kind() == reflect.Pointer && !value.IsNil() && value.Elem().Kind() == reflect.Struct {
				copied := reflect.New(value.Elem().Type())
				copied.Elem().Set(value.Elem())
				out.Options = copied.Interface()
				if dialer, ok := out.Options.(option.DialerOptionsWrapper); ok {
					dialer.ReplaceDialerOptions(adaptiveDialer(dialer.TakeDialerOptions()))
				}
				if tls, ok := out.Options.(option.OutboundTLSOptionsWrapper); ok {
					tls.ReplaceOutboundTLSOptions(adaptiveTLS(tls.TakeOutboundTLSOptions(), true))
				}
			}
		}
	}
	return out
}
