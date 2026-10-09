package hcore

import (
	"context"
	"fmt"
	"path/filepath"
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

func (s *CoreService) Start(ctx context.Context, in *StartRequest) (*CoreInfoResponse, error) {
	return Start(ctx, in)
}

func Start(ctx context.Context, in *StartRequest) (*CoreInfoResponse, error) {
	return StartService(ctx, in)
}

func (s *CoreService) StartService(ctx context.Context, in *StartRequest) (*CoreInfoResponse, error) {
	return StartService(ctx, in)
}

func saveLastStartRequest(in *StartRequest) error {
	if in.ConfigContent == "" && in.ConfigPath == "" {
		return nil
	}
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

// Caller holds static.lock throughout the lifecycle operation.
func startServiceLocked(ctx context.Context, in *StartRequest, epoch uint64) (coreResponse *CoreInfoResponse, err error) {
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
	SetCoreStatus(CoreStates_STARTING, MessageType_EMPTY, "")

	in, err = loadLastStartRequestIfNeeded(in)
	if err != nil {
		return errorWrapper(MessageType_ERROR_BUILDING_CONFIG, err)
	}

	static.stateLock.Lock()
	static.previousStartRequest = in
	static.stateLock.Unlock()

	options, err := BuildConfig(ctx, in)
	if err != nil {
		return errorWrapper(MessageType_ERROR_BUILDING_CONFIG, err)
	}
	saveLastStartRequest(in)

	Log(LogLevel_DEBUG, LogType_CORE, "Main Service pre start")
	if err := service_manager.OnMainServicePreStart(options); err != nil {
		return errorWrapper(MessageType_ERROR_EXTENSION, err)
	}
	currentBuildConfigPath := filepath.Join(sWorkingPath, "data/current-config.json")
	Log(LogLevel_DEBUG, LogType_CORE, "Saving config to ", currentBuildConfigPath)

	config.SaveCurrentConfig(ctx, currentBuildConfigPath, *options)
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
	libbox.SetMemoryLimit(C.IsIos || !in.DisableMemoryLimit)
	if err := ctx.Err(); err != nil {
		return errorWrapper(MessageType_START_SERVICE, err)
	}
	instance, err := NewServiceWithStartupContext(serviceCtx, ctx, *options)
	if err != nil {
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

	return SetCoreStatus(CoreStates_STARTED, MessageType_EMPTY, ""), nil
}
