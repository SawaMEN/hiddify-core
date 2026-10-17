package paneluri

import (
    "strings"
    "testing"
)

func TestOLCRTCShareTransportParameters(t *testing.T) {
    const key = "d823fa01cb3e0609b67322f7cf984c4ee2e4ce2e294936fc24ef38c9e59f4799"
    cases := []struct {
        link, provider, transport, room, tag, field string
        value any
    }{
        {"olcrtc://jitsi?datachannel@https://meet.jit.si/myroom#" + key + "$Jitsi / primary",
            "jitsi", "datachannel", "https://meet.jit.si/myroom", "Jitsi / primary", "", nil},
        {"olcrtc://jitsi?datachannel@https://meet.jit.si/room@group#" + key,
            "jitsi", "datachannel", "https://meet.jit.si/room@group", "olcRTC", "", nil},
        {"olcrtc://wbstream?vp8channel<vp8-fps=60&vp8-batch=64>@room1#" + key,
            "wbstream", "vp8channel", "room1", "olcRTC", "vp8_fps", 60},
        {"olcrtc://wbstream?seichannel<fps=30&batch=64&frag=900&ack-ms=2000>@room1#" + key,
            "wbstream", "seichannel", "room1", "olcRTC", "sei_fragment", 900},
        {"olcrtc://telemost?videochannel<video-w=1080&video-h=1080&video-codec=tile>@room1#" + key,
            "telemost", "videochannel", "room1", "olcRTC", "video_codec", "tile"},
    }
    for _, tc := range cases {
        result, err := Parse(tc.link)
        if err != nil { t.Fatalf("valid link: %v", err) }
        if len(result.Outbounds) != 1 { t.Fatal("missing olcrtc outbound") }
        got := result.Outbounds[0]
        if got["type"] != "olcrtc" || got["provider"] != tc.provider || got["transport"] != tc.transport ||
            got["room_id"] != tc.room || got["key"] != key || got["tag"] != tc.tag {
            t.Fatal("olcrtc share fields do not match")
        }
        if tc.field != "" && got[tc.field] != tc.value { t.Fatal("transport field lost") }
    }
}

func TestOLCRTCRejectsInvalidOrUnsupportedLinksWithoutKeyLeaks(t *testing.T) {
    const key = "d823fa01cb3e0609b67322f7cf984c4ee2e4ce2e294936fc24ef38c9e59f4799"
    for _, link := range []string{
        "olcrtc://jitsi?datachannel@room#short",
        "olcrtc://unknown?datachannel@room#" + key,
        "olcrtc://jitsi?unknown@room#" + key,
        "olcrtc://telemost?datachannel@room#" + key,
        "olcrtc://telemost?seichannel@room#" + key,
        "olcrtc://wbstream?datachannel@room#" + key,
        "olcrtc://jitsi?datachannel<fps=60>@room#" + key,
        "olcrtc://jitsi?vp8channel<vp8-fps=30&vp8-fps=45>@room#" + key,
        "olcrtc://jitsi?vp8channel<vp8-fps=-30>@room#" + key,
        "olcrtc://jitsi?vp8channel<vp8-fps=30>@room#" + key + "\nSECRET",
        "olcrtc://jitsi?datachannel@" + strings.Repeat("A", 2049) + "#" + key,
    } {
        _, err := Parse(link)
        if err == nil { t.Fatal("invalid olcrtc link accepted") }
        if strings.Contains(err.Error(), key) || strings.Contains(err.Error(), "SECRET") { t.Fatal("sensitive information in parse error") }
    }
}
