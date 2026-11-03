package hcore

import (
	"context"
	"encoding/json"
	"os"

	"github.com/hiddify/hiddify-core/v2/config"
	"github.com/hiddify/hiddify-core/v2/db"
	hcommon "github.com/hiddify/hiddify-core/v2/hcommon"
	hutils "github.com/hiddify/hiddify-core/v2/hutils"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/experimental/libbox"
	"github.com/sagernet/sing-box/option"
)

func BuildConfigJson(ctx context.Context, in *StartRequest) (string, error) {
	Log(LogLevel_DEBUG, LogType_CORE, "Stating Service ")

	parsedContent, err := BuildConfig(ctx, in)
	if err != nil {
		return "", err
	}
	res, err := parsedContent.MarshalJSONContext(ctx)
	return string(res), err
}

func BuildConfig(ctx context.Context, in *StartRequest) (*option.Options, error) {
	Log(LogLevel_DEBUG, LogType_CORE, "Building Config...")

	readOpt := &config.ReadOptions{Content: in.ConfigContent, Path: in.ConfigPath}
	if !in.EnableRawConfig {
		// hcontent, err := json.MarshalIndent(static.HiddifyOptions, "", " ")
		// if err != nil {
		// 	return nil, err
		// }

		// Log(LogLevel_DEBUG, LogType_CORE, "Building config ", string(hcontent))
		// Log(LogLevel_DEBUG, LogType_CORE, "Building config ")
		return config.ParseBuildConfig(ctx, settingsSnapshot(), readOpt)
	}
	return config.ReadSingOptions(ctx, readOpt)

}

func (s *CoreService) Parse(ctx context.Context, in *ParseRequest) (*ParseResponse, error) {
	return Parse(libbox.FromContext(ctx, nil), in)
}

func Parse(ctx context.Context, in *ParseRequest) (response *ParseResponse, err error) {
	defer config.DeferPanicToError("parse", func(recovered error) {
		err = recovered
		Log(LogLevel_ERROR, LogType_CONFIG, recovered.Error())
	})

	path := in.TempPath
	if path == "" {
		path = in.ConfigPath
	}

	config, err := config.ParseConfigBytes(ctx, &config.ReadOptions{Content: in.Content, Path: path}, in.Debug, settingsSnapshot(), true)
	if err != nil {
		return &ParseResponse{
			ResponseCode: hcommon.ResponseCode_FAILED,
			Message:      err.Error(),
		}, err
	}
	if in.ConfigPath != "" {
		err = os.WriteFile(in.ConfigPath, config, 0o644)
		if err != nil {
			return &ParseResponse{
				ResponseCode: hcommon.ResponseCode_FAILED,
				Message:      err.Error(),
			}, err
		}
	}
	return &ParseResponse{
		ResponseCode: hcommon.ResponseCode_OK,
		Content:      string(config),
		Message:      "",
	}, err
}

func (s *CoreService) ChangeHiddifySettings(ctx context.Context, in *ChangeHiddifySettingsRequest) (*CoreInfoResponse, error) {
	return ChangeHiddifySettings(in, true)
}

func ChangeHiddifySettings(in *ChangeHiddifySettingsRequest, insert bool) (*CoreInfoResponse, error) {
	static.settingsLock.Lock()
	defer static.settingsLock.Unlock()
	next := config.DefaultHiddifyOptions()
	if in.HiddifySettingsJson != "" {
		if err := json.Unmarshal([]byte(in.HiddifySettingsJson), next); err != nil {
			return nil, err
		}
	}
	if next.Warp.WireguardConfigStr != "" {
		if err := json.Unmarshal([]byte(next.Warp.WireguardConfigStr), &next.Warp.WireguardConfig); err != nil {
			return nil, err
		}
	}
	if next.Warp2.WireguardConfigStr != "" {
		if err := json.Unmarshal([]byte(next.Warp2.WireguardConfigStr), &next.Warp2.WireguardConfig); err != nil {
			return nil, err
		}
	}
	if insert {
		if err := db.GetTable[hcommon.AppSettings]().UpdateInsert(&hcommon.AppSettings{Id: "HiddifySettingsJson", Value: in.HiddifySettingsJson}); err != nil {
			return nil, err
		}
	}
	static.HiddifyOptions = next
	switch next.LogLevel {
	case "trace":
		static.logLevel.Store(int32(LogLevel_TRACE))
	case "debug":
		static.logLevel.Store(int32(LogLevel_DEBUG))
	case "warn":
		static.logLevel.Store(int32(LogLevel_WARNING))
	case "error":
		static.logLevel.Store(int32(LogLevel_ERROR))
	case "fatal":
		static.logLevel.Store(int32(LogLevel_FATAL))
	default:
		static.logLevel.Store(int32(LogLevel_INFO))
	}

	return &CoreInfoResponse{}, nil
}

func (s *CoreService) GenerateConfig(ctx context.Context, in *GenerateConfigRequest) (*GenerateConfigResponse, error) {
	return GenerateConfig(libbox.FromContext(ctx, nil), in)
}

func GenerateConfig(ctx context.Context, in *GenerateConfigRequest) (response *GenerateConfigResponse, err error) {
	defer config.DeferPanicToError("generateConfig", func(recovered error) {
		err = recovered
		Log(LogLevel_ERROR, LogType_CONFIG, recovered.Error())
	})
	config, err := config.ParseBuildConfigBytes(ctx, settingsSnapshot(), &config.ReadOptions{Path: in.Path})
	if err != nil {
		return nil, err
	}

	return &GenerateConfigResponse{
		ConfigContent: string(config),
	}, nil
}

func removeTunnelIfNeeded(options *option.Options) (tuninb *option.TunInboundOptions) {
	if hutils.TunAllowed() {
		return nil
	}

	// Create a new slice to hold the remaining inbounds
	newInbounds := make([]option.Inbound, 0, len(options.Inbounds))

	for _, inb := range options.Inbounds {
		if inb.Type == C.TypeTun {
			if d, ok := inb.Options.(option.TunInboundOptions); ok {
				tuninb = &d
			}

		} else {
			newInbounds = append(newInbounds, inb)
		}
	}

	options.Inbounds = newInbounds
	return tuninb
}
