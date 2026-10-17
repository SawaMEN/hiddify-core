package config

import (
    "strings"
    "testing"

    "github.com/sagernet/sing-box/experimental/libbox"
    "github.com/sagernet/sing-box/option"
)

// Ensure UI import and native ProfileValidation share the same real parser.
// No network or WebRTC endpoint is contacted by this unit test.
func TestOlcRTCRealConfigParsing(t *testing.T) {
    const key = "d823fa01cb3e0609b67322f7cf984c4ee2e4ce2e294936fc24ef38c9e59f4799"
    const room = "https://meet.jit.si/myroom"
    raw := "olcrtc://jitsi?datachannel@" + room + "#" + key + "$Test"
    parsed, err := ParseConfig(libbox.BaseContext(nil), &ReadOptions{Content: raw}, false, nil, false)
    if err != nil { t.Fatalf("cannot parse real olcrtc profile: %v", err) }
    if len(parsed.Outbounds) != 1 || parsed.Outbounds[0].Type != "olcrtc" {
        t.Fatal("olcrtc outbound not preserved through core parser")
    }
    opts, ok := parsed.Outbounds[0].Options.(*option.OlcRTCOutboundOptions)
    if !ok || opts.Provider != "jitsi" || opts.Transport != "datachannel" || opts.RoomID != room || opts.Key != key {
        t.Fatal("olcrtc options lost or changed by the core parser")
    }
    if !strings.HasPrefix(parsed.Outbounds[0].Tag, "Test §") {
        t.Fatal("metadata label was not preserved")
    }
}

func TestOlcRTCFullJSONProfile(t *testing.T) {
    const key = "d823fa01cb3e0609b67322f7cf984c4ee2e4ce2e294936fc24ef38c9e59f4799"
    raw := `{"outbounds":[{"type":"olcrtc","tag":"webrtc","provider":"jitsi","transport":"datachannel","room_id":"https://meet.jit.si/myroom","key":"` + key + `"}]}`
    parsed, err := ParseConfig(libbox.BaseContext(nil), &ReadOptions{Content: raw}, false, nil, false)
    if err != nil { t.Fatalf("cannot parse full JSON profile: %v", err) }
    if len(parsed.Outbounds) != 1 || parsed.Outbounds[0].Type != "olcrtc" { t.Fatal("JSON olcrtc outbound lost") }
}
