package paneluri

import (
	"encoding/base64"
	"strings"
	"testing"

	pb "github.com/enfein/mieru/v3/pkg/appctl/appctlpb"
	"google.golang.org/protobuf/proto"
)

func TestSnellPanelUserKey(t *testing.T) {
	for _, version := range []string{"4", "6"} {
		r, err := Parse("snell://shared%2Bkey@[2001:db8::1]:8443?version=" + version + "&userkey=user%2Bkey&mode=unshaped&obfs=http&obfs-host=example.org#Panel")
		if err != nil {
			t.Fatal(err)
		}
		o := r.Outbounds[0]
		if o["psk"] != "shared+key" || o["userkey"] != "user+key" || o["server"] != "2001:db8::1" {
			t.Fatalf("Snell options lost: %v", o)
		}
		if version == "6" && o["mode"] != "unshaped" {
			t.Fatal("Snell v6 mode lost")
		}
		if version == "4" && o["obfs_host"] != "example.org" {
			t.Fatal("Snell obfuscation host lost")
		}
	}
}

func TestMieruPanelFullConfiguration(t *testing.T) {
	cfg := &pb.ClientConfig{ActiveProfile: proto.String("panel"), Profiles: []*pb.ClientProfile{{ProfileName: proto.String("panel"),
		User: &pb.User{Name: proto.String("name"), Password: proto.String("password")}, Mtu: proto.Int32(1280),
		HandshakeMode: pb.HandshakeMode_HANDSHAKE_NO_WAIT.Enum(), Multiplexing: &pb.MultiplexingConfig{Level: pb.MultiplexingLevel_MULTIPLEXING_HIGH.Enum()},
		Servers: []*pb.ServerEndpoint{{DomainName: proto.String("server.example"), PortBindings: []*pb.PortBinding{
			{Protocol: pb.TransportProtocol_TCP.Enum(), Port: proto.Int32(443)}, {Protocol: pb.TransportProtocol_UDP.Enum(), PortRange: proto.String("10000-10010")}}}}}}}
	data, err := proto.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Parse("mieru://" + base64.StdEncoding.EncodeToString(data))
	if err != nil {
		t.Fatal(err)
	}
	o := r.Outbounds[0]
	if o["server"] != "server.example" || o["mtu"] != int32(1280) || o["handshake_mode"] != "HANDSHAKE_NO_WAIT" || o["multiplexing"] != "MULTIPLEXING_HIGH" {
		t.Fatal("Mieru profile options lost")
	}
	bindings := o["portBindings"].([]map[string]any)
	if len(bindings) != 2 || bindings[1]["portRange"] != "10000-10010" {
		t.Fatal("Mieru transport bindings lost")
	}
}

func TestMieruRepeatedQueryValues(t *testing.T) {
	r, err := Parse("mierus://name:pass@server.example?port=443&protocol=TCP&port=10000-10010&protocol=UDP&handshake-mode=HANDSHAKE_NO_WAIT&mtu=1280")
	if err != nil {
		t.Fatal(err)
	}
	o := r.Outbounds[0]
	if o["handshake_mode"] != "HANDSHAKE_NO_WAIT" || len(o["portBindings"].([]map[string]any)) != 2 || o["mtu"] != 1280 {
		t.Fatal("Mieru query options lost")
	}
}

func TestAmneziaPanelVPNLink(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	text := "[Interface]\nPrivateKey = " + key + "\nAddress = 10.8.0.2/32, fd00::2/128\nMTU = 1280\nJc = 4\nS3 = 20\nH1 = 123-456\nI1 = <b 0x01>\n[Peer]\nPublicKey = " + key + "\nEndpoint = [2001:db8::1]:51820\nAllowedIPs = 0.0.0.0/0, ::/0\nPersistentKeepalive = 25\n[Peer]\nPublicKey = " + key + "\nEndpoint = server.example:51821\nAllowedIPs = 10.9.0.0/16\n"
	r, err := Parse("vpn://" + base64.RawURLEncoding.EncodeToString([]byte(text)))
	if err != nil {
		t.Fatal(err)
	}
	o := r.Endpoints[0]
	if o["type"] != "awg" || o["mtu"] != 1280 || len(o["peers"].([]map[string]any)) != 2 {
		t.Fatal("Amnezia interface or peers lost")
	}
	awg := o["awg"].(map[string]any)
	if awg["s3"] != 20 || awg["h1"] != "123-456" || awg["i1"] != "<b 0x01>" {
		t.Fatal("AWG v2 options lost")
	}
	if o["useIntegratedTun"] != false {
		t.Fatal("Android import requested a second system TUN")
	}
}

func TestAmneziaV3Options(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	text := "[Interface]\nPrivateKey = " + key + "\nAddress = 10.8.0.2/32\nS1 = 20\nS2 = 20\nS3 = 20\nS4 = 20\nHeaderProtectionKey = " + key + "\nContentPaddingAddition = 5-10\nRekeyAfterTime = 90-120\nRandomTrailers = on\nDisableCookies = off\n[Peer]\nPublicKey = " + key + "\nEndpoint = server.example:51820\nAllowedIPs = 0.0.0.0/0\n"
	r, err := Parse("vpn://" + base64.RawURLEncoding.EncodeToString([]byte(text)))
	if err != nil {
		t.Fatal(err)
	}
	awg := r.Endpoints[0]["awg"].(map[string]any)
	if awg["header_protection_key"] != key || awg["content_padding_addition"] != "5-10" || awg["rekey_after_time"] != "90-120" || awg["random_trailers"] != true || awg["disable_cookies"] != false {
		t.Fatal("AWG v3 options lost")
	}
}

func trustLink(fields map[byte][]byte, addresses ...string) string {
	data := []byte{}
	put := func(tag byte, value []byte) {
		data = append(data, tag)
		if len(value) < 64 {
			data = append(data, byte(len(value)))
		} else {
			data = append(data, byte(len(value)>>8)|0x40, byte(len(value)))
		}
		data = append(data, value...)
	}
	for tag, value := range fields {
		put(tag, value)
	}
	for _, value := range addresses {
		put(2, []byte(value))
	}
	return "tt://?" + base64.RawURLEncoding.EncodeToString(data)
}

func TestTrustTunnelNativeDeepLink(t *testing.T) {
	fields := map[byte][]byte{0: {2}, 1: []byte("vpn.example"), 3: []byte("sni.example"), 5: []byte("user"), 6: []byte("password"), 9: {2}, 12: []byte(strings.Repeat("n", 80)), 63: []byte("future")}
	r, err := Parse(trustLink(fields, "[2001:db8::1]:443", "server.example:8443"))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Outbounds) != 2 {
		t.Fatal("TrustTunnel alternative addresses lost")
	}
	o := r.Outbounds[0]
	if o["quic"] != true || o["username"] != "user" || o["tls"].(map[string]any)["server_name"] != "sni.example" {
		t.Fatal("TrustTunnel credentials or transport lost")
	}
	if o["tls"].(map[string]any)["insecure"] == true {
		t.Fatal("certificate verification was disabled")
	}
}

func TestMalformedSharesAndSensitiveErrors(t *testing.T) {
	for _, link := range []string{
		"snell://secret@server.example:65536?version=4", "snell://secret@server.example:443?version=5",
		"snell://secret@server.example:443?version=6&mode=bad", "vpn://invalid-secret", "mieru://invalid-secret",
		"mierus://user:secret@server.example?port=70000&protocol=TCP", "mierus://user:secret@server.example?port=443&protocol=UDP&handshake-mode=bad",
		"tt://?AQ", "tt://?", trustLink(map[byte][]byte{0: {3}}), trustLink(map[byte][]byte{14: []byte("http://secret")}),
		trustLink(map[byte][]byte{0: {2}, 1: []byte("vpn.example"), 5: []byte("user"), 6: []byte("secret"), 10: {1}}, "server.example:443"),
	} {
		if _, err := Parse(link); err == nil {
			t.Errorf("accepted malformed share scheme %s", strings.SplitN(link, ":", 2)[0])
		} else if strings.Contains(err.Error(), "secret") {
			t.Fatal("credentials leaked in parser error")
		}
	}
}

func FuzzTrustTunnel(f *testing.F) {
	f.Add([]byte{1, 2, 'a', 'b'})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) <= 4096 {
			_, _ = Parse("tt://?" + base64.RawURLEncoding.EncodeToString(data))
		}
	})
}
