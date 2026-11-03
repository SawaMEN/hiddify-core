package hcore

import (
	"context"
	"errors"
	"github.com/hiddify/hiddify-core/v2/config"
	"github.com/sagernet/sing-box/experimental/libbox"
	"net"
	"syscall"
	"testing"
	"time"
)

func TestFailedListenerDoesNotPoisonRegistry(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if _, err := StartGrpcServerByMode(address, SetupMode_GRPC_NORMAL_INSECURE); err == nil {
		t.Fatal("accepted occupied port")
	}
	listener.Close()
	server, err := StartGrpcServerByMode(address, SetupMode_GRPC_NORMAL_INSECURE)
	if err != nil {
		t.Fatal(err)
	}
	if server == nil {
		t.Fatal("missing server")
	}
	defer CloseGrpcServer(SetupMode_GRPC_NORMAL_INSECURE)
	// Setup must return success without dereferencing a nil error.
	response, err := (&CoreService{}).Setup(context.Background(), &SetupRequest{Mode: SetupMode_GRPC_NORMAL_INSECURE})
	if err != nil || response == nil {
		t.Fatalf("setup: %v %v", response, err)
	}
}
func TestInvalidSettingsAreTransactional(t *testing.T) {
	before := settingsSnapshot()
	if _, err := ChangeHiddifySettings(&ChangeHiddifySettingsRequest{HiddifySettingsJson: `{"mixed-port":"invalid"}`}, false); err == nil {
		t.Fatal("invalid settings accepted")
	}
	after := settingsSnapshot()
	if before.InboundOptions.MixedPort != after.InboundOptions.MixedPort {
		t.Fatal("failed update changed settings")
	}
}
func TestCancelledStartup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := StartService(ctx, &StartRequest{ConfigContent: `{}`, DelayStart: true})
	if err == nil {
		t.Fatal("cancelled startup succeeded")
	}
	if currentCoreState() != CoreStates_STOPPED {
		t.Fatal("cancelled startup left busy state")
	}
}

func TestStartupRequestCancellationDoesNotStopSuccessfulService(t *testing.T) {
	dir := t.TempDir()
	if err := libbox.Setup(&libbox.SetupOptions{BasePath: dir, WorkingPath: dir, TempPath: dir}); err != nil {
		t.Fatal(err)
	}
	base := libbox.BaseContext(nil)
	options, err := config.ReadSingOptions(base, &config.ReadOptions{Content: `{"outbounds":[{"type":"direct","tag":"direct"}]}`})
	if err != nil {
		t.Fatal(err)
	}
	request, cancel := context.WithCancel(base)
	instance, err := NewServiceWithStartupContext(base, request, *options)
	if errors.Is(err, syscall.EPERM) {
		t.Skip("runtime environment prohibits netlink sockets")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer instance.Close()
	defer instance.CloseService()
	cancel()
	if err := instance.Instance().Context().Err(); err != nil {
		t.Fatalf("successful service tied to RPC lifetime: %v", err)
	}
}

func TestQueuedRestartInvalidatedByStop(t *testing.T) {
	static.lock.Lock()
	queue.Lock()
	result := queueRestartLocked(context.Background(), &StartRequest{ConfigContent: `{}`})
	// This is the same stop signal the public Stop operation sends before it
	// waits for the lifecycle lock. The restart must not start after it.
	signalStopLocked()
	queue.Unlock()
	static.lock.Unlock()
	select {
	case outcome := <-result:
		if !errors.Is(outcome.err, errStopped) {
			t.Fatalf("invalidated restart returned %v", outcome.err)
		}
	case <-time.After(time.Second):
		t.Fatal("invalidated restart remained blocked")
	}
}
