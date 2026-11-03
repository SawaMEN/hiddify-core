package config

import (
	"fmt"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/experimental/libbox"
	"github.com/sagernet/sing-box/option"
	"strings"
	"testing"
)

const chainStageTestProfile = `{"outbounds":[{"type":"socks","tag":"proxy-a","server":"127.0.0.2","server_port":1081}]}`

func chainTestBuild(t *testing.T, direction, mode string) *option.Options {
	t.Helper()
	opts := DefaultHiddifyOptions()
	opts.ChainStage = &ChainStageOptions{Direction: direction, Mode: mode, ProfileContent: chainStageTestProfile, Region: "DE"}
	built, err := ParseBuildConfig(libbox.BaseContext(nil), opts, &ReadOptions{Content: legacyDNSOutboundProfile})
	if err != nil {
		t.Fatal(err)
	}
	if err := libbox.CheckConfigOptions(built); err != nil {
		t.Fatal(err)
	}
	return built
}

func chainTestOutbound(t *testing.T, built *option.Options, tag string) option.Outbound {
	t.Helper()
	for _, out := range built.Outbounds {
		if out.Tag == tag {
			return out
		}
	}
	t.Fatalf("missing outbound %q", tag)
	return option.Outbound{}
}

func TestChainProfileExtraSecurityRoutesThroughStageThenMain(t *testing.T) {
	built := chainTestBuild(t, "extra_security", "profile")
	if built.Route.Final != chainPrefix+OutboundSelectTag {
		t.Fatalf("wrong route: %s", built.Route.Final)
	}
	main := chainTestOutbound(t, built, "proxy-a").Options.(*option.SOCKSOutboundOptions)
	extra := chainTestOutbound(t, built, chainPrefix+"proxy-a").Options.(*option.SOCKSOutboundOptions)
	if main.Detour != "" || extra.Detour != OutboundSelectTag {
		t.Fatalf("main=%q stage=%q", main.Detour, extra.Detour)
	}
	if main.Server == extra.Server {
		t.Fatal("stage profile replaced main instead of composing it")
	}
	selector := chainTestOutbound(t, built, chainPrefix+OutboundSelectTag).Options.(*option.SelectorOutboundOptions)
	if selector.Default != chainPrefix+"proxy-a" || selector.Outbounds[0] != chainPrefix+"proxy-a" {
		t.Fatalf("unmapped selector: %#v", selector)
	}
}

func TestChainProfileUnblockerRoutesMainTransportThroughStage(t *testing.T) {
	built := chainTestBuild(t, "unblocker", "profile")
	if built.Route.Final != OutboundSelectTag {
		t.Fatal(built.Route.Final)
	}
	if chainTestOutbound(t, built, "proxy-a").Options.(*option.SOCKSOutboundOptions).Detour != chainPrefix+OutboundSelectTag {
		t.Fatal("main transport bypasses stage")
	}
	if chainTestOutbound(t, built, chainPrefix+"proxy-a").Options.(*option.SOCKSOutboundOptions).Detour != "" {
		t.Fatal("stage transport cycles back to main")
	}
	for _, out := range built.Outbounds {
		if out.Type == C.TypeDirect && out.Options.(*option.DirectOutboundOptions).Detour != "" {
			t.Fatal("direct bootstrap became recursive")
		}
	}
}

func TestChainPsiphonBothDirectionsAndRegion(t *testing.T) {
	for _, direction := range []string{"extra_security", "unblocker"} {
		t.Run(direction, func(t *testing.T) {
			built := chainTestBuild(t, direction, "psiphon")
			psiphon := chainTestOutbound(t, built, chainPrefix+"Psiphon").Options.(*option.PsiphonOutboundOptions)
			if psiphon.Config != "hiddify" {
				t.Fatalf("Psiphon bootstrap config = %q, want hiddify", psiphon.Config)
			}
			if psiphon.EgressRegion != "DE" {
				t.Fatal("Psiphon region ignored")
			}
			if direction == "extra_security" && psiphon.Detour != OutboundSelectTag {
				t.Fatal("Psiphon bootstrap bypasses main")
			}
			if direction == "unblocker" && psiphon.Detour != "" {
				t.Fatal("Psiphon bootstrap cycle")
			}
		})
	}
}

func TestChainRejectsInvalidAndIncompatibleSettings(t *testing.T) {
	for _, test := range []struct {
		name      string
		configure func(*HiddifyOptions)
	}{
		{"direction", func(o *HiddifyOptions) { o.ChainStage.Direction = "other" }},
		{"mode", func(o *HiddifyOptions) { o.ChainStage.Mode = "other" }},
		{"empty profile", func(o *HiddifyOptions) { o.ChainStage.ProfileContent = "" }},
		{"large profile", func(o *HiddifyOptions) { o.ChainStage.ProfileContent = strings.Repeat("x", MaxConfigBytes+1) }},
		{"direct profile", func(o *HiddifyOptions) { o.ChainStage.ProfileContent = `{"type":"direct","tag":"exit"}` }},
		{"raw routing", func(o *HiddifyOptions) { o.EnableFullConfig = true }},
		{"legacy warp", func(o *HiddifyOptions) { o.Warp.EnableWarp = true }},
		{"region", func(o *HiddifyOptions) { o.ChainStage.Mode = "psiphon"; o.ChainStage.Region = "INVALID" }},
		{"modern psiphon", func(o *HiddifyOptions) { o.ChainStage.Mode = "psiphon"; o.ModernProtocolsOnly = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			opts := DefaultHiddifyOptions()
			opts.ChainStage = &ChainStageOptions{Direction: "unblocker", Mode: "profile", ProfileContent: chainStageTestProfile}
			test.configure(opts)
			_, err := ParseBuildConfig(libbox.BaseContext(nil), opts, &ReadOptions{Content: legacyDNSOutboundProfile})
			if err == nil {
				t.Fatal("accepted incompatible chain")
			}
		})
	}
}

func TestChainPreservesAndNamespacesStageDetourEdges(t *testing.T) {
	opts := DefaultHiddifyOptions()
	opts.ChainStage = &ChainStageOptions{Direction: "extra_security", Mode: "profile", ProfileContent: `{"outbounds":[{"type":"socks","tag":"exit","detour":"entry §hide§","server":"127.0.0.2","server_port":1081},{"type":"socks","tag":"entry §hide§","server":"127.0.0.3","server_port":1082}]}`}
	built, err := ParseBuildConfig(libbox.BaseContext(nil), opts, &ReadOptions{Content: legacyDNSOutboundProfile})
	if err != nil {
		t.Fatal(err)
	}
	if err := libbox.CheckConfigOptions(built); err != nil {
		t.Fatal(err)
	}
	if chainTestOutbound(t, built, chainPrefix+"exit").Options.(*option.SOCKSOutboundOptions).Detour != chainPrefix+"entry §hide§" {
		t.Fatal("internal detour lost")
	}
	if chainTestOutbound(t, built, chainPrefix+"entry §hide§").Options.(*option.SOCKSOutboundOptions).Detour != OutboundSelectTag {
		t.Fatal("terminal stage bypasses main")
	}
}

func TestChainDoesNotPublishTwoPsiphonDatastoreOwners(t *testing.T) {
	opts := DefaultHiddifyOptions()
	opts.ChainStage = &ChainStageOptions{Direction: "unblocker", Mode: "psiphon"}
	_, err := ParseBuildConfig(libbox.BaseContext(nil), opts, &ReadOptions{Content: `{"type":"psiphon","tag":"primary"}`})
	if err == nil || !strings.Contains(err.Error(), "one Psiphon") {
		t.Fatalf("expected explicit ownership error, got %v", err)
	}
}

func TestConcurrentChainBuildsKeepTheirOwnRouteTargets(t *testing.T) {
	errors := make(chan error, 16)
	for i := 0; i < 16; i++ {
		go func(i int) {
			direction := "extra_security"
			expected := chainPrefix + OutboundSelectTag
			if i%2 == 0 {
				direction = "unblocker"
				expected = OutboundSelectTag
			}
			opts := DefaultHiddifyOptions()
			opts.ChainStage = &ChainStageOptions{Direction: direction, Mode: "profile", ProfileContent: chainStageTestProfile}
			built, err := ParseBuildConfig(libbox.BaseContext(nil), opts, &ReadOptions{Content: legacyDNSOutboundProfile})
			if err == nil && built.Route.Final != expected {
				err = fmt.Errorf("route %q expected %q", built.Route.Final, expected)
			}
			errors <- err
		}(i)
	}
	for i := 0; i < 16; i++ {
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
	}
}
