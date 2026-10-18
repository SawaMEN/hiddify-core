package mobile

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/hiddify/hiddify-core/v2/config"
	"github.com/hiddify/hiddify-core/v2/hcore"
)

// packetTunnelSettings builds a private, macOS-only configuration profile.
// This intentionally does not alter Android's default profile preferences.
func packetTunnelSettings() *config.HiddifyOptions {
	settings := config.DefaultHiddifyOptions()
	settings.EnableTun = true
	settings.FullTunnel = true
	settings.StrictRoute = true
	settings.DisableLocalProxy = true
	settings.EnableClashApi = false
	settings.SetSystemProxy = false
	settings.AllowConnectionFromLAN = false
	settings.WifiVPNSharing = false
	settings.MixedPort = 0
	settings.TProxyPort = 0
	settings.RedirectPort = 0
	settings.DirectPort = 0
	settings.MTU = 1500
	settings.EnableFullConfig = false
	// The built-in remote and direct resolvers must not use plaintext UDP/53.
	// Upstream DoH traffic is still subject to actual-network leak testing.
	settings.RemoteDnsAddress = "https://1.1.1.1/dns-query"
	settings.DirectDnsAddress = "https://1.1.1.1/dns-query"
	return settings
}

// StartPacketTunnel enables a real sing-box TUN *only* for callers explicitly
// using the macOS Network Extension. Ordinary mobile.Start remains untouched.
//
// It accepts a JSON proxy/outbound profile; the fork's own ParseBuildConfig
// generates the full TUN, routing, DNS and outbound policy. It does not
// launch a second RPC server or a local proxy listener.
func StartPacketTunnel(configContent string) error {
	if strings.TrimSpace(configContent) == "" || len(configContent) > config.MaxConfigBytes {
		return errors.New("packet tunnel profile must be non-empty and at most 8 MiB")
	}
	bytes, err := json.Marshal(packetTunnelSettings())
	if err != nil {
		return err
	}
	_, err = hcore.ChangeHiddifySettings(
		&hcore.ChangeHiddifySettingsRequest{HiddifySettingsJson: string(bytes)}, false,
	)
	if err != nil {
		return err
	}
	return Start("", configContent)
}
