package config

import (
	"encoding/json"
	"sync"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
)

type NetworkPrivacyPolicy struct {
	BlockQUIC      bool `json:"privacy-anonymization-block-quic,omitempty"`
	BlockSTUN      bool `json:"privacy-anonymization-block-stun,omitempty"`
	BlockPlainHTTP bool `json:"privacy-anonymization-block-plain-http,omitempty"`
	IsolateLAN     bool `json:"privacy-anonymization-isolate-lan,omitempty"`
}

var networkPrivacyState struct {
	sync.RWMutex
	policy NetworkPrivacyPolicy
}

func SetNetworkPrivacyPolicyJSON(policyJSON string) error {
	var next NetworkPrivacyPolicy
	if err := json.Unmarshal([]byte(policyJSON), &next); err != nil {
		return err
	}
	networkPrivacyState.Lock()
	networkPrivacyState.policy = next
	networkPrivacyState.Unlock()
	return nil
}

func networkPrivacyPolicy() NetworkPrivacyPolicy {
	networkPrivacyState.RLock()
	defer networkPrivacyState.RUnlock()
	return networkPrivacyState.policy
}

func rejectPrivacyTraffic(match option.RawDefaultRule) option.Rule {
	return option.Rule{
		Type: C.RuleTypeDefault,
		DefaultOptions: option.DefaultRule{
			RawDefaultRule: match,
			RuleAction: option.RuleAction{
				Action:        C.RuleActionTypeReject,
				RejectOptions: option.RejectActionOptions{Method: C.RuleActionRejectMethodDefault},
			},
		},
	}
}

func applyNetworkPrivacy(options *option.Options) {
	policy := networkPrivacyPolicy()
	if !policy.BlockQUIC && !policy.BlockSTUN && !policy.BlockPlainHTTP && !policy.IsolateLAN {
		return
	}

	rules := make([]option.Rule, 0, 7)
	rules = append(rules, option.Rule{
		Type: C.RuleTypeDefault,
		DefaultOptions: option.DefaultRule{
			RawDefaultRule: option.RawDefaultRule{Inbound: []string{InboundTUNTag}},
			RuleAction:     option.RuleAction{Action: C.RuleActionTypeSniff},
		},
	})
	if policy.IsolateLAN {
		// The Android TUN DNS address is private too. Hijack DNS before rejecting
		// private destinations so LAN isolation cannot cut off name resolution.
		rules = append(rules,
			option.Rule{
				Type: C.RuleTypeDefault,
				DefaultOptions: option.DefaultRule{
					RawDefaultRule: option.RawDefaultRule{Inbound: []string{InboundTUNTag}, Port: []uint16{53}},
					RuleAction:     option.RuleAction{Action: C.RuleActionTypeHijackDNS},
				},
			},
			option.Rule{
				Type: C.RuleTypeDefault,
				DefaultOptions: option.DefaultRule{
					RawDefaultRule: option.RawDefaultRule{Inbound: []string{InboundTUNTag}, Protocol: []string{C.ProtocolDNS}},
					RuleAction:     option.RuleAction{Action: C.RuleActionTypeHijackDNS},
				},
			},
		)
	}
	if policy.BlockQUIC {
		rules = append(rules, rejectPrivacyTraffic(option.RawDefaultRule{Inbound: []string{InboundTUNTag}, Protocol: []string{C.ProtocolQUIC}}))
	}
	if policy.BlockSTUN {
		rules = append(rules, rejectPrivacyTraffic(option.RawDefaultRule{Inbound: []string{InboundTUNTag}, Protocol: []string{C.ProtocolSTUN}}))
	}
	if policy.BlockPlainHTTP {
		rules = append(rules, rejectPrivacyTraffic(option.RawDefaultRule{Inbound: []string{InboundTUNTag}, Protocol: []string{C.ProtocolHTTP}}))
	}
	if policy.IsolateLAN {
		rules = append(rules, rejectPrivacyTraffic(option.RawDefaultRule{Inbound: []string{InboundTUNTag}, IPIsPrivate: true}))
	}
	options.Route.Rules = append(rules, options.Route.Rules...)
}
