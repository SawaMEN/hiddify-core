package hcore

import (
	"bytes"
	"context"
	"testing"
)

func TestDownloadProbeBoundsAndTransport(t *testing.T) {
	for _, req := range []*NetworkProbeRequest{
		nil, {Url: "https://example.com", DownloadBytes: -1}, {Url: "https://example.com", DownloadBytes: maxProbeDownload + 1},
		{Url: "http://example.com", DownloadBytes: 1}, {Url: "https://user:secret@example.com", DownloadBytes: 1},
		{Url: "https://example.com/#fragment", DownloadBytes: 1},
	} {
		if validateProbeRequest(req) == nil {
			t.Fatalf("accepted invalid request: %+v", req)
		}
	}
	if err := validateProbeRequest(&NetworkProbeRequest{Url: "https://example.com", DownloadBytes: maxProbeDownload}); err != nil {
		t.Fatal(err)
	}
	if err := validateProbeRequest(&NetworkProbeRequest{Url: "http://example.com"}); err != nil {
		t.Fatal("legacy header probe rejected", err)
	}
}
func TestDownloadProbeStopsAtByteLimit(t *testing.T) {
	reader := bytes.NewReader(bytes.Repeat([]byte{1}, 100000))
	n, elapsed, err := readProbeDownload(context.Background(), reader, 65536)
	if err != nil || n != 65536 || elapsed <= 0 || reader.Len() != 100000-65536 {
		t.Fatalf("unexpected bounded result: %d %d %v, remaining %d", n, elapsed, err, reader.Len())
	}
}
func TestDownloadProbeEmptyAndCancellation(t *testing.T) {
	if _, _, err := readProbeDownload(context.Background(), bytes.NewReader(nil), 65536); err == nil {
		t.Fatal("accepted empty response")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reader := bytes.NewReader([]byte("unread"))
	if _, _, err := readProbeDownload(ctx, reader, 65536); err == nil {
		t.Fatal("ignored cancellation")
	}
	if reader.Len() != 6 {
		t.Fatal("read after cancellation")
	}
}
