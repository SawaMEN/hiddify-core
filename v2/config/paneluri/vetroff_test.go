package paneluri

import (
    "encoding/base64"
    "strings"
    "testing"
)

func TestVetrOFFShareLinkImportsAsNativeOutbound(t *testing.T) {
    payload := `{"v":1,"servers":["https://edge.example.com","https://backup.example.com"],"username":"alice","password":"private-secret","transport":"xhttp-stream-one","xhttp_path":"/connect","tls":"go"}`
    link := "vetroff://" + base64.RawURLEncoding.EncodeToString([]byte(payload))
    if !Handles("vetroff") { t.Fatal("VetrOFFProxy share links not registered") }
    decoded, err := Parse(link)
    if err != nil { t.Fatal(err) }
    if len(decoded.Outbounds) != 1 || decoded.Outbounds[0]["type"] != "vetroff" ||
        decoded.Outbounds[0]["link"] != link || decoded.Outbounds[0]["tag"] != "VetrOFFProxy · edge.example.com" {
        t.Fatal("VetrOFFProxy profile import lost its native type, link or safe display name")
    }
}

func TestVetrOFFShareLinkRejectsInvalidProfilesWithoutSecrets(t *testing.T) {
    for _, raw := range []string{
        `{"v":2,"servers":["https://edge.example.com"],"username":"alice","password":"private-secret"}`,
        `{"v":1,"servers":[],"username":"alice","password":"private-secret"}`,
        `{"v":1,"servers":["https://edge.example.com"],"username":"alice","password":"private-secret","transport":"unsupported"}`,
        `{"v":1,"servers":["https://edge.example.com"],"username":"alice","password":"private-secret","unknown":true}`,
        `{"v":1,"v":1,"servers":["https://edge.example.com"],"username":"alice","password":"private-secret"}`,
    } {
        uri := "vetroff://" + base64.RawURLEncoding.EncodeToString([]byte(raw))
        _, err := Parse(uri)
        if err == nil || strings.Contains(err.Error(), "private-secret") || strings.Contains(err.Error(), uri) {
            t.Fatal("invalid VetrOFFProxy profile was accepted or leaked credentials")
        }
    }
}
