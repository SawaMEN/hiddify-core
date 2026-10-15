package config

import (
	"reflect"
	"testing"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/experimental/libbox"
	"github.com/sagernet/sing-box/option"
)

func TestRussianNetworkPriorityAcrossRoutingModes(t *testing.T) {
	for _, mode := range []string{"off", "ru-bypass", "proxy-selected"} {
		for _, full := range []bool{false, true} {
			for _, community := range []bool{false, true} {
				h := DefaultHiddifyOptions()
				h.Region, h.PrivacyRoutingMode, h.FullTunnel, h.HandbookRouting = "ru", mode, full, community
				h.PrivacyRussianNetworkBypass = true
				h.PrivacyProxyPackages = []string{"com.example.app"}
				h.PrivacyProxyDomains = []string{"example.ru"}
				// Simulate existing community/profile/full-tunnel rules: they must stay behind the exception.
				existing := option.Rule{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultRule{
					RuleAction: option.RuleAction{Action: C.RuleActionTypeRoute, RouteOptions: option.RouteActionOptions{Outbound: OutboundMainDetour}},
				}}
				opts := &option.Options{Route: &option.RouteOptions{Rules: []option.Rule{existing}}, DNS: &option.DNSOptions{}}
				applyRegionalPrivacy(opts, h)
				if opts.Route.Rules[0].DefaultOptions.Action != C.RuleActionTypeSniff ||
					opts.Route.Rules[1].DefaultOptions.Action != C.RuleActionTypeHijackDNS {
					t.Fatal("sniffing and DNS interception must precede routing")
				}
				domains, geo := opts.Route.Rules[3].DefaultOptions, opts.Route.Rules[4].DefaultOptions
				if domains.RouteOptions.Outbound != OutboundDirectTag ||
					!reflect.DeepEqual([]string(domains.DomainSuffix), []string{"ru", "su", "xn--p1ai"}) {
					t.Fatalf("Russian domains must route first: mode=%s full=%v community=%v", mode, full, community)
				}
				if geo.RouteOptions.Outbound != OutboundDirectTag ||
					!reflect.DeepEqual([]string(geo.RuleSet), []string{"geosite-ru", "geoip-ru"}) {
					t.Fatal("Russian geosite and IP rules must precede VPN application rules")
				}
				if opts.Route.Rules[len(opts.Route.Rules)-1].DefaultOptions.RouteOptions.Outbound != OutboundMainDetour {
					t.Fatal("existing VPN fallback must be preserved")
				}
				if !reflect.DeepEqual([]string(opts.DNS.Rules[1].DefaultOptions.RuleSet), []string{"geosite-ru"}) ||
					opts.DNS.Rules[1].DefaultOptions.RouteOptions.Server != DNSMultiDirectTag {
					t.Fatal("Russian names must resolve directly without a GeoIP DNS matcher")
				}
				if len(opts.Route.RuleSet) != 2 {
					t.Fatal("both geographic datasets are required")
				}
				applyRussianNetworkPriority(opts, h)
				if len(opts.Route.RuleSet) != 2 {
					t.Fatal("existing geographic datasets must not be duplicated")
				}
			}
		}
	}
}

func TestDisabledRussianNetworkAddsNoPriorityRules(t *testing.T) {
	h := DefaultHiddifyOptions()
	h.Region = "other"
	opts := &option.Options{Route: &option.RouteOptions{}, DNS: &option.DNSOptions{}}
	applyRussianNetworkPriority(opts, h)
	if len(opts.Route.Rules) != 0 || len(opts.Route.RuleSet) != 0 || len(opts.DNS.Rules) != 0 {
		t.Fatal("disabled network exception must not add rules or download datasets")
	}
}

func TestRussianNetworkPriorityInGeneratedFullTunnelConfig(t *testing.T) {
	h := DefaultHiddifyOptions()
	h.Region = "ru"
	h.PrivacyRussianNetworkBypass = true
	h.FullTunnel = true
	h.EnableTun = true
	built, err := BuildConfig(libbox.BaseContext(nil), h, &ReadOptions{Content: legacyDNSOutboundProfile})
	if err != nil {
		t.Fatal(err)
	}
	if built.Route.Rules[4].DefaultOptions.RouteOptions.Outbound != OutboundDirectTag {
		t.Fatal("Russian geographic exception must precede full tunnel")
	}
	if built.DNS.Rules[1].DefaultOptions.RouteOptions.Server != DNSMultiDirectTag {
		t.Fatal("Russian DNS exception must precede full tunnel DNS")
	}
	foundTunnel := false
	for _, r := range built.Route.Rules {
		if r.DefaultOptions.RouteOptions.Outbound == OutboundMainDetour {
			foundTunnel = true
		}
	}
	if !foundTunnel {
		t.Fatal("other traffic must retain the VPN fallback")
	}
}
