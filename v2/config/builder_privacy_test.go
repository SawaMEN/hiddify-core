package config

import (
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/experimental/libbox"
	"github.com/sagernet/sing-box/option"
	"strings"
	"testing"
)

func TestPrivacyRemovesProxyListenersButKeepsTun(t *testing.T) {
	opts := DefaultHiddifyOptions()
	opts.EnableTun = true
	opts.DisableLocalProxy = true
	var built option.Options
	setInbound(&built, opts)
	if len(built.Inbounds) != 1 || built.Inbounds[0].Type != C.TypeTun {
		t.Fatalf("expected only TUN, got %#v", built.Inbounds)
	}
}

func TestDisabledMixedPortDoesNotBindEphemeralPort(t *testing.T) {
	opts := DefaultHiddifyOptions()
	opts.MixedPort = 0
	opts.DirectPort = 0
	var built option.Options
	setInbound(&built, opts)
	for _, inbound := range built.Inbounds {
		if inbound.Type == C.TypeMixed {
			t.Fatal("port 0 must disable the mixed inbound")
		}
	}
}

func TestNoClashListenerKeepsMonitoringAndTrafficAccounting(t *testing.T) {
	opts := DefaultHiddifyOptions()
	opts.EnableClashApi = false
	var built option.Options
	setExperimental(&built, opts)
	if built.Experimental.ClashAPI == nil || built.Experimental.ClashAPI.ExternalController != "" {
		t.Fatal("internal accounting should exist without an external controller")
	}
	if built.Experimental.Monitoring == nil || built.Experimental.CacheFile == nil {
		t.Fatal("monitoring and selection cache must survive closing the REST port")
	}
}

func TestFullTunnelOverridesDirectRulesAndDnsOnlyForTun(t *testing.T) {
	opts := DefaultHiddifyOptions()
	opts.EnableTun = true
	opts.FullTunnel = true
	opts.Region = "ru"
	opts.BypassLAN = true
	built, err := BuildConfig(libbox.BaseContext(nil), opts, &ReadOptions{Content: legacyDNSOutboundProfile})
	if err != nil {
		t.Fatal(err)
	}
	if len(built.Route.Rules) < 4 {
		t.Fatal("missing full tunnel policy")
	}
	catchAll := built.Route.Rules[3].DefaultOptions
	if catchAll.Action != C.RuleActionTypeRoute || catchAll.RouteOptions.Outbound != OutboundMainDetour ||
		len(catchAll.Inbound) != 1 || catchAll.Inbound[0] != InboundTUNTag {
		t.Fatalf("TUN must route through the selected proxy: %#v", catchAll)
	}
	dnsRule := built.DNS.Rules[0].DefaultOptions
	if dnsRule.RouteOptions.Server != DNSRemoteTag || len(dnsRule.Inbound) != 1 || dnsRule.Inbound[0] != InboundTUNTag {
		t.Fatalf("TUN DNS must use the remote resolver: %#v", dnsRule)
	}
	if built.Route.DefaultDomainResolver.Server != DNSMultiDirectTag {
		t.Fatal("bootstrap resolution must remain available outside the TUN policy")
	}
}

func TestImportedListenersCannotOverrideDevicePrivacy(t *testing.T) {
	opts := DefaultHiddifyOptions()
	opts.EnableTun = true
	opts.EnableFullConfig = true
	opts.DisableLocalProxy = true
	profile := strings.Replace(legacyDNSOutboundProfile, "{", `{"inbounds":[{"type":"mixed","tag":"imported","listen":"127.0.0.1","listen_port":9000}],`, 1)
	built, err := BuildConfig(libbox.BaseContext(nil), opts, &ReadOptions{Content: profile})
	if err != nil {
		t.Fatal(err)
	}
	if len(built.Inbounds) != 1 || built.Inbounds[0].Type != C.TypeTun {
		t.Fatalf("imported listeners bypassed privacy: %#v", built.Inbounds)
	}
}
