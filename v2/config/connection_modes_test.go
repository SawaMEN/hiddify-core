package config

import (
	"strings"
	"testing"
	"time"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/experimental/libbox"
	"github.com/sagernet/sing-box/option"
)

func modernTestOutbound(tag string) option.Outbound {
	return option.Outbound{Type: C.TypeHysteria2, Tag: tag, Options: &option.Hysteria2OutboundOptions{
		ServerOptions: option.ServerOptions{Server: "example.com", ServerPort: 443},
		Password:      "test",
		UpMbps:        100, DownMbps: 100,
		OutboundTLSOptionsContainer: option.OutboundTLSOptionsContainer{TLS: &option.OutboundTLSOptions{Enabled: true}},
	}}
}

func TestModernModeExcludesLegacyInsecureHiddenAndEndpointFallbacks(t *testing.T) {
	good := modernTestOutbound("verified")
	insecure := modernTestOutbound("insecure")
	insecure.Options.(*option.Hysteria2OutboundOptions).TLS.Insecure = true
	chained := modernTestOutbound("chained")
	chained.Options.(*option.Hysteria2OutboundOptions).Detour = "legacy"
	tuic := option.Outbound{Type: C.TypeTUIC, Tag: "tuic", Options: &option.TUICOutboundOptions{
		ServerOptions: option.ServerOptions{Server: "example.com", ServerPort: 443},
		UUID:          "00000000-0000-0000-0000-000000000001", Password: "test", ZeroRTTHandshake: true,
		OutboundTLSOptionsContainer: option.OutboundTLSOptionsContainer{TLS: &option.OutboundTLSOptions{Enabled: true}},
	}}
	profile := &option.Options{Outbounds: []option.Outbound{good, insecure, chained, tuic,
		{Type: C.TypeVLESS, Tag: "legacy", Options: &option.VLESSOutboundOptions{}},
		{Type: C.TypeVMess, Tag: "hidden §hide§", Options: &option.VMessOutboundOptions{}},
		{Type: C.TypeSOCKS, Tag: "fallback", Options: &option.SOCKSOutboundOptions{}},
	}, Endpoints: []option.Endpoint{{Type: C.TypeWireGuard, Tag: "endpoint-fallback"}}}
	h := DefaultHiddifyOptions()
	h.ModernProtocolsOnly = true
	h.ModernAllowUDP = true
	built, err := BuildConfig(libbox.BaseContext(nil), h, &ReadOptions{Options: profile})
	if err != nil {
		t.Fatal(err)
	}
	if len(built.Endpoints) != 0 {
		t.Fatal("endpoint bypassed protocol allow-list")
	}
	for _, out := range built.Outbounds {
		switch out.Type {
		case C.TypeHysteria2, C.TypeTUIC:
			if !modernOutboundAllowed(out, true) {
				t.Fatalf("insecure modern outbound: %s", out.Tag)
			}
			if typed, ok := out.Options.(*option.TUICOutboundOptions); ok && typed.ZeroRTTHandshake {
				t.Fatal("0-RTT remains enabled")
			}
		case C.TypeSelector:
			for _, tag := range out.Options.(*option.SelectorOutboundOptions).Outbounds {
				if tag != "verified" && tag != "tuic" && tag != OutboundURLTestTag && tag != OutboundRoundRobinTag {
					t.Fatalf("selector fallback: %s", tag)
				}
			}
		case C.TypeBalancer:
			for _, tag := range out.Options.(*option.BalancerOutboundOptions).Outbounds {
				if tag != "verified" && tag != "tuic" {
					t.Fatalf("balancer fallback: %s", tag)
				}
			}
		case C.TypeDirect:
			if !strings.Contains(out.Tag, "§hide§") {
				t.Fatalf("unexpected direct candidate: %s", out.Tag)
			}
		default:
			t.Fatalf("legacy outbound remains available: %s", out.Type)
		}
	}
	if !tuic.Options.(*option.TUICOutboundOptions).ZeroRTTHandshake {
		t.Fatal("input profile was modified")
	}
}

func TestModernModeFailsClosedWhenNoCompatibleServersOrWarpChain(t *testing.T) {
	h := DefaultHiddifyOptions()
	h.ModernProtocolsOnly = true
	h.ModernAllowUDP = true
	for _, profile := range []*option.Options{
		{Outbounds: []option.Outbound{{Type: C.TypeVLESS, Tag: "legacy", Options: &option.VLESSOutboundOptions{}}}},
		{Outbounds: []option.Outbound{{Type: C.TypeTUIC, Tag: "no-tls", Options: &option.TUICOutboundOptions{}}}},
	} {
		_, err := BuildConfig(libbox.BaseContext(nil), h, &ReadOptions{Options: profile})
		if err == nil || !strings.Contains(err.Error(), "modern protocols only:") {
			t.Fatalf("must fail closed: %v", err)
		}
	}
	h.Warp.EnableWarp = true
	_, err := BuildConfig(libbox.BaseContext(nil), h, &ReadOptions{Options: &option.Options{Outbounds: []option.Outbound{modernTestOutbound("ok")}}})
	if err == nil || !strings.Contains(err.Error(), "disable WARP") {
		t.Fatalf("chain bypass: %v", err)
	}
}

func TestAdaptiveModePreservesProfilesAndStreamsAndAdaptsQuic(t *testing.T) {
	first, second := modernTestOutbound("a"), modernTestOutbound("b")
	h := DefaultHiddifyOptions()
	h.AdaptiveNetwork = true
	h.MTU = 9000
	built, err := BuildConfig(libbox.BaseContext(nil), h, &ReadOptions{Options: &option.Options{Outbounds: []option.Outbound{first, second}}})
	if err != nil {
		t.Fatal(err)
	}
	if h.MTU != 9000 || h.URLTestInterval != 600 {
		t.Fatal("stored settings were mutated")
	}
	if built.Experimental.Monitoring.Interval.Build() != time.Minute {
		t.Fatal("adaptive monitoring not applied")
	}
	for _, out := range built.Outbounds {
		switch typed := out.Options.(type) {
		case *option.Hysteria2OutboundOptions:
			if typed.UpMbps != 0 || typed.DownMbps != 0 || typed.BBRProfile != "conservative" {
				t.Fatal("fixed bandwidth remains")
			}
			if typed.ConnectTimeout.Build() < 30*time.Second || typed.TLS.HandshakeTimeout.Build() < 30*time.Second {
				t.Fatal("short timeout remains")
			}
		case *option.BalancerOutboundOptions:
			if typed.Strategy != "sticky-sessions" || typed.MaxRetry != 3 || typed.TTL.Build() != 3*time.Minute || typed.InterruptExistConnections {
				t.Fatal("unstable or interrupting balancer")
			}
		case *option.SelectorOutboundOptions:
			if typed.InterruptExistConnections {
				t.Fatal("selector interrupts streams")
			}
		}
	}
	original := first.Options.(*option.Hysteria2OutboundOptions)
	if original.UpMbps != 100 || original.TLS.HandshakeTimeout != 0 {
		t.Fatal("stored outbound mutated")
	}
}

func TestAdaptiveModeAlsoExtendsLegacyDialBudgetWithoutChangingTransport(t *testing.T) {
	original := &option.VLESSOutboundOptions{ServerOptions: option.ServerOptions{Server: "example.com", ServerPort: 443}}
	out := applyAdaptiveOutbound(option.Outbound{Type: C.TypeVLESS, Options: original}, true, false)
	changed := out.Options.(*option.VLESSOutboundOptions)
	if changed.ConnectTimeout.Build() != 30*time.Second || original.ConnectTimeout != 0 || out.Type != C.TypeVLESS {
		t.Fatal("legacy adaptation mutated or changed the transport")
	}
}

func TestMaskedPolicySeparatesTcpAndQuicAndPreservesTls(t *testing.T) {
	tls := &option.OutboundTLSOptions{Enabled: true}
	any := option.Outbound{Type: C.TypeAnyTLS, Tag: "tcp", Options: &option.AnyTLSOutboundOptions{OutboundTLSOptionsContainer: option.OutboundTLSOptionsContainer{TLS: tls}}}
	naive := option.Outbound{Type: C.TypeNaive, Tag: "web", Options: &option.NaiveOutboundOptions{InsecureConcurrency: 8, OutboundTLSOptionsContainer: option.OutboundTLSOptionsContainer{TLS: tls}}}
	for _, out := range []option.Outbound{any, naive} {
		if !modernOutboundAllowed(out, false) {
			t.Fatal("TCP masked node rejected")
		}
	}
	if modernOutboundAllowed(modernTestOutbound("quic"), false) {
		t.Fatal("UDP permitted without opt-in")
	}
	quicNaive := *naive.Options.(*option.NaiveOutboundOptions)
	quicNaive.QUIC = true
	if modernOutboundAllowed(option.Outbound{Type: C.TypeNaive, Options: &quicNaive}, false) {
		t.Fatal("Naive HTTP/3 bypassed UDP policy")
	}
	if !modernOutboundAllowed(option.Outbound{Type: C.TypeNaive, Options: &quicNaive}, true) {
		t.Fatal("explicit HTTP/3 rejected")
	}
	changedAny := applyAdaptiveOutbound(any, false, true).Options.(*option.AnyTLSOutboundOptions)
	changedNaive := applyAdaptiveOutbound(naive, false, true).Options.(*option.NaiveOutboundOptions)
	if changedAny.TLS.UTLS == nil || !changedAny.TLS.UTLS.Enabled || changedNaive.InsecureConcurrency != 0 || changedNaive.TLS.UTLS != nil {
		t.Fatal("TLS imitation or Chromium policy missing")
	}
	if tls.UTLS != nil || naive.Options.(*option.NaiveOutboundOptions).InsecureConcurrency != 8 {
		t.Fatal("input mutated")
	}
	for _, invalid := range []*option.OutboundTLSOptions{{Enabled: true, Insecure: true}, {Enabled: true, DisableSNI: true}, {Enabled: true, MaxVersion: "1.1"}, {Enabled: true, Reality: &option.OutboundRealityOptions{Enabled: true}}} {
		if modernOutboundAllowed(option.Outbound{Type: C.TypeAnyTLS, Options: &option.AnyTLSOutboundOptions{OutboundTLSOptionsContainer: option.OutboundTLSOptionsContainer{TLS: invalid}}}, true) {
			t.Fatal("unsafe TLS admitted")
		}
	}
	h := DefaultHiddifyOptions()
	h.ModernProtocolsOnly = true
	built, err := BuildConfig(libbox.BaseContext(nil), h, &ReadOptions{Options: &option.Options{Outbounds: []option.Outbound{any, naive, modernTestOutbound("udp")}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, out := range built.Outbounds {
		if out.Type == C.TypeHysteria2 || out.Type == C.TypeTUIC {
			t.Fatal("UDP entered TCP-only configuration")
		}
	}
}
