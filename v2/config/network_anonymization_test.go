package config

import (
	"testing"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
)

func TestNetworkPrivacyPolicyPrependsExperimentalFilters(t *testing.T) {
	defer SetNetworkPrivacyPolicyJSON("{}")
	if err := SetNetworkPrivacyPolicyJSON(`{
		"privacy-anonymization-block-quic":true,
		"privacy-anonymization-block-stun":true,
		"privacy-anonymization-block-plain-http":true,
		"privacy-anonymization-isolate-lan":true
	}`); err != nil {
		t.Fatal(err)
	}

	options := &option.Options{Route: &option.RouteOptions{Rules: []option.Rule{{
		Type: C.RuleTypeDefault,
		DefaultOptions: option.DefaultRule{
			RuleAction: option.RuleAction{Action: C.RuleActionTypeRoute, RouteOptions: option.RouteActionOptions{Outbound: OutboundDirectTag}},
		},
	}}}}
	applyNetworkPrivacy(options)

	if len(options.Route.Rules) != 8 {
		t.Fatalf("expected 7 privacy rules before the existing rule, got %d", len(options.Route.Rules))
	}
	if options.Route.Rules[0].DefaultOptions.Action != C.RuleActionTypeSniff {
		t.Fatal("privacy protocol filters must be preceded by sniffing")
	}
	if options.Route.Rules[1].DefaultOptions.Action != C.RuleActionTypeHijackDNS ||
		len(options.Route.Rules[1].DefaultOptions.Port) != 1 || options.Route.Rules[1].DefaultOptions.Port[0] != 53 {
		t.Fatalf("expected DNS port hijack before LAN isolation, got %#v", options.Route.Rules[1].DefaultOptions)
	}
	if options.Route.Rules[2].DefaultOptions.Action != C.RuleActionTypeHijackDNS {
		t.Fatalf("expected protocol DNS hijack before LAN isolation, got %#v", options.Route.Rules[2].DefaultOptions)
	}
	protocols := []string{C.ProtocolQUIC, C.ProtocolSTUN, C.ProtocolHTTP}
	for i, protocol := range protocols {
		rule := options.Route.Rules[i+3].DefaultOptions
		if rule.Action != C.RuleActionTypeReject || len(rule.Protocol) != 1 || rule.Protocol[0] != protocol {
			t.Fatalf("unexpected protocol privacy rule %d: %#v", i, rule)
		}
		if len(rule.Inbound) != 1 || rule.Inbound[0] != InboundTUNTag {
			t.Fatalf("privacy rule must be scoped to TUN: %#v", rule)
		}
	}
	lan := options.Route.Rules[6].DefaultOptions
	if lan.Action != C.RuleActionTypeReject || !lan.IPIsPrivate {
		t.Fatalf("expected private-network isolation rule, got %#v", lan)
	}
	if options.Route.Rules[7].DefaultOptions.Action != C.RuleActionTypeRoute {
		t.Fatal("existing routes must remain after privacy filters")
	}
}
