package hcore

import (
	"context"
	"time"

	"github.com/hiddify/hiddify-core/v2/config"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
)

func (s *CoreService) Restart(ctx context.Context, in *StartRequest) (*CoreInfoResponse, error) {
	return Restart(ctx, in)
}

func Restart(ctx context.Context, in *StartRequest) (coreResponse *CoreInfoResponse, err error) {
	defer config.DeferPanicToError("startmobile", func(recovered_err error) {
		coreResponse, err = errorWrapper(MessageType_UNEXPECTED_ERROR, recovered_err)
	})
	log.Debug("[Service] Restarting")
	// Reserve one generation and keep stop/delay/start in one lifecycle lock.
	// A user Stop can still invalidate and cancel this operation without the lock.
	epoch := cancelStart()
	static.lock.Lock()
	defer static.lock.Unlock()
	restartCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	static.startLock.Lock()
	if epoch != static.startEpoch.Load() {
		static.startLock.Unlock()
		return nil, context.Canceled
	}
	static.startCancel = cancel
	static.startLock.Unlock()
	defer func() { static.startLock.Lock(); static.startCancel = nil; static.startLock.Unlock() }()
	if err := restartCtx.Err(); err != nil {
		return nil, err
	}
	resp, err := stopLocked()
	if err != nil {
		return resp, err
	}

	if C.IsAndroid && settingsSnapshot().EnableTun {
		select {
		case <-restartCtx.Done():
			return SetCoreStatus(CoreStates_STOPPED, MessageType_INSTANCE_NOT_STARTED, "restart cancelled"), restartCtx.Err()
		case <-time.After(time.Second):
		}
	}
	return startServiceLocked(restartCtx, in, epoch)
}
