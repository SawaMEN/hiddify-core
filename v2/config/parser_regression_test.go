package config

import (
	"github.com/sagernet/sing-box/experimental/libbox"
	"testing"
)

func TestParserAcceptedJSONShapes(t *testing.T) {
	ctx := libbox.BaseContext(nil)
	for _, input := range []string{`{"type":"direct","tag":"direct"}`, `[{"type":"direct","tag":"direct"}]`, `{"outbounds":[{"type":"direct","tag":"direct"}]}`} {
		parsed, err := ParseConfig(ctx, &ReadOptions{Content: input}, false, DefaultHiddifyOptions(), false)
		if err != nil {
			t.Fatalf("%s: %v", input, err)
		}
		if len(parsed.Outbounds) != 1 || parsed.Outbounds[0].Tag != "direct" {
			t.Fatalf("lost outbound: %+v", parsed)
		}
	}
}
func TestParserPreservesFullConfig(t *testing.T) {
	parsed, err := ParseConfig(libbox.BaseContext(nil), &ReadOptions{Content: `{"outbounds":[{"type":"direct","tag":"direct"}],"route":{"final":"direct"}}`}, false, DefaultHiddifyOptions(), true)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Route == nil || parsed.Route.Final != "direct" {
		t.Fatal("route discarded")
	}
}
func TestParserRejectsTrailingJSON(t *testing.T) {
	_, err := ParseConfig(libbox.BaseContext(nil), &ReadOptions{Content: `{"type":"direct"} {"type":"direct"}`}, false, nil, false)
	if err == nil {
		t.Fatal("accepted trailing JSON")
	}
}

func TestReadContentRejectsOversizedInput(t *testing.T) {
	_, err := ReadContent(libbox.BaseContext(nil), &ReadOptions{Content: string(make([]byte, MaxConfigBytes+1))})
	if err == nil {
		t.Fatal("oversized content accepted")
	}
}
