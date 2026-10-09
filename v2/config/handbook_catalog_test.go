package config

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

const handbookFixture = `{"service.example":{"domains":["*.service.example","service.example."],"ip4":["203.0.113.4"],"ip6":["2001:db8::4"],"cidr4":["198.51.100.7/24"],"cidr6":["2001:db8:1::/48"]}}`

func TestHandbookCatalogueNormalization(t *testing.T) {
	domains, ips, err := parseHandbook([]byte(handbookFixture))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(domains, []string{"service.example"}) {
		t.Fatal(domains)
	}
	for _, expected := range []string{"203.0.113.4/32", "2001:db8::4/128", "198.51.100.0/24", "2001:db8:1::/48"} {
		found := false
		for _, actual := range ips {
			if actual == expected {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing %s: %v", expected, ips)
		}
	}
	for _, body := range []string{`<html>error</html>`, `{"message":"Controller not found"}`, `{}`, `{"x":{"cidr4":["0.0.0.0/0"]}}`, `{"x":{"domains":["."]}}`, `{"x":{"ip4":["invalid"]}}`} {
		if _, _, err := parseHandbook([]byte(body)); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}

func TestHandbookURLMultipleSelections(t *testing.T) {
	u, err := url.Parse(handbookURL("iplist.my-handbook.ru", "YouTube.com, discord.com;example.org"))
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "https" || u.Host != "iplist.my-handbook.ru" || u.Query().Get("format") != "json" {
		t.Fatal(u)
	}
	if !reflect.DeepEqual(u.Query()["site"], []string{"youtube.com", "discord.com", "example.org"}) {
		t.Fatal(u.Query())
	}
}

func TestHandbookOfflineCacheAndCorruptRefresh(t *testing.T) {
	t.Chdir(t.TempDir())
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			w.Write([]byte(handbookFixture))
		} else {
			w.Write([]byte(`{"message":"bad gateway"}`))
		}
	}))
	defer server.Close()
	first, err := loadHandbook(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = loadHandbook(context.Background(), server.URL); err != nil || requests != 1 {
		t.Fatalf("fresh cache was not reused: %d %v", requests, err)
	}
	files, err := os.ReadDir("data/handbook")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		past := time.Now().Add(-48 * time.Hour)
		os.Chtimes("data/handbook/"+file.Name(), past, past)
	}
	fallback, err := loadHandbook(context.Background(), server.URL)
	if err != nil || string(fallback) != string(first) || requests != 2 {
		t.Fatalf("invalid refresh replaced cache: %v", err)
	}
	server.Close()
	if _, err = loadHandbook(context.Background(), server.URL); err != nil {
		t.Fatal(err)
	}
	if _, err = loadHandbook(context.Background(), server.URL+"/uncached"); err == nil || !strings.Contains(err.Error(), "no valid cache") {
		t.Fatal("first use must report an unavailable provider")
	}
}

func TestHandbookLocalServiceDomains(t *testing.T) {
	for _, test := range []struct {
		name    string
		body    string
		domains []string
	}{
		{"repo", `{"repo":{"domains":["github.com"]}}`, []string{"docker.io", "github.com"}},
		{"pornhub", `{"pornhub.com":{"domains":["pornhub.com", "phncdn.com"]}}`, []string{"phncdn.com", "pornhub.com", "pornhub.org"}},
		{"unrelated", `{"youtube.com":{"domains":["youtube.com"]}}`, []string{"youtube.com"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			domains, _, err := parseHandbook([]byte(test.body))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(domains, test.domains) {
				t.Fatalf("domains = %v, want %v", domains, test.domains)
			}
		})
	}
}
