package config

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const handbookMaxBytes = 32 << 20

// Local corrections complement the selected provider service, including cached exports.
// They are never sent as site= identifiers or applied to unrelated services.
var handbookExtraDomains = map[string][]string{
	"repo":        {"docker.io"},
	"pornhub.com": {"phncdn.com", "pornhub.org"},
}

var handbookCacheLock sync.Mutex
var handbookClient = &http.Client{Timeout: 12 * time.Second}

type handbookPortal struct {
	Domains []string `json:"domains"`
	IPv4    []string `json:"ip4"`
	IPv6    []string `json:"ip6"`
	CIDR4   []string `json:"cidr4"`
	CIDR6   []string `json:"cidr6"`
}

// Preserve only validated provider data. Error pages/JSON must never replace a
// working cache or silently turn a selected routing policy into an empty one.
func parseHandbook(body []byte) ([]string, []string, error) {
	var catalogue map[string]handbookPortal
	if err := json.Unmarshal(body, &catalogue); err != nil {
		return nil, nil, err
	}
	domains, prefixes := map[string]bool{}, map[string]bool{}
	for id, portal := range catalogue {
		serviceDomains := append(append([]string{}, portal.Domains...), handbookExtraDomains[strings.ToLower(id)]...)
		for _, value := range serviceDomains {
			value = strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(value), "*."), "."))
			if value == "" || len(value) > 253 || strings.ContainsAny(value, "/: @\\\t\r\n") || !strings.Contains(value, ".") {
				continue // Legacy catalogues contain occasional non-domain labels.
			}
			domains[value] = true
		}
		for _, value := range append(append([]string{}, portal.CIDR4...), portal.CIDR6...) {
			prefix, err := netip.ParsePrefix(value)
			if err != nil || prefix.Bits() == 0 {
				return nil, nil, fmt.Errorf("invalid CIDR in catalogue")
			}
			prefixes[prefix.Masked().String()] = true
		}
		for _, value := range append(append([]string{}, portal.IPv4...), portal.IPv6...) {
			address, err := netip.ParseAddr(value)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid IP in catalogue")
			}
			prefixes[netip.PrefixFrom(address, address.BitLen()).String()] = true
		}
	}
	if len(domains)+len(prefixes) == 0 {
		return nil, nil, fmt.Errorf("empty or unsupported catalogue")
	}
	if len(domains)+len(prefixes) > 250000 {
		return nil, nil, fmt.Errorf("catalogue too large")
	}
	d, p := make([]string, 0, len(domains)), make([]string, 0, len(prefixes))
	for value := range domains {
		d = append(d, value)
	}
	for value := range prefixes {
		p = append(p, value)
	}
	sort.Strings(d)
	sort.Strings(p)
	return d, p, nil
}

// Refresh at most daily when the configuration is rebuilt. Bootstrap uses HTTPS
// before the VPN exists; a validated on-disk cache supports offline restarts.
func loadHandbook(ctx context.Context, sourceURL string) ([]byte, error) {
	handbookCacheLock.Lock()
	defer handbookCacheLock.Unlock()
	sum := sha256.Sum256([]byte(sourceURL))
	path := filepath.Join("data", "handbook", hex.EncodeToString(sum[:])+".json")
	cached, cacheErr := os.ReadFile(path)
	if cacheErr == nil {
		if _, _, err := parseHandbook(cached); err != nil {
			cached = nil
		}
		if info, err := os.Stat(path); err == nil && cached != nil && time.Since(info.ModTime()) < 24*time.Hour {
			return cached, nil
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := handbookClient.Do(request)
	var body []byte
	if err == nil {
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			err = fmt.Errorf("HTTP %d", response.StatusCode)
		} else {
			body, err = io.ReadAll(io.LimitReader(response.Body, handbookMaxBytes+1))
			if err == nil && len(body) > handbookMaxBytes {
				err = fmt.Errorf("catalogue download too large")
			}
			if err == nil {
				_, _, err = parseHandbook(body)
			}
		}
	}
	if err != nil {
		if cached != nil {
			return cached, nil
		}
		return nil, fmt.Errorf("routing list unavailable and no valid cache: %w", err)
	}
	// Cache failure does not discard a valid download. Atomic rename protects the
	// previous version if writing is interrupted; no external file path is accepted.
	if os.MkdirAll(filepath.Dir(path), 0700) == nil {
		if file, e := os.CreateTemp(filepath.Dir(path), ".download-*"); e == nil {
			name := file.Name()
			_, writeErr := file.Write(body)
			closeErr := file.Close()
			if writeErr == nil && closeErr == nil {
				_ = os.Rename(name, path)
			}
			_ = os.Remove(name)
		}
	}
	return body, nil
}
