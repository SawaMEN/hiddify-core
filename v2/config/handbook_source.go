package config

import (
	"net/url"
	"strings"
	"unicode"
)

func handbookURL(host, sites string) string {
	q := url.Values{"format": {"json"}}
	// Match the client's comma, semicolon and whitespace selection separators.
	// The provider expects repeated site= parameters rather than a joined value.
	for _, site := range strings.FieldsFunc(sites, func(c rune) bool {
		return c == ',' || c == ';' || unicode.IsSpace(c)
	}) {
		q.Add("site", strings.ToLower(site))
	}
	return "https://" + host + "/?" + q.Encode()
}
