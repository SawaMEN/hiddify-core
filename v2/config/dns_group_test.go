package config

import (
	"net/netip"
	"slices"
	"testing"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/experimental/libbox"
	"github.com/sagernet/sing-box/option"
)

func TestDNSGroupAddresses(t *testing.T) {
	cases := map[string][]string{
		"udp://1.1.1.1": {"udp://1.1.1.1", "tcp://1.1.1.1", "tls://1.1.1.1", "udp://8.8.8.8", "tcp://8.8.8.8", "tls://8.8.8.8"},
		// selected is already 8.8.8.8: fall back to 1.1.1.1
		"8.8.8.8":                      {"udp://8.8.8.8", "tcp://8.8.8.8", "tls://8.8.8.8", "udp://1.1.1.1", "tcp://1.1.1.1", "tls://1.1.1.1"},
		"https://dns.google/dns-query": {"https://dns.google/dns-query", "udp://dns.google", "tcp://dns.google", "tls://dns.google", "udp://8.8.8.8", "tcp://8.8.8.8", "tls://8.8.8.8"},
		// duplicates of the selected address are dropped
		"tcp://9.9.9.9": {"tcp://9.9.9.9", "udp://9.9.9.9", "tls://9.9.9.9", "udp://8.8.8.8", "tcp://8.8.8.8", "tls://8.8.8.8"},
		// no host to derive variants from
		"local":                        {"local", "udp://8.8.8.8", "tcp://8.8.8.8", "tls://8.8.8.8"},
		"udp://[2606:4700:4700::1111]": {"udp://[2606:4700:4700::1111]", "tcp://[2606:4700:4700::1111]", "tls://[2606:4700:4700::1111]", "udp://8.8.8.8", "tcp://8.8.8.8", "tls://8.8.8.8"},
	}
	for selected, want := range cases {
		if got := dnsGroupAddresses(selected); !slices.Equal(got, want) {
			t.Errorf("%s:\n got %v\nwant %v", selected, got, want)
		}
	}
}

func TestBuildConfigDNSGroups(t *testing.T) {
	opts := DefaultHiddifyOptions()
	opts.RemoteDnsAddress = "udp://1.1.1.1"
	opts.DirectDnsAddress = "udp://9.9.9.9"
	options, err := BuildConfig(libbox.BaseContext(nil), opts, &ReadOptions{Content: chainTestProfile})
	if err != nil {
		t.Fatalf("BuildConfig: %v", err)
	}
	if err := libbox.CheckConfigOptions(options); err != nil {
		t.Fatalf("sing-box rejected the config: %v", err)
	}
	servers := map[string]option.DNSServerOptions{}
	for _, server := range options.DNS.Servers {
		servers[server.Tag] = server
	}
	if options.DNS.Final != DNSMultiRemoteTag {
		t.Fatalf("DNS final = %q, want the remote group", options.DNS.Final)
	}
	for tag, want := range map[string]string{DNSMultiRemoteTag: DNSRemoteTag, DNSMultiDirectTag: DNSDirectTag} {
		group, ok := servers[tag]
		if !ok || group.Type != C.DNSTypeGroup {
			t.Fatalf("%s: group not found", tag)
		}
		g := group.Options.(*option.GroupDNSServerOptions)
		if g.Mode != C.DNSGroupModeSequential {
			t.Fatalf("%s: mode %q, want sequential", tag, g.Mode)
		}
		if len(g.IgnoreRanges) != 1 || netip.Prefix(g.IgnoreRanges[0]).String() != dnsGroupIgnoreRange {
			t.Fatalf("%s: ignore ranges %v", tag, g.IgnoreRanges)
		}
		// TCP is prioritized over UDP by the current group builder. The
		// selected resolver remains a member rather than forcibly first.
		if len(g.Servers) != 6 || !slices.Contains(g.Servers, want) {
			t.Fatalf("%s: members %v must contain selected %s and six candidates", tag, g.Servers, want)
		}
		if tag == DNSMultiRemoteTag && servers[g.Servers[0]].Type != C.DNSTypeTCP {
			t.Fatalf("%s: first member %s should be TCP for fast failure", tag, g.Servers[0])
		}
		seen := map[string]bool{}
		for _, member := range g.Servers {
			if _, found := servers[member]; !found || seen[member] {
				t.Fatalf("%s: missing or duplicate member %q", tag, member)
			}
			seen[member] = true
		}
	}
	// remote members dial through the main selector
	if d := servers[DNSMultiRemoteTag+"-1"].Options.(*option.RemoteDNSServerOptions).Detour; d != OutboundSelectTag {
		t.Fatalf("remote tcp member detour = %q, want %q", d, OutboundSelectTag)
	}
}

// Remote DoH bootstrap resolves through dns-direct, while dns-direct resolves
// through dns-local. Keep the dependency order stable for sing-box startup.
func TestDNSBootstrapResolverIsDeclaredFirst(t *testing.T) {
    options, err := BuildConfig(libbox.BaseContext(nil), DefaultHiddifyOptions(), &ReadOptions{Content: chainTestProfile})
    if err != nil { t.Fatal(err) }
    position := map[string]int{}
    for i, server := range options.DNS.Servers { position[server.Tag] = i }
    for _, dependency := range [][2]string{
        {DNSLocalTag, DNSDirectTag},
        {DNSDirectTag, DNSRemoteTag},
        {DNSDirectTag, DNSRemoteTagFallback},
        {DNSDirectTag, DNSRemoteNoWarpTag},
        {DNSDirectTag, DNSTricksDirectTag},
        {DNSDirectTag, DNSMultiRemoteTag},
    } {
        a, foundA := position[dependency[0]]
        b, foundB := position[dependency[1]]
        if !foundA || !foundB || a >= b {
            t.Errorf("DNS dependency %q must be available before %q: positions %d and %d", dependency[0], dependency[1], a, b)
        }
    }
}
