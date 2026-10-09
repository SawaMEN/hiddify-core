package config

import (
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
)

// Device privacy policy precedes profile rules. Region now controls only the
// optional Russian network bypass (geo/TLD rules); application policy remains
// independently configurable. Full tunnel deliberately wins over all bypasses.
func applyRegionalPrivacy(options *option.Options, h *HiddifyOptions) {
	defer applyNetworkPrivacy(options)
	if h.HandbookRouting {
		return
	}
	if h.FullTunnel || (h.PrivacyRoutingMode != "ru-bypass" && h.PrivacyRoutingMode != "proxy-selected") {
		return
	}

	rules := []option.Rule{
		{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultRule{RuleAction: option.RuleAction{Action: C.RuleActionTypeSniff}}},
		{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultRule{RawDefaultRule: option.RawDefaultRule{Port: []uint16{53}}, RuleAction: option.RuleAction{Action: C.RuleActionTypeHijackDNS}}},
		{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultRule{RawDefaultRule: option.RawDefaultRule{Protocol: []string{C.ProtocolDNS}}, RuleAction: option.RuleAction{Action: C.RuleActionTypeHijackDNS}}},
	}
	add := func(match option.RawDefaultRule, outbound string) {
		rules = append(rules, option.Rule{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultRule{RawDefaultRule: match, RuleAction: option.RuleAction{Action: C.RuleActionTypeRoute, RouteOptions: option.RouteActionOptions{Outbound: outbound}}}})
	}

	dnsRules := []option.DNSRule{}
	addDNS := func(domains []string, server string) {
		if len(domains) == 0 {
			return
		}
		dnsRules = append(dnsRules, option.DNSRule{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultDNSRule{RawDefaultDNSRule: option.RawDefaultDNSRule{DomainSuffix: domains}, DNSRuleAction: option.DNSRuleAction{Action: C.RuleActionTypeRoute, RouteOptions: option.DNSRouteActionOptions{Server: server}}}})
	}

	// The Android app maps the new "Russian domains and IPs" switch to Region=ru.
	// Keep .su and .рф alongside the generic core's .ru/geosite/geoip rules.
	directDomains := append([]string{}, h.PrivacyDirectDomains...)
	if h.Region == "ru" {
		directDomains = append([]string{"ru", "su", "xn--p1ai"}, directDomains...)
	}
	if len(directDomains) > 0 {
		add(option.RawDefaultRule{DomainSuffix: directDomains}, OutboundDirectTag)
		addDNS(directDomains, DNSMultiDirectTag)
	}
	if len(h.PrivacyDirectPackages) > 0 {
		add(option.RawDefaultRule{PackageName: h.PrivacyDirectPackages}, OutboundDirectTag)
	}

	// Restricted applications/domains always use the VPN when their independent
	// switch is enabled by the device policy. Direct application rules are placed
	// first so an explicit direct choice wins if lists overlap.
	if len(h.PrivacyProxyPackages) > 0 {
		add(option.RawDefaultRule{PackageName: h.PrivacyProxyPackages}, OutboundMainDetour)
	}
	if len(h.PrivacyProxyDomains) > 0 {
		add(option.RawDefaultRule{DomainSuffix: h.PrivacyProxyDomains}, OutboundMainDetour)
		addDNS(h.PrivacyProxyDomains, DNSRemoteTag)
	}

	if h.PrivacyRoutingMode == "proxy-selected" {
		// Legacy selected-services mode keeps its terminal direct fallback.
		add(option.RawDefaultRule{}, OutboundDirectTag)
		dnsRules = append(dnsRules, option.DNSRule{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultDNSRule{DNSRuleAction: option.DNSRuleAction{Action: C.RuleActionTypeRoute, RouteOptions: option.DNSRouteActionOptions{Server: DNSMultiDirectTag}}}})
	}

	options.Route.Rules = append(rules, options.Route.Rules...)
	options.DNS.Rules = append(dnsRules, options.DNS.Rules...)
}
