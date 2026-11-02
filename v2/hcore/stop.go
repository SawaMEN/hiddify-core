package hcore

import (
	"context"
	"fmt"

	hcommon "github.com/hiddify/hiddify-core/v2/hcommon"
)

// stoppable are the states a stop moves to STOPPING.
var stoppable = append([]CoreStates{CoreStates_STARTING, CoreStates_HOT_RELOADING}, running...)

// Stop queues a stop of the core and answers at once with STOPPING (or ALREADY_STOPPED when the
// core is stopped or will be); the outcome is reported through CoreInfoListener.
func (s *CoreService) Stop(ctx context.Context, empty *hcommon.Empty) (*CoreInfoResponse, error) {
	queue.Lock()
	defer queue.Unlock()
	if !startingLocked() {
		return &CoreInfoResponse{CoreState: currentCoreState(), MessageType: MessageType_ALREADY_STOPPED}, nil
	}
	queueStopLocked()
	// disconnecting shows at once, not when the queue reaches the stop
	setCoreStatusIf(stoppable, CoreStates_STOPPING, MessageType_EMPTY, "")
	return &CoreInfoResponse{CoreState: CoreStates_STOPPING, MessageType: MessageType_EMPTY}, nil
}

// Stop stops the core after the operations queued before it, and returns the outcome. A start
// queued before it gives up instead of completing to be stopped.
func Stop() (*CoreInfoResponse, error) {
	queue.Lock()
	result := queueStopLocked()
	// disconnecting shows at once, not when the queue reaches the stop
	setCoreStatusIf(stoppable, CoreStates_STOPPING, MessageType_EMPTY, "")
	queue.Unlock()
	return wait(result)
}

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
