package hcore

import (
	"encoding/json"
	"github.com/hiddify/hiddify-core/v2/config"
)

// ApplyDevicePrivacy enforces current Android device policy before an Always-on start,
// even when Flutter is not running and the last saved profile has older settings.
func ApplyDevicePrivacy(fullTunnel, hideProxy, hideClash, disableSystemProxy, encryptedDNS bool) {
	static.settingsLock.Lock()
	defer static.settingsLock.Unlock()
	if static.HiddifyOptions == nil {
		static.HiddifyOptions = config.DefaultHiddifyOptions()
	}
	next := *static.HiddifyOptions
	next.FullTunnel = fullTunnel && next.EnableTun
	next.DisableLocalProxy = hideProxy && next.EnableTun
	if hideClash {
		next.EnableClashApi = false
	}
	if disableSystemProxy && next.EnableTun {
		next.SetSystemProxy = false
	}
	if next.FullTunnel {
		next.StrictRoute = true
		next.BypassLAN = false
	}
	if encryptedDNS || (next.FullTunnel && next.RemoteDnsAddress == "local") {
		next.RemoteDnsAddress = "https://1.1.1.1/dns-query"
	}
	static.HiddifyOptions = &next
}

func ApplyRegionalPrivacy(policyJSON string) error {
	static.settingsLock.Lock()
	defer static.settingsLock.Unlock()
	if static.HiddifyOptions == nil {
		static.HiddifyOptions = config.DefaultHiddifyOptions()
	}
	if err := config.SetNetworkPrivacyPolicyJSON(policyJSON); err != nil {
		return err
	}
	next := *static.HiddifyOptions
	if err := json.Unmarshal([]byte(policyJSON), &next); err != nil {
		return err
	}
	static.HiddifyOptions = &next
	return nil
}
