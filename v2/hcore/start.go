package hcore

import (
	"context"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/hiddify/hiddify-core/v2/config"
	"github.com/hiddify/hiddify-core/v2/db"
	hcommon "github.com/hiddify/hiddify-core/v2/hcommon"
	service_manager "github.com/hiddify/hiddify-core/v2/service_manager"
	"github.com/sagernet/sing-box/adapter"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/experimental/libbox"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/service"
)

// Start queues a start of the core and answers at once with STARTING; the outcome is reported
// through CoreInfoListener. The core is started only once it is disconnected: ALREADY_STARTED
// when it is started or will be, INSTANCE_NOT_STOPPED while it is stopping.
func (s *CoreService) Start(ctx context.Context, in *StartRequest) (*CoreInfoResponse, error) {
	return Start(ctx, in)
}

func Start(ctx context.Context, in *StartRequest) (*CoreInfoResponse, error) {
	return StartService(ctx, in)
}

// StartService starts the core after the operations queued before it, and returns the outcome.
func StartService(ctx context.Context, in *StartRequest) (*CoreInfoResponse, error) {
	queue.Lock()
	result := queueStartLocked(ctx, in)
	queue.Unlock()
	return wait(result)
}

// queueStartLocked queues a start that a stop called after it cancels; queue must be locked.
func queueStartLocked(ctx context.Context, in *StartRequest) <-chan operationResult {
	ctx = withStopSignalLocked(ctx)
	return enqueueLocked(operationStart, "start", func() (*CoreInfoResponse, error) {
		if err := contextErr(ctx); err != nil {
			return cancelledStart(err)
		}
		return startServiceLocked(ctx, in)
	})
}

// cancelledStart reports a start given up because its context is done; the core stays STOPPED.
// For a stop, nothing is published: the stop that follows reports STOPPING and STOPPED.
func cancelledStart(err error) (*CoreInfoResponse, error) {
	Log(LogLevel_WARNING, LogType_CORE, "start cancelled: ", err.Error())
	if errors.Is(err, errStopped) {
		// the stop that gave this start up already reports STOPPING and then STOPPED
		stateMu.Lock()
		if currentCoreState() == CoreStates_STARTING {
			setCoreState(CoreStates_STOPPED)
		}
		stateMu.Unlock()
		return &CoreInfoResponse{CoreState: CoreStates_STOPPED, MessageType: MessageType_INSTANCE_NOT_STARTED, Message: "start cancelled"}, err
	}
	return SetCoreStatus(CoreStates_STOPPED, MessageType_INSTANCE_NOT_STARTED, "start cancelled"), err
}

// currentProfileName is the name of the profile the core was last started with. It is what
// SystemInfo.current_profile reports; kept in memory so status readers never hit the database
// and never show a stale name after a profile switch.
var currentProfileName atomic.Value

func CurrentProfileName() string {
	if v, ok := currentProfileName.Load().(string); ok {
		return v
	}
	return ""
}

func saveLastStartRequest(in *StartRequest) error {
	if in.ConfigContent == "" && in.ConfigPath == "" {
		return nil
	}
	currentProfileName.Store(in.ConfigName)
	settings := db.GetTable[hcommon.AppSettings]()
	return settings.UpdateInsert(
		&hcommon.AppSettings{
			Id:    "lastStartRequestPath",
			Value: in.ConfigPath,
		},
		&hcommon.AppSettings{
			Id:    "lastStartRequestContent",
			Value: in.ConfigContent,
		},
		&hcommon.AppSettings{
			Id:    "lastStartRequestName",
			Value: in.ConfigName,
		},
	)
}

func loadLastStartRequestIfNeeded(in *StartRequest) (*StartRequest, error) {
	if in != nil && (in.ConfigContent != "" || in.ConfigPath != "") {
		return in, nil
	}
	settings := db.GetTable[hcommon.AppSettings]()
	lastPath, err := settings.Get("lastStartRequestPath")
	if err != nil {
		return nil, err
	}
	lastContent, err := settings.Get("lastStartRequestContent")
	if err != nil {
		return nil, err
	}

	lastName, err := settings.Get("lastStartRequestName")
	if err != nil {
		return nil, err
	}
	return &StartRequest{
		ConfigPath:    lastPath.Value.(string),
		ConfigContent: lastContent.Value.(string),
		ConfigName:    lastName.Value.(string),
	}, nil
}

func StartService(ctx context.Context, in *StartRequest) (coreResponse *CoreInfoResponse, err error) {
	defer config.DeferPanicToError("startmobile", func(recovered_err error) {
		coreResponse, err = errorWrapper(MessageType_UNEXPECTED_ERROR, recovered_err)
	})
	epoch := static.startEpoch.Load()
	static.lock.Lock()
	defer static.lock.Unlock()
	return startServiceLocked(ctx, in, epoch)
}

// noConfigPersistenceKey is a per-start capability used by macOS Network
// Extensions. The ordinary desktop/Android StartService path remains unchanged.
type noConfigPersistenceKey struct{}

// StartServiceWithoutConfigPersistence starts the same service but never
// writes the raw profile or expanded (credential-bearing) configuration to
// the normal Hiddify AppSettings/current-config.json disk locations.
func StartServiceWithoutConfigPersistence(ctx context.Context, in *StartRequest) (*CoreInfoResponse, error) {
	return StartService(context.WithValue(ctx, noConfigPersistenceKey{}, true), in)
}

// Caller holds static.lock throughout the lifecycle operation.
func startServiceLocked(ctx context.Context, in *StartRequest, epoch uint64) (coreResponse *CoreInfoResponse, err error) {
	persistConfig := ctx.Value(noConfigPersistenceKey{}) == nil
	startupCtx, cancel := context.WithCancel(ctx)
	static.startLock.Lock()
	if epoch != static.startEpoch.Load() {
		static.startLock.Unlock()
		cancel()
		return nil, context.Canceled
	}
	static.startCancel = cancel
	static.startLock.Unlock()
	defer func() { cancel(); static.startLock.Lock(); static.startCancel = nil; static.startLock.Unlock() }()
	static.stateLock.RLock()
	baseCtx, platform := static.BaseContext, static.globalPlatformInterface
	static.stateLock.RUnlock()
	ctx = libbox.FromContext(startupCtx, platform)

	if coreState() != CoreStates_STOPPED {
		// return errorWrapper(MessageType_ALREADY_STARTED, fmt.Errorf("instance already started"))
		return &CoreInfoResponse{
			CoreState:   coreState(),
			MessageType: MessageType_ALREADY_STARTED,
			Message:     "instance already started",
		}, nil
	}
	watch := newStopwatch("start")
	SetCoreStatus(CoreStates_STARTING, MessageType_EMPTY, "")

	in, err = loadLastStartRequestIfNeeded(in)
	if err != nil {
		return errorWrapper(MessageType_ERROR_BUILDING_CONFIG, err)
	}

	static.stateLock.Lock()
	static.previousStartRequest = in
	static.stateLock.Unlock()

	watch.step("loading the request")
	options, err := BuildConfig(ctx, in)
	if err != nil {
		return errorWrapper(MessageType_ERROR_BUILDING_CONFIG, err)
	}
	if persistConfig {
		saveLastStartRequest(in)
	}

	Log(LogLevel_DEBUG, LogType_CORE, "Main Service pre start")
	if err := service_manager.OnMainServicePreStart(options); err != nil {
		return errorWrapper(MessageType_ERROR_EXTENSION, err)
	}
	if persistConfig {
		currentBuildConfigPath := filepath.Join(sWorkingPath, "data/current-config.json")
		Log(LogLevel_DEBUG, LogType_CORE, "Saving config to ", currentBuildConfigPath)
		config.SaveCurrentConfig(ctx, currentBuildConfigPath, *options)
	}
	if static.debug.Load() {
		pout, err := options.MarshalJSONContext(ctx)
		if err != nil {
			return errorWrapper(MessageType_ERROR_BUILDING_CONFIG, err)
		}
		Log(LogLevel_INFO, LogType_CORE, "Current Config is:\n", string(pout))
	}
	serviceCtx := libbox.FromContext(baseCtx, platform)
	if platform != nil {
		platformWrapper := libbox.WrapPlatformInterface(platform)
		service.MustRegister[adapter.PlatformInterface](serviceCtx, platformWrapper)
		// } else {
		// 	service.MustRegister[adapter.PlatformInterface](ctx, (*adapter.PlatformInterface)nil)
	}
	Log(LogLevel_DEBUG, LogType_CORE, "Stating Service with delay ?", in.DelayStart)
	if in.DelayStart {
		select {
		case <-ctx.Done():
			return errorWrapper(MessageType_START_SERVICE, ctx.Err())
		case <-time.After(time.Second):
		}
	}
	if err := contextErr(ctx); err != nil {
		return cancelledStart(err)
	}
	watch.step("preparing the platform and the delay")
	libbox.SetMemoryLimit(C.IsIos || !in.DisableMemoryLimit)
	if err := ctx.Err(); err != nil {
		return errorWrapper(MessageType_START_SERVICE, err)
	}
	instance, err := NewServiceWithStartupContext(serviceCtx, ctx, *options)
	if err != nil {
		watch.done("failed: " + err.Error())
		return errorWrapper(MessageType_START_SERVICE, err)
	}
	if err := ctx.Err(); err != nil {
		instance.CloseService()
		instance.Close()
		return errorWrapper(MessageType_START_SERVICE, err)
	}
	setStartedService(instance)
	if static.debug.Load() {
		dumpGoroutinesToFile(fmt.Sprint(sWorkingPath, "/data/goroutine-start.log"))
	}
	for inb := range options.Inbounds {
		if opts, ok := options.Inbounds[inb].Options.(option.SocksInboundOptions); ok {
			static.ListenPort = opts.ListenPort
		}
	}
	// Publish WAITING so CoreInfoListener leaves STARTING; the connectivity watcher only
	// moves among WAITING/STARTED/NO_AVAILABLE_PROXIES and cannot leave STARTING itself.
	response, ok := setCoreStatusIf([]CoreStates{CoreStates_STARTING}, CoreStates_WAITING, MessageType_EMPTY, "")
	if !ok {
		// a stop called meanwhile has already reported STOPPING; it closes the service
		return &CoreInfoResponse{CoreState: currentCoreState(), MessageType: MessageType_INSTANCE_NOT_STARTED, Message: "start cancelled"}, errStopped
	}
	startConnectivityWatcher()
	watch.done("core is up (WAITING for a ping)")
	return response, nil
}
