package paneluri

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func encodeAdditional(v any, compress bool) string {
	b, _ := json.Marshal(v)
	if compress {
		var buf bytes.Buffer
		w, _ := flate.NewWriter(&buf, flate.BestSpeed)
		w.Write(b)
		w.Close()
		b = buf.Bytes()
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func TestAdditionalPanelShares(t *testing.T) {
	sudoku := "sudoku://" + encodeAdditional(map[string]any{"h": "server.example", "p": 443, "k": "key", "a": "ascii", "e": "chacha20-poly1305", "x": true, "hm": "ws", "ht": true, "hh": "cdn.example", "hx": "on", "hy": "mask"}, false)
	flux := "openflux://v1/" + encodeAdditional(map[string]any{"name": "node", "secret": strings.Repeat("x", 32), "negotiate": true, "codec": "batched", "context": "session", "transports": []map[string]any{{"type": "direct", "dial": "server.example:8443", "priority": 2}, {"type": "boards", "url": "https://boards.yandex.ru/channel", "priority": 1}}}, true)
	fptn := "fptn:" + encodeAdditional(map[string]any{"version": 1, "username": "user", "password": "password", "servers": []map[string]any{{"name": "normal", "host": "server.example", "port": 443, "md5_fingerprint": strings.Repeat("01", 16)}}, "censored_zone_servers": []map[string]any{{"name": "alternate", "host": "alternate.example", "port": 8443, "md5_fingerprint": strings.Repeat("ab", 16)}}}, false)
	for _, test := range []struct {
		link, kind string
		count      int
	}{{sudoku, "sudoku", 1}, {flux, "openflux", 1}, {fptn, "fptn", 2}, {"pingtunnel://server.example?key=12345&encrypt=chacha20&encrypt_key=secret", "pingtunnel", 1}} {
		r, e := Parse(test.link)
		if e != nil {
			t.Fatal(e)
		}
		if len(r.Outbounds) != test.count || r.Outbounds[0]["type"] != test.kind {
			t.Fatalf("unexpected result: %s", test.kind)
		}
	}
	r, e := Parse(sudoku)
	if e != nil {
		t.Fatal(e)
	}
	if r.Outbounds[0]["enable_pure_downlink"] != false || r.Outbounds[0]["httpmask"].(map[string]any)["multiplex"] != "on" {
		t.Fatal("Sudoku options were lost")
	}
}
func TestAdditionalMalformedShares(t *testing.T) {
	for _, link := range []string{"sudoku://" + encodeAdditional(map[string]any{"h": "server.example", "p": 65536, "k": "private-secret"}, false), "fptn:" + encodeAdditional(map[string]any{"version": 1, "username": "private-secret", "password": "password", "servers": []map[string]any{{"host": "s.example", "port": 443, "md5_fingerprint": "invalid"}}}, false), "openflux://v2/payload", "pingtunnel://server.example:443?key=12", "pingtunnel://server.example?key=-1", "pingtunnel://server.example?encrypt=chacha20"} {
		_, e := Parse(link)
		if e == nil {
			t.Fatal("malformed share accepted")
		}
		if strings.Contains(e.Error(), "private-secret") {
			t.Fatal("credentials leaked in error")
		}
	}
	var buf bytes.Buffer
	w, _ := flate.NewWriter(&buf, flate.BestCompression)
	w.Write(bytes.Repeat([]byte("x"), maxAdditionalPayload+1))
	w.Close()
	if _, e := Parse("openflux://v1/" + base64.RawURLEncoding.EncodeToString(buf.Bytes())); e == nil {
		t.Fatal("compressed bomb accepted")
	}
}
