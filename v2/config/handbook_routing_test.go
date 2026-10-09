package config

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/experimental/libbox"
	"github.com/sagernet/sing-box/option"
)

func TestHandbookReplacesBuiltInPolicy(t *testing.T) {
	t.Chdir(t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(handbookFixture)) }))
	defer server.Close()
	// Seed the validated cache using the same loader. Keep production hosts fixed.
	for _, host := range []string{"iplist.my-handbook.ru", "ru-iplist.my-handbook.ru"} {
		body, err := loadHandbook(context.Background(), server.URL+"/"+host)
		if err != nil {
			t.Fatal(err)
		}
		if err = writeHandbookTestCache(handbookURL(host, ""), body); err != nil {
			t.Fatal(err)
		}
	}
	h := DefaultHiddifyOptions()
	h.HandbookRouting = true
	h.HandbookProxy = true
	h.HandbookDirect = true
	h.Region = "ru"
	h.PrivacyRoutingMode = "ru-bypass"
	h.PrivacyDirectDomains = []string{"bank.example"}
	built, err := BuildConfig(libbox.BaseContext(nil), h, &ReadOptions{Content: legacyDNSOutboundProfile})
	if err != nil {
		t.Fatal(err)
	}
	if hasDomainRoute(built.Route.Rules, "bank.example", OutboundDirectTag) || hasDomainRoute(built.Route.Rules, ".ru", OutboundDirectTag) {
		t.Fatal("built-in routing survived alternative mode")
	}
	var tags []string
	for _, rule := range built.Route.Rules {
		tags = append(tags, rule.DefaultOptions.RuleSet...)
	}
	if len(tags) < 4 || tags[0] != "handbook-proxy-domains" || tags[2] != "handbook-direct-domains" {
		t.Fatalf("unexpected priority: %v", tags)
	}
	if h.Region != "ru" || h.PrivacyRoutingMode != "ru-bypass" {
		t.Fatal("building config mutated stored preferences")
	}
	h.FullTunnel = true
	h.EnableTun = true
	full, err := BuildConfig(libbox.BaseContext(nil), h, &ReadOptions{Content: legacyDNSOutboundProfile})
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range full.Route.RuleSet {
		for _, tag := range rule.Tag {
			if tag == "handbook-proxy-domains" {
				t.Fatal("alternative bypass must not weaken full tunnel")
			}
		}
	}
}

func TestWifiSharingListenerAndRootRedirect(t *testing.T) {
	h := DefaultHiddifyOptions()
	h.WifiVPNSharing = true
	h.EnableTun = true
	h.DisableLocalProxy = true
	h.LanSharingPassword = "test-password"
	var built option.Options
	setInbound(&built, h)
	mixed := false
	for _, inbound := range built.Inbounds {
		if inbound.Type == C.TypeMixed {
			mixed = true
			options := inbound.Options.(*option.HTTPMixedInboundOptions)
			if netip.Addr(*options.Listen).String() != "0.0.0.0" && netip.Addr(*options.Listen).String() != "::" {
				t.Fatal("sharing listener is not reachable over LAN")
			}
			if len(options.Users) != 1 || options.Users[0].Password != "test-password" || options.SetSystemProxy {
				t.Fatal("sharing credentials or system proxy policy lost")
			}
		}
	}
	if !mixed {
		t.Fatal("sharing must override hidden/disabled mixed listener")
	}
	if os.Getuid() == 0 {
		h.PrivacyRoot = true
		h.PrivacyRootTable = 12345
		h.DisableLocalProxy = true
		built = option.Options{}
		setInbound(&built, h)
		for _, inbound := range built.Inbounds {
			if inbound.Type == C.TypeMixed {
				t.Fatal("root sharing must not open a LAN proxy")
			}
			if inbound.Type == C.TypeTun && !inbound.Options.(*option.TunInboundOptions).AutoRedirect {
				t.Fatal("root hotspot forwarding is disabled")
			}
		}
	}
}

func writeHandbookTestCache(source string, body []byte) error {
	sum := sha256.Sum256([]byte(source))
	path := filepath.Join("data", "handbook", hex.EncodeToString(sum[:])+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0600)
}
