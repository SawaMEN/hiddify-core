package config

import (
	"context"
	"fmt"
	"strings"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
)

// ChainStageOptions is an ephemeral app-resolved snapshot. No filesystem paths
// supplied by a subscription or backup are opened by the core.
type ChainStageOptions struct {
	Direction        string `json:"direction"`
	Mode             string `json:"mode"`
	Region           string `json:"region,omitempty"`
	ConduitPairingID string `json:"conduit-pairing-id,omitempty"`
	ProfileContent   string `json:"profile-content,omitempty"`
}

const chainPrefix = "chain §hide§ / "

func applyChainStage(ctx context.Context, built *option.Options, opts *HiddifyOptions) error {
	stage := opts.ChainStage
	if stage == nil {
		return nil
	}
	if opts.ChainStatus != "" && opts.ChainStatus != ChainStatusOff {
		return fmt.Errorf("cannot combine chain-stage and chain-status")
	}
	if stage.Direction != "extra_security" && stage.Direction != "unblocker" {
		return fmt.Errorf("invalid chain direction %q", stage.Direction)
	}
	if opts.EnableFullConfig {
		return fmt.Errorf("disable execute-config-as-is before enabling a chain stage")
	}
	if opts.Warp.EnableWarp || opts.Warp2.EnableWarp {
		return fmt.Errorf("cannot combine a chain stage with legacy WARP")
	}
	for _, out := range built.Outbounds {
		if strings.HasPrefix(out.Tag, chainPrefix) {
			return fmt.Errorf("reserved chain tag: %q", out.Tag)
		}
	}
	for _, end := range built.Endpoints {
		if strings.HasPrefix(end.Tag, chainPrefix) {
			return fmt.Errorf("reserved chain tag: %q", end.Tag)
		}
	}
	stageOptions := DefaultHiddifyOptions()
	stageOptions.ModernProtocolsOnly = opts.ModernProtocolsOnly
	stageOptions.ModernAllowUDP = opts.ModernAllowUDP
	stageOptions.AdaptiveNetwork = opts.AdaptiveNetwork
	var input *option.Options
	switch stage.Mode {
	case "psiphon":
		if opts.ModernProtocolsOnly {
			return fmt.Errorf("modern protocols only: Psiphon chain is incompatible")
		}
		region := strings.ToUpper(strings.TrimSpace(stage.Region))
		if region == "AUTO" {
			region = ""
		}
		if region != "" && (len(region) != 2 || region[0] < 'A' || region[0] > 'Z' || region[1] < 'A' || region[1] > 'Z') {
			return fmt.Errorf("invalid Psiphon region")
		}
		input = &option.Options{Outbounds: []option.Outbound{{Type: C.TypePsiphon, Tag: "Psiphon", Options: &option.PsiphonOutboundOptions{
			Config: "hiddify", EgressRegion: region, ConduitPairingID: strings.TrimSpace(stage.ConduitPairingID),
		}}}}
	case "profile":
		if len(stage.ProfileContent) == 0 || len(stage.ProfileContent) > MaxConfigBytes {
			return fmt.Errorf("chain profile is empty or exceeds 8 MiB")
		}
		var err error
		input, err = ParseConfig(ctx, &ReadOptions{Content: stage.ProfileContent}, false, stageOptions, false)
		if err != nil {
			return fmt.Errorf("chain profile: %w", err)
		}
	default:
		return fmt.Errorf("unsupported chain mode %q", stage.Mode)
	}
	for _, out := range input.Outbounds {
		if out.Type == C.TypeDirect && out.Tag != "direct" && out.Tag != "bypass" {
			return fmt.Errorf("chain profile contains a selectable direct outbound")
		}
	}

	// setOutbounds still uses the legacy builder globals. Save them around the
	// secondary group so the main route and DNS retain their intended target.
	main, warp := OutboundMainDetour, OutboundWARPConfigDetour
	var extra option.Options
	ips := make(map[string][]string)
	err := setOutbounds(&extra, input, stageOptions, &ips)
	OutboundMainDetour, OutboundWARPConfigDetour = main, warp
	if err != nil {
		return fmt.Errorf("chain stage: %w", err)
	}
	psiphons := 0
	for _, group := range [][]option.Outbound{built.Outbounds, extra.Outbounds} {
		for _, out := range group {
			if out.Type == C.TypePsiphon {
				psiphons++
			}
		}
	}
	if psiphons > 1 {
		return fmt.Errorf("only one Psiphon instance is supported per connection")
	}

	tags := make(map[string]string)
	for _, out := range extra.Outbounds {
		tags[out.Tag] = chainPrefix + out.Tag
	}
	for _, end := range extra.Endpoints {
		tags[end.Tag] = chainPrefix + end.Tag
	}
	rename := func(tag string) string {
		if name, ok := tags[tag]; ok {
			return name
		}
		return tag
	}
	for i := range extra.Outbounds {
		out := &extra.Outbounds[i]
		out.Tag = rename(out.Tag)
		if err := rewriteChainOptions(out.Options, rename, stage.Direction == "extra_security", OutboundSelectTag); err != nil {
			return fmt.Errorf("chain outbound %q: %w", out.Tag, err)
		}
	}
	for i := range extra.Endpoints {
		end := &extra.Endpoints[i]
		end.Tag = rename(end.Tag)
		if err := rewriteChainOptions(end.Options, rename, stage.Direction == "extra_security", OutboundSelectTag); err != nil {
			return fmt.Errorf("chain endpoint %q: %w", end.Tag, err)
		}
	}
	stageTag := rename(OutboundSelectTag)
	if stage.Direction == "extra_security" {
		OutboundMainDetour = stageTag
	} else {
		identity := func(tag string) string { return tag }
		for i := range built.Outbounds {
			out := &built.Outbounds[i]
			// Built-in direct routes remain available for bootstrap and LAN policy.
			if out.Type == C.TypeDirect {
				continue
			}
			if err := rewriteChainOptions(out.Options, identity, true, stageTag); err != nil {
				return fmt.Errorf("main outbound %q: %w", out.Tag, err)
			}
		}
		for i := range built.Endpoints {
			if err := rewriteChainOptions(built.Endpoints[i].Options, identity, true, stageTag); err != nil {
				return err
			}
		}
	}
	built.Outbounds = append(built.Outbounds, extra.Outbounds...)
	built.Endpoints = append(built.Endpoints, extra.Endpoints...)
	return nil
}

func rewriteChainOptions(value any, rename func(string) string, redirect bool, target string) error {
	switch group := value.(type) {
	case *option.DirectOutboundOptions:
		return nil
	case *option.SelectorOutboundOptions:
		for i, tag := range group.Outbounds {
			group.Outbounds[i] = rename(tag)
		}
		group.Default = rename(group.Default)
		return nil
	case *option.URLTestOutboundOptions:
		for i, tag := range group.Outbounds {
			group.Outbounds[i] = rename(tag)
		}
		return nil
	case *option.BalancerOutboundOptions:
		for i, tag := range group.Outbounds {
			group.Outbounds[i] = rename(tag)
		}
		return nil
	}
	dialer, ok := value.(option.DialerOptionsWrapper)
	if !ok {
		return fmt.Errorf("outbound cannot participate in a detour chain")
	}
	options := dialer.TakeDialerOptions()
	if redirect && (options.Detour == "" || options.Detour == OutboundDirectTag || options.Detour == OutboundDirectFragmentTag) {
		options.Detour = target
	} else {
		options.Detour = rename(options.Detour)
	}
	dialer.ReplaceDialerOptions(options)
	if warp, ok := value.(*option.WARPEndpointOptions); ok {
		warp.Profile.Detour = rename(warp.Profile.Detour)
	}
	return nil
}
