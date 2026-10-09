package config

import (
	"testing"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/experimental/libbox"
	"github.com/sagernet/sing-box/option"
)

func TestAndroidResolvedChainConsumesEditorStateOnce(t *testing.T) {
	for _, direction := range []string{ChainStatusExtraSecurity, ChainStatusUnblocker} {
		opts := DefaultHiddifyOptions()
		opts.ChainStatus = direction
		opts.ChainStage = &ChainStageOptions{Direction: direction, Mode: ChainModeProfile, ProfileContent: chainStageTestProfile}
		built, err := BuildConfig(libbox.BaseContext(nil), opts, &ReadOptions{Content: chainTestProfile})
		if err != nil {
			t.Fatal(err)
		}
		if err := libbox.CheckConfigOptions(built); err != nil {
			t.Fatal(err)
		}
		if opts.ChainStatus != direction {
			t.Fatal("persisted options mutated")
		}
		if direction == ChainStatusExtraSecurity && built.Route.Final != chainPrefix+OutboundSelectTag {
			t.Fatal(built.Route.Final)
		}
		if direction == ChainStatusUnblocker && outboundDetour(t, built, "proxy-a") != chainPrefix+OutboundSelectTag {
			t.Fatal("missing detour")
		}
	}
}

func TestAndroidProjectedWarpCreatesOneNativeHop(t *testing.T) {
	built := buildChainConfig(t, `{"chain-status":"unblocker","unblocker":{"mode":"warp","warp":{"license-key":"key"}},"warp":{"enable":true,"mode":"proxy_over_warp","id":"key"}}`)
	if len(built.Endpoints) != 1 || built.Endpoints[0].Type != C.TypeWARP {
		t.Fatal("expected one WARP hop")
	}
	if built.Endpoints[0].Options.(*option.WARPEndpointOptions).Profile.License != "key" {
		t.Fatal("license lost")
	}
}

func TestLegacyWarpMigratesWithoutDroppingDetour(t *testing.T) {
	built := buildChainConfig(t, `{"warp":{"enable":true,"mode":"proxy_over_warp","id":"old-key","clean-ip":"162.159.192.1","clean-port":2408}}`)
	if outboundDetour(t, built, "proxy-a") != ChainUnblockerTag {
		t.Fatal("legacy WARP bypassed")
	}
	w := built.Endpoints[0].Options.(*option.WARPEndpointOptions)
	if w.Profile.License != "old-key" || w.ServerPort != 2408 {
		t.Fatal("legacy WARP options lost")
	}
}

func TestChainCompatibilityRejectsAmbiguousOrForbiddenModes(t *testing.T) {
	for _, settings := range []string{
		`{"chain-status":"unblocker","chain-stage":{"direction":"extra_security","mode":"psiphon"}}`,
		`{"warp2":{"enable":true}}`,
		`{"warp":{"enable":true,"mode":"unknown"}}`,
		`{"chain-status":"unblocker","unblocker":{"mode":"warp"},"privacy-modern-protocols-only":true}`,
		`{"chain-status":"unblocker","unblocker":{"mode":"psiphon"},"enable-full-config":true}`,
	} {
		if _, err := BuildConfig(libbox.BaseContext(nil), chainTestOptions(t, settings), &ReadOptions{Content: chainTestProfile}); err == nil {
			t.Fatalf("accepted %s", settings)
		}
	}
}

func TestNativePsiphonPreservesPairingAndNormalizesRegion(t *testing.T) {
	built := buildChainConfig(t, `{"chain-status":"unblocker","unblocker":{"mode":"psiphon","psiphon":{"region":" de ","conduit-pairing-id":" pair "}}}`)
	for _, out := range built.Outbounds {
		if out.Tag == ChainUnblockerTag {
			p := out.Options.(*option.PsiphonOutboundOptions)
			if p.EgressRegion != "DE" || p.ConduitPairingID != "pair" {
				t.Fatal("Psiphon settings lost")
			}
			return
		}
	}
	t.Fatal("missing Psiphon")
}
