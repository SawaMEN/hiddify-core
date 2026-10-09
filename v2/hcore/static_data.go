package hcore

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hiddify/hiddify-core/v2/config"
	"github.com/sagernet/sing-box/common/monitoring"
	"github.com/sagernet/sing-box/daemon"
	"github.com/sagernet/sing-box/experimental/libbox"
	"github.com/sagernet/sing-box/log"
)

type HiddifyInstance struct {
	StartedService *daemon.StartedService
	HiddifyOptions *config.HiddifyOptions
	// activeConfigPath string
	CoreLogFactory            log.Factory
	coreInfoObserver          *monitoring.Broadcaster[*CoreInfoResponse]
	CoreState                 CoreStates
	logObserver               *monitoring.Broadcaster[*LogMessage]
	systemInfoObserver        *monitoring.Broadcaster[*SystemInfo]
	outboundsInfoObserver     *monitoring.Broadcaster[*OutboundGroupList]
	mainOutboundsInfoObserver *monitoring.Broadcaster[*OutboundGroupList]
	lock                      sync.Mutex
	stateLock                 sync.RWMutex
	settingsLock              sync.RWMutex
	startLock                 sync.Mutex
	startCancel               context.CancelFunc
	startEpoch                atomic.Uint64
	globalPlatformInterface   libbox.PlatformInterface
	previousStartRequest      *StartRequest
	debug                     atomic.Bool
	ListenPort                uint16
	BaseContext               context.Context
	endPauseTimer             *time.Timer // only for ios

	logLevel atomic.Int32
}

var static = &HiddifyInstance{
	CoreState:                 CoreStates_STOPPED,
	coreInfoObserver:          monitoring.NewBroadcaster[*CoreInfoResponse](context.Background()),
	logObserver:               monitoring.NewBroadcaster[*LogMessage](context.Background()),
	systemInfoObserver:        monitoring.NewBroadcaster[*SystemInfo](context.Background()),
	outboundsInfoObserver:     monitoring.NewBroadcaster[*OutboundGroupList](context.Background()),
	mainOutboundsInfoObserver: monitoring.NewBroadcaster[*OutboundGroupList](context.Background()),
}

// Published settings are immutable; builders hold their own value snapshot.
func settingsSnapshot() *config.HiddifyOptions {
	static.settingsLock.RLock()
	defer static.settingsLock.RUnlock()
	if static.HiddifyOptions == nil {
		return config.DefaultHiddifyOptions()
	}
	next := *static.HiddifyOptions
	return &next
}
func coreState() CoreStates {
	static.stateLock.RLock()
	defer static.stateLock.RUnlock()
	return static.CoreState
}
func setStartedService(ss *daemon.StartedService) {
	static.stateLock.Lock()
	static.StartedService = ss
	static.stateLock.Unlock()
}
func cancelStart() uint64 {
	epoch := static.startEpoch.Add(1)
	static.startLock.Lock()
	defer static.startLock.Unlock()
	if static.startCancel != nil {
		static.startCancel()
	}
	return epoch
}
