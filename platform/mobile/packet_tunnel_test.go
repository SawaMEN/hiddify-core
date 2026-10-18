package mobile

import (
	"strings"
	"testing"

	"github.com/hiddify/hiddify-core/v2/config"
)

func TestPacketTunnelSettingsDoNotExposeProxyListeners(t *testing.T) {
	s := packetTunnelSettings()
	if !s.EnableTun || !s.FullTunnel || !s.StrictRoute {
		t.Fatal("a macOS packet tunnel must always create a strict full-route TUN")
	}
	if !s.DisableLocalProxy || s.EnableClashApi || s.SetSystemProxy || s.AllowConnectionFromLAN {
		t.Fatal("packet tunnel must not expose local proxy/API servers")
	}
	if s.MixedPort != 0 || s.DirectPort != 0 || s.TProxyPort != 0 || s.RedirectPort != 0 {
		t.Fatal("local listener ports must be disabled")
	}
	if s.EnableFullConfig || s.MTU != 1500 {
		t.Fatal("packet tunnel must use a bounded MTU and its own managed routing")
	}
	if !strings.HasPrefix(s.RemoteDnsAddress, "https://") ||
		!strings.HasPrefix(s.DirectDnsAddress, "https://") {
		t.Fatal("packet tunnel DNS bootstrap must use HTTPS")
	}

	defaults := config.DefaultHiddifyOptions()
	if defaults.EnableTun || defaults.FullTunnel {
		t.Fatal("macOS packet mode must not mutate global default settings")
	}
}

func TestPacketTunnelRejectsEmptyOrHugeSourceBeforeSetup(t *testing.T) {
	for _, input := range []string{"", "  \n ", strings.Repeat("x", config.MaxConfigBytes+1)} {
		if err := StartPacketTunnel(input); err == nil {
			t.Fatal("expected invalid packet tunnel source rejection")
		}
	}
}
