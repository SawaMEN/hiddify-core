package hcore

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/hiddify/hiddify-core/v2/config"
	"github.com/hiddify/hiddify-core/v2/hcommon"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/hiddify/ipinfo"
	M "github.com/sagernet/sing/common/metadata"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func selectedOutbound(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc, adapter.Outbound, error) {
	instance := static.Instance()
	if instance == nil {
		return nil, nil, nil, status.Error(codes.FailedPrecondition, "VPN is stopped")
	}
	outbound, ok := instance.Box().Outbound().Outbound(config.OutboundSelectTag)
	if !ok {
		return nil, nil, nil, status.Error(codes.FailedPrecondition, "selected outbound is unavailable")
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	stop := context.AfterFunc(instance.Context(), cancel)
	return probeCtx, func() { stop(); cancel() }, outbound, nil
}

const maxProbeDownload = int64(5 * 1024 * 1024)

var downloadProbeGate = make(chan struct{}, 1)

func validateProbeRequest(req *NetworkProbeRequest) error {
	if req == nil || len(req.Url) > 2048 || req.DownloadBytes < 0 || req.DownloadBytes > maxProbeDownload {
		return status.Error(codes.InvalidArgument, "invalid probe request")
	}
	uri, err := url.Parse(req.Url)
	if err != nil || uri.Hostname() == "" || uri.User != nil || uri.Fragment != "" ||
		(uri.Scheme != "https" && uri.Scheme != "http") || (req.DownloadBytes > 0 && uri.Scheme != "https") {
		return status.Error(codes.InvalidArgument, "invalid probe URL")
	}
	return nil
}

func readProbeDownload(ctx context.Context, body io.Reader, limit int64) (int64, int64, error) {
	if limit <= 0 || limit > maxProbeDownload {
		return 0, 0, status.Error(codes.InvalidArgument, "invalid download limit")
	}
	if err := ctx.Err(); err != nil {
		return 0, 0, err
	}
	started := time.Now()
	bytes, err := io.CopyN(io.Discard, body, limit)
	elapsed := time.Since(started).Nanoseconds()
	if err != nil && err != io.EOF {
		return 0, 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, 0, err
	}
	if bytes == 0 {
		return 0, 0, io.ErrUnexpectedEOF
	}
	if elapsed < 1 {
		elapsed = 1
	}
	return bytes, elapsed, nil
}

func (s *CoreService) ProbeConnection(ctx context.Context, req *NetworkProbeRequest) (*NetworkProbeResponse, error) {
	if err := validateProbeRequest(req); err != nil {
		return nil, err
	}
	if req.DownloadBytes > 0 {
		select {
		case downloadProbeGate <- struct{}{}:
			defer func() { <-downloadProbeGate }()
		default:
			return nil, status.Error(codes.ResourceExhausted, "download test already running")
		}
	}

	static.settingsLock.RLock()
	adaptive := static.HiddifyOptions != nil && static.HiddifyOptions.AdaptiveNetwork
	static.settingsLock.RUnlock()
	budget, tlsTimeout, headerTimeout := 12*time.Second, 5*time.Second, 8*time.Second
	if adaptive || req.DownloadBytes > 0 {
		budget, tlsTimeout, headerTimeout = 30*time.Second, 12*time.Second, 25*time.Second
	}
	ctx, cancel, outbound, err := selectedOutbound(ctx, budget)
	if err != nil {
		return nil, err
	}
	defer cancel()
	transport := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return outbound.DialContext(ctx, network, M.ParseSocksaddr(address))
	}, DisableKeepAlives: true, DisableCompression: true, TLSHandshakeTimeout: tlsTimeout, ResponseHeaderTimeout: headerTimeout}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, req.Url, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	result := &NetworkProbeResponse{StatusCode: int32(response.StatusCode)}
	if req.DownloadBytes > 0 {
		if response.StatusCode != http.StatusOK {
			return nil, status.Error(codes.Unavailable, "download endpoint rejected request")
		}
		bytes, elapsed, err := readProbeDownload(ctx, response.Body, req.DownloadBytes)
		if err != nil {
			return nil, err
		}
		result.DownloadBytes = bytes
		result.TransferNanos = elapsed
	}
	return result, nil
}

func (s *CoreService) GetCurrentIpInfo(ctx context.Context, _ *hcommon.Empty) (*IpInfo, error) {
	ctx, cancel, outbound, err := selectedOutbound(ctx, 12*time.Second)
	if err != nil {
		return nil, err
	}
	defer cancel()
	static.stateLock.RLock()
	factory := static.CoreLogFactory
	static.stateLock.RUnlock()
	if factory == nil {
		return nil, status.Error(codes.FailedPrecondition, "core logger unavailable")
	}
	info, _, err := ipinfo.GetIpInfo(factory.NewLogger("diagnostics"), ctx, outbound)
	if err != nil {
		return nil, fmt.Errorf("proxy IP lookup: %w", err)
	}
	return &IpInfo{Ip: info.IP, CountryCode: info.CountryCode, Region: info.Region, City: info.City, Asn: int32(info.ASN), Org: info.Org, Latitude: info.Latitude, Longitude: info.Longitude, PostalCode: info.PostalCode}, nil
}
