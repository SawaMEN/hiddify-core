package hcore

import (
	"fmt"
	"slices"
	"sync"

	hcommon "github.com/hiddify/hiddify-core/v2/hcommon"
	"google.golang.org/grpc"
)

// stateMu orders every change of the core state with its publication, so listeners see the
// states in the order they were set. The ping watcher changes the state without static.lock,
// hence the separate lock.
var stateMu sync.Mutex

func SetCoreStatus(state CoreStates, msgType MessageType, message string) *CoreInfoResponse {
	stateMu.Lock()
	defer stateMu.Unlock()
	return setCoreStatusLocked(state, msgType, message)
}

// setCoreStatusIf changes the state only while it is one of from and is not state already, and
// reports whether it did.
func setCoreStatusIf(from []CoreStates, state CoreStates, msgType MessageType, message string) (*CoreInfoResponse, bool) {
	stateMu.Lock()
	defer stateMu.Unlock()
	if !slices.Contains(from, currentCoreState()) {
		return nil, false
	}
	if currentCoreState() == state && msgType == MessageType_EMPTY {
		// nothing changed: listeners are told of changes only
		return nil, false
	}
	return setCoreStatusLocked(state, msgType, message), true
}

func setCoreStatusLocked(state CoreStates, msgType MessageType, message string) *CoreInfoResponse {
	msg := fmt.Sprintf("%s: %s %s", state.String(), msgType.String(), message)
	if msgType == MessageType_EMPTY {
		msg = fmt.Sprintf("%s: %s", state.String(), message)
	}
	Log(LogLevel_INFO, LogType_CORE, msg)
	setCoreState(state)
	info := CoreInfoResponse{
		CoreState:   state,
		MessageType: msgType,
		Message:     message,
	}
	static.coreInfoObserver.Publish(&info)

	return &info
}

// setCoreState changes the state without publishing it; stateMu must be held.
func setCoreState(state CoreStates) {
	static.CoreState = state
	coreState.Store(int32(state))
}

func (s *CoreService) CoreInfoListener(req *hcommon.Empty, stream grpc.ServerStreamingServer[CoreInfoResponse]) error {
	coreSub := static.coreInfoObserver.Subscribe(1)
	defer static.coreInfoObserver.Unsubscribe(coreSub)
	if err := stream.Send(&CoreInfoResponse{
		CoreState:   currentCoreState(),
		MessageType: MessageType_EMPTY,
		Message:     "",
	}); err != nil {
		return err
	}
	for {
		select {
		case <-stream.Context().Done():
			return nil
		case info := <-coreSub:
			if err := stream.Send(info); err != nil {
				return err
			}
			// case <-time.After(500 * time.Millisecond):
			// 	// 	info := SetCoreStatus(CoreStates_STOPPED, MessageType_EMPTY, "")
			// 	if err := stream.Send(&CoreInfoResponse{
			// 		CoreState:   currentCoreState(),
			// 		MessageType: MessageType_EMPTY,
			// 		Message:     "",
			// 	})
		}
	}
}
