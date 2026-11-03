package config

import (
	"testing"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/experimental/libbox"
	"github.com/sagernet/sing-box/option"
)

func hasPackageRoute(rules []option.Rule, pkg, outbound string) bool {
	for _, rule := range rules {
		for _, candidate := range rule.DefaultOptions.PackageName {
			if candidate == pkg && rule.DefaultOptions.RouteOptions.Outbound == outbound {
				return true
			}
		}
	}
	return false
}

func hasDomainRoute(rules []option.Rule, domain, outbound string) bool {
	for _, rule := range rules {
		for _, candidate := range rule.DefaultOptions.DomainSuffix {
			if candidate == domain && rule.DefaultOptions.RouteOptions.Outbound == outbound {
				return true
			}
		}
	}
	return false
}

func TestRegionalPrivacyIndependentFromRegion(t *testing.T) {
	for _, region := range []string{"other", "ru"} {
		t.Run(region, func(t *testing.T) {
			h := DefaultHiddifyOptions()
			h.Region = region
			h.PrivacyRoutingMode = "ru-bypass"
			h.PrivacyDirectPackages = []string{"ru.example.bank"}
			h.PrivacyProxyPackages = []string{"com.example.foreign"}
			h.PrivacyDirectDomains = []string{"bank.example"}
			h.PrivacyProxyDomains = []string{"foreign.example"}

			opts, err := BuildConfig(libbox.BaseContext(nil), h, &ReadOptions{Content: legacyDNSOutboundProfile})
			if err != nil {
				t.Fatal(err)
			}
			if opts.Route.Rules[0].DefaultOptions.Action != C.RuleActionTypeSniff {
				t.Fatal("privacy policy must sniff before device routing rules")
			}
			if !hasPackageRoute(opts.Route.Rules, "ru.example.bank", OutboundDirectTag) {
				t.Fatal("direct application policy must work independently from region")
			}
			if !hasPackageRoute(opts.Route.Rules, "com.example.foreign", OutboundMainDetour) {
				t.Fatal("restricted application policy must work independently from region")
			}
			if !hasDomainRoute(opts.Route.Rules, "bank.example", OutboundDirectTag) {
				t.Fatal("explicit direct domains must work independently from region")
			}
			if !hasDomainRoute(opts.Route.Rules, "foreign.example", OutboundMainDetour) {
				t.Fatal("restricted domains must work independently from region")
			}
			if region == "ru" && !hasDomainRoute(opts.Route.Rules, "xn--p1ai", OutboundDirectTag) {
				t.Fatal("Russian network mode must include .рф")
			}
			if region == "other" && hasDomainRoute(opts.Route.Rules, "xn--p1ai", OutboundDirectTag) {
				t.Fatal("other network mode must not force .рф direct")
			}
		})
	}
}

func TestFullTunnelWinsOverPrivacyBypass(t *testing.T) {
	h := DefaultHiddifyOptions()
	h.Region = "ru"
	h.PrivacyRoutingMode = "ru-bypass"
	h.PrivacyDirectPackages = []string{"ru.example.bank"}
	h.FullTunnel = true
	h.EnableTun = true

	opts, err := BuildConfig(libbox.BaseContext(nil), h, &ReadOptions{Content: legacyDNSOutboundProfile})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Route.Rules[3].DefaultOptions.RouteOptions.Outbound != OutboundMainDetour {
		t.Fatal("full tunnel must win over regional bypass")
	}
}
