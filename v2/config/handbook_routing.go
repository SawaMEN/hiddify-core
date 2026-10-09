package config

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"
)

func handbookURL(host, sites string) string {
	q := url.Values{"format": {"json"}}
	// The provider accepts multiple site= parameters, not a comma-separated value.
	for _, site := range strings.FieldsFunc(sites, func(c rune) bool { return c == ',' || c == ';' || c == '\n' || c == ' ' }) {
		q.Add("site", strings.ToLower(site))
	}
	return "https://" + host + "/?" + q.Encode()
}

func applyHandbookRouting(ctx context.Context, options *option.Options, h *HiddifyOptions) error {
	rules := []option.Rule{
		{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultRule{RuleAction: option.RuleAction{Action: C.RuleActionTypeSniff}}},
		{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultRule{RawDefaultRule: option.RawDefaultRule{Port: []uint16{53}}, RuleAction: option.RuleAction{Action: C.RuleActionTypeHijackDNS}}},
		{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultRule{RawDefaultRule: option.RawDefaultRule{Protocol: []string{C.ProtocolDNS}}, RuleAction: option.RuleAction{Action: C.RuleActionTypeHijackDNS}}},
	}
	dnsRules := []option.DNSRule{}
	// Proxy takes precedence when the providers contain overlapping entries.
	for _, source := range []struct {
		enabled                         bool
		host, tag, sites, outbound, dns string
	}{
		{h.HandbookProxy, "iplist.my-handbook.ru", "handbook-proxy", h.HandbookProxySites, OutboundMainDetour, DNSRemoteTag},
		{h.HandbookDirect, "ru-iplist.my-handbook.ru", "handbook-direct", h.HandbookDirectSites, OutboundDirectTag, DNSMultiDirectTag},
	} {
		if !source.enabled {
			continue
		}
		catalogue, err := loadHandbook(ctx, handbookURL(source.host, source.sites))
		if err != nil {
			return fmt.Errorf("%s: %w", source.host, err)
		}
		domains, cidrs, err := parseHandbook(catalogue)
		if err != nil {
			return fmt.Errorf("%s: %w", source.host, err)
		}
		tags := []string{}
		for _, data := range []string{"domains", "cidrs"} {
			tag := source.tag + "-" + data
			var match option.DefaultHeadlessRule
			if data == "domains" {
				if len(domains) == 0 {
					continue
				}
				match.DomainSuffix = domains
			} else {
				if len(cidrs) == 0 {
					continue
				}
				match.IPCIDR = cidrs
			}
			tags = append(tags, tag)
			options.Route.RuleSet = append(options.Route.RuleSet, option.RuleSet{
				Type: C.RuleSetTypeInline, Tag: badoption.Listable[string]{tag},
				InlineOptions: option.PlainRuleSet{Rules: []option.HeadlessRule{{Type: C.RuleTypeDefault, DefaultOptions: match}}},
			})
			if data == "domains" {
				dnsRules = append(dnsRules, option.DNSRule{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultDNSRule{
					RawDefaultDNSRule: option.RawDefaultDNSRule{RuleSet: []string{tag}},
					DNSRuleAction:     option.DNSRuleAction{Action: C.RuleActionTypeRoute, RouteOptions: option.DNSRouteActionOptions{Server: source.dns}},
				}})
			}
		}
		if len(tags) == 0 {
			return fmt.Errorf("%s returned an empty routing list", source.host)
		}
		rules = append(rules, option.Rule{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultRule{
			RawDefaultRule: option.RawDefaultRule{RuleSet: tags},
			RuleAction:     option.RuleAction{Action: C.RuleActionTypeRoute, RouteOptions: option.RouteActionOptions{Outbound: source.outbound}},
		}})
	}
	options.Route.Rules = append(rules, options.Route.Rules...)
	options.DNS.Rules = append(dnsRules, options.DNS.Rules...)
	return nil
}
