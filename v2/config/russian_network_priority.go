package config

import (
	"time"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"
)

// PrivacyRussianNetworkBypass is the Android client's highest-priority network
// exception. Apply it last so app rules, community lists and full-tunnel
// fallbacks cannot capture Russian destinations first.
func applyRussianNetworkPriority(options *option.Options, h *HiddifyOptions) {
	if !h.PrivacyRussianNetworkBypass {
		return
	}
	for _, tag := range []string{"geosite-ru", "geoip-ru"} {
		found := false
		for _, set := range options.Route.RuleSet {
			for _, existing := range set.Tag {
				if existing == tag {
					found = true
				}
			}
		}
		if !found {
			options.Route.RuleSet = append(options.Route.RuleSet, option.RuleSet{
				Type: C.RuleSetTypeRemote, Tag: badoption.Listable[string]{tag}, Format: C.RuleSetFormatBinary,
				RemoteOptions: option.RemoteRuleSet{
					URL:            "https://raw.githubusercontent.com/hiddify/hiddify-geo/rule-set/country/" + tag + ".srs",
					UpdateInterval: badoption.Duration(5 * 24 * time.Hour), DownloadDetour: OutboundSelectTag,
				},
			})
		}
	}
	domains := append([]string{"ru", "su", "xn--p1ai"}, h.PrivacyDirectDomains...)
	direct := func(match option.RawDefaultRule) option.Rule {
		return option.Rule{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultRule{
			RawDefaultRule: match,
			RuleAction:     option.RuleAction{Action: C.RuleActionTypeRoute, RouteOptions: option.RouteActionOptions{Outbound: OutboundDirectTag}},
		}}
	}
	rules := []option.Rule{
		{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultRule{RuleAction: option.RuleAction{Action: C.RuleActionTypeSniff}}},
		{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultRule{RawDefaultRule: option.RawDefaultRule{Port: []uint16{53}}, RuleAction: option.RuleAction{Action: C.RuleActionTypeHijackDNS}}},
		{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultRule{RawDefaultRule: option.RawDefaultRule{Protocol: []string{C.ProtocolDNS}}, RuleAction: option.RuleAction{Action: C.RuleActionTypeHijackDNS}}},
		direct(option.RawDefaultRule{DomainSuffix: domains}),
		direct(option.RawDefaultRule{RuleSet: []string{"geosite-ru", "geoip-ru"}}),
	}
	options.Route.Rules = append(rules, options.Route.Rules...)
	dns := func(match option.RawDefaultDNSRule) option.DNSRule {
		return option.DNSRule{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultDNSRule{
			RawDefaultDNSRule: match,
			DNSRuleAction:     option.DNSRuleAction{Action: C.RuleActionTypeRoute, RouteOptions: option.DNSRouteActionOptions{Server: DNSMultiDirectTag}},
		}}
	}
	options.DNS.Rules = append([]option.DNSRule{
		dns(option.RawDefaultDNSRule{DomainSuffix: domains}),
		dns(option.RawDefaultDNSRule{RuleSet: []string{"geosite-ru"}}),
	}, options.DNS.Rules...)
}
