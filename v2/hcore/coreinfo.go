package hcore

import (
	"fmt"

	hcommon "github.com/hiddify/hiddify-core/v2/hcommon"
	"google.golang.org/grpc"
)

func SetCoreStatus(state CoreStates, msgType MessageType, message string) *CoreInfoResponse {
	msg := fmt.Sprintf("%s: %s %s", state.String(), msgType.String(), message)
	if msgType == MessageType_EMPTY {
		msg = fmt.Sprintf("%s: %s", state.String(), message)
	}
	Log(LogLevel_INFO, LogType_CORE, msg)
	static.stateLock.Lock()
	static.CoreState = state
	static.stateLock.Unlock()
	info := CoreInfoResponse{
		CoreState:   state,
		MessageType: msgType,
		Message:     message,
	}
	static.coreInfoObserver.Publish(&info)

	return &info
}

func (s *CoreService) CoreInfoListener(req *hcommon.Empty, stream grpc.ServerStreamingServer[CoreInfoResponse]) error {
	coreSub := static.coreInfoObserver.Subscribe(1)
	defer static.coreInfoObserver.Unsubscribe(coreSub)
	if err := stream.Send(&CoreInfoResponse{
		CoreState:   coreState(),
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
			// 		CoreState:   coreState(),
			// 		MessageType: MessageType_EMPTY,
			// 		Message:     "",
			// 	})
		}
	}
}
