package config

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/sagernet/sing-box/experimental/libbox"
	"github.com/sagernet/sing-box/option"
)

func TestPanelSnellRawAndBase64(t *testing.T) {
	links := "# profile-title: panel\nsnell://psk@server.example:443?version=4&userkey=per-user&obfs=http&obfs-host=www.example\ntrojan://password@server.example:8443?security=tls#Second"
	for _, text := range []string{links, base64.StdEncoding.EncodeToString([]byte(links))} {
		parsed, err := ParseConfig(libbox.BaseContext(nil), &ReadOptions{Content: text}, false, nil, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(parsed.Outbounds) != 2 {
			t.Fatal("a mixed subscription lost an outbound")
		}
		snell := parsed.Outbounds[0].Options.(*option.SnellOutboundOptions)
		if snell.UserKey != "per-user" || snell.ObfsOptions.ObfsHost != "www.example" {
			t.Fatal("Snell user key or obfuscation lost")
		}
	}
}

func TestPanelMixedSubscriptionRejectsUnsupportedEntry(t *testing.T) {
	_, err := ParseConfig(libbox.BaseContext(nil), &ReadOptions{Content: "trojan://password@server.example:443\nsudoku://secret"}, false, nil, false)
	if err == nil || strings.Contains(err.Error(), "secret") || !strings.Contains(err.Error(), "sudoku") {
		t.Fatal("unsupported entry was dropped or error leaked credentials")
	}
}

func TestPanelPreservesVLESSEncryptionAndNaiveTransport(t *testing.T) {
	parsed, err := parseKnownLink("vless://00000000-0000-0000-0000-000000000001@server.example:443?encryption=mlkem-test&type=tcp", "vless")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Outbounds[0].Options.(*option.VLESSOutboundOptions).Encryption != "mlkem-test" {
		t.Fatal("VLESS encryption parameter dropped")
	}
	parsed, err = parseKnownLink("naive://user:password@server.example:443?quic=0", "naive")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Outbounds[0].Options.(*option.NaiveOutboundOptions).QUIC {
		t.Fatal("quic=0 enabled QUIC")
	}
	parsed, err = parseKnownLink("naive+quic://user:password@server.example:443?quic_congestion_control=bbr", "naive+quic")
	if err != nil {
		t.Fatal(err)
	}
	naive := parsed.Outbounds[0].Options.(*option.NaiveOutboundOptions)
	if !naive.QUIC || naive.QUICCongestionControl != "bbr" {
		t.Fatal("Naive QUIC settings dropped")
	}
}

func TestPanelConfigurationArrayNamespacesDetours(t *testing.T) {
	text := `[
	{"outbounds":[{"type":"shadowsocks","tag":"proxy","server":"one.example","server_port":443,"method":"aes-128-gcm","password":"password","detour":"tls"},{"type":"shadowtls","tag":"tls","server":"one.example","server_port":443,"version":3,"password":"tls-password","tls":{"enabled":true,"server_name":"one.example"}},{"type":"direct","tag":"direct"}],"route":{"final":"proxy"}},
	{"outbounds":[{"type":"trojan","tag":"proxy","server":"two.example","server_port":443,"password":"password","tls":{"enabled":true}},{"type":"direct","tag":"direct"}],"route":{"final":"proxy"}}
	]`
	parsed, err := ParseConfig(libbox.BaseContext(nil), &ReadOptions{Content: text}, false, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Outbounds) != 5 {
		t.Fatal("configuration array lost proxies")
	}
	ss := parsed.Outbounds[0].Options.(*option.ShadowsocksOutboundOptions)
	if parsed.Outbounds[0].Tag != "panel-1/proxy" || ss.Detour != "panel-1/tls" || parsed.Outbounds[3].Tag != "panel-2/proxy" {
		t.Fatal("proxy tags or detours collide")
	}
	_, err = ParseConfig(libbox.BaseContext(nil), &ReadOptions{Content: text}, false, nil, true)
	if err == nil {
		t.Fatal("array silently discarded incompatible routing policies in full-config mode")
	}
}

func TestPanelEndpointArrayIsNotOutboundArray(t *testing.T) {
	// Check options parsing, without starting the endpoint or requiring an OS TUN.
	parsed, err := parseConfigContent(libbox.BaseContext(nil), []byte(`[{"outbounds":[{"type":"direct","tag":"direct"}],"endpoints":[{"type":"masque-client","tag":"proxy","server":"server.example","server_port":443,"username":"user","password":"password","version":2,"tls":{"enabled":true}}]}]`), false, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Endpoints) != 1 || parsed.Endpoints[0].Tag != "panel-1/proxy" {
		t.Fatal("MASQUE endpoint was not preserved")
	}
}
