package paneluri

import (
    "encoding/hex"
    "errors"
    "strconv"
    "strings"
)

// olcRTC v1 uses '#' to delimit its 32-byte encryption key and '$' for
// optional UI metadata. It is NOT a conventional URL: url.Parse loses data.
func olcRTCLink(link string) (Result, error) {
    invalid := func(message string) (Result, error) { return Result{}, errors.New("olcrtc: " + message) }
    if len(link) > 16384 || !strings.HasPrefix(strings.ToLower(link), "olcrtc://") {
        return invalid("invalid share link")
    }
    if strings.ContainsAny(link, "\x00\r\n") { return invalid("invalid control character") }
    body := link[len("olcrtc://"):]
    if p := strings.IndexByte(body, '$'); p >= 0 {
        body = body[:p]
    }
    hash := strings.LastIndexByte(body, '#')
    if hash <= 0 { return invalid("missing encryption key") }
    key := body[hash+1:]
    decoded, err := hex.DecodeString(key)
    if err != nil || len(decoded) != 32 { return invalid("key must be a 64-character hexadecimal value") }
    route := body[:hash]
    at := strings.LastIndexByte(route, '@')
    if at <= 0 || at == len(route)-1 { return invalid("missing room identifier") }
    room := route[at+1:]
    if len(room) > 2048 { return invalid("room identifier is too long") }
    header := route[:at]
    question := strings.IndexByte(header, '?')
    if question <= 0 || question == len(header)-1 { return invalid("missing provider or transport") }
    provider := strings.ToLower(header[:question])
    switch provider {
    case "jitsi", "telemost", "wbstream":
    default: return invalid("unsupported provider")
    }
    transportSpec := strings.ToLower(header[question+1:])
    transport, payload, withParams := strings.Cut(transportSpec, "<")
    allowed := map[string]map[string]string{
        "datachannel": {},
        "vp8channel": {"vp8-fps": "vp8_fps", "vp8-batch": "vp8_batch"},
        "seichannel": {"fps": "sei_fps", "batch": "sei_batch", "frag": "sei_fragment", "ack-ms": "sei_ack_timeout_ms"},
        "videochannel": {
            "video-w": "video_width", "video-h": "video_height", "video-fps": "video_fps",
            "video-codec": "video_codec", "video-qr-size": "video_qr_size",
            "video-qr-recovery": "video_qr_recovery", "video-tile-module": "video_tile_module",
            "video-tile-rs": "video_tile_rs",
        },
    }
    fields, ok := allowed[transport]
    if !ok { return invalid("unsupported transport") }
    if provider == "telemost" && (transport == "datachannel" || transport == "seichannel") {
        return invalid("transport unavailable for Telemost")
    }
    if provider == "wbstream" && transport == "datachannel" {
        return invalid("WB Stream requires a moderator token for datachannel")
    }
    out := map[string]any{
        "type": "olcrtc", "tag": "olcRTC", "provider": provider,
        "transport": transport, "room_id": room, "key": strings.ToLower(key),
    }
    if _, label, found := strings.Cut(link, "$"); found && strings.TrimSpace(label) != "" {
        // The optional label is after '$'; never include the key in a tag.
        if len(label) > 256 { return invalid("label is too long") }
        out["tag"] = strings.TrimSpace(label)
    }
    if withParams {
        if len(payload) < 2 || payload[len(payload)-1] != '>' || strings.ContainsAny(payload[:len(payload)-1], "<>") {
            return invalid("invalid transport parameters")
        }
        params := payload[:len(payload)-1]
        for _, pair := range strings.Split(params, "&") {
            name, value, found := strings.Cut(pair, "=")
            field, allowed := fields[name]
            if !found || !allowed || value == "" { return invalid("invalid transport parameter") }
            if _, duplicated := out[field]; duplicated { return invalid("duplicate transport parameter") }
            if len(value) > 64 { return invalid("transport parameter is too long") }
            if name == "video-codec" || name == "video-qr-recovery" {
                out[field] = value
            } else {
                n, e := strconv.Atoi(value)
                if e != nil || n > 1000000 || n < 0 || (n == 0 && name != "video-qr-size" && name != "video-tile-rs") {
                    return invalid("invalid numeric parameter")
                }
                out[field] = n
            }
        }
    }
    return Result{Outbounds: []map[string]any{out}}, nil
}
