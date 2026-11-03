package paneluri

import (
    "errors"
    "net/url"

    vetroff "github.com/SawaMEN/VetroffProxy"
)

// vetroffLink accepts the canonical, versioned VetroffProxy share format.
// Keep credentials in the private outbound option; never put them in tags
// or parser errors.
func vetroffLink(link string) (Result, error) {
    config, err := vetroff.ClientConfigFromLink(link)
    if err != nil {
        return Result{}, errors.New("vetroff: invalid connection link")
    }
    name := "VetrOFFProxy"
    if parsed, err := url.Parse(config.ServerURL); err == nil && parsed.Hostname() != "" {
        name += " · " + parsed.Hostname()
    }
    return Result{Outbounds: []map[string]any{{
        "type": "vetroff",
        "tag": name,
        "link": link,
    }}}, nil
}
