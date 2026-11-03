package config

import (
	"testing"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
)

// Android's VpnService.Builder omits IPv6 addresses in ipv4_only mode.
// The Go TUN configuration must match, even without root.
func TestTunIPv4OnlyDoesNotConfigureIPv6WithoutRoot(t *testing.T) {
	settings := DefaultHiddifyOptions()
	settings.EnableTun = true
	settings.PrivacyRoot = false
	settings.IPv6Mode = option.DomainStrategy(C.DomainStrategyIPv4Only)
	options := &option.Options{}
	setInbound(options, settings)

	foundTun := false
	for _, inbound := range options.Inbounds {
		if inbound.Type != C.TypeTun {
			continue
		}
		foundTun = true
		tun, ok := inbound.Options.(*option.TunInboundOptions)
		if !ok {
			t.Fatalf("unexpected TUN options type: %T", inbound.Options)
		}
		if len(tun.Address) != 1 || !tun.Address[0].Addr().Is4() {
			t.Fatalf("IPv4-only mode must configure only an IPv4 TUN address, got %v", tun.Address)
		}
	}
	if !foundTun {
		t.Fatal("TUN inbound was not generated")
	}
	for _, inbound := range options.Inbounds {
		if inbound.Tag == InboundMixedTag+"::1" || inbound.Tag == InboundDirectTag+"::1" {
			t.Fatalf("IPv4-only mode unexpectedly created IPv6 listener %s", inbound.Tag)
		}
	}
}

func TestTunDualStackStillConfiguresIPv6WhenSupported(t *testing.T) {
	if !isIPv6Supported() {
		t.Skip("host has no IPv6 stack")
	}
	settings := DefaultHiddifyOptions()
	settings.EnableTun = true
	settings.PrivacyRoot = false
	settings.IPv6Mode = option.DomainStrategy(C.DomainStrategyAsIS)
	options := &option.Options{}
	setInbound(options, settings)
	for _, inbound := range options.Inbounds {
		if inbound.Type != C.TypeTun {
			continue
		}
		tun := inbound.Options.(*option.TunInboundOptions)
		if len(tun.Address) != 2 || !tun.Address[1].Addr().Is6() {
			t.Fatalf("dual-stack TUN should contain IPv6 address, got %v", tun.Address)
		}
		return
	}
	t.Fatal("TUN inbound was not generated")
}
