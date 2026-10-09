package hcore

import (
	"context"
	"fmt"

	"github.com/hiddify/hiddify-core/v2/config"
	hcommon "github.com/hiddify/hiddify-core/v2/hcommon"
)

func (s *CoreService) Stop(ctx context.Context, empty *hcommon.Empty) (*CoreInfoResponse, error) {
	return Stop()
}

func Stop() (coreResponse *CoreInfoResponse, err error) {
	defer config.DeferPanicToError("stop", func(recovered_err error) {
		coreResponse, err = errorWrapper(MessageType_UNEXPECTED_ERROR, recovered_err)
	})

	// if static.CoreState != CoreStates_STARTED {
	// 	return errorWrapper(MessageType_INSTANCE_NOT_STARTED, fmt.Errorf("instance not started"))
	// }
	// if static.Box == nil {
	// 	return errorWrapper(MessageType_INSTANCE_NOT_FOUND, fmt.Errorf("instance not found"))
	// }
	cancelStart()
	static.lock.Lock()
	defer static.lock.Unlock()
	return stopLocked()
}

// Caller holds static.lock. Cancellation must happen before acquiring it.
func stopLocked() (coreResponse *CoreInfoResponse, err error) {
	SetCoreStatus(CoreStates_STOPPING, MessageType_EMPTY, "")
	static.stateLock.RLock()
	ss := static.StartedService
	static.stateLock.RUnlock()
	if ss == nil {
		return SetCoreStatus(CoreStates_STOPPED, MessageType_ALREADY_STOPPED, ""), nil
	}

	defer ss.Close()
	if err := ss.CloseService(); err != nil {
		setStartedService(nil)
		dumpGoroutinesToFile(fmt.Sprint(sWorkingPath, "/data/goroutine-stop.log"))
		return errorWrapper(MessageType_UNEXPECTED_ERROR, err)
	}
	// err = common.Close(static.StartedService)
	setStartedService(nil)

	return SetCoreStatus(CoreStates_STOPPED, MessageType_EMPTY, ""), nil
}
