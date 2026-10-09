package config

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/hiddify/ray2sing/ray2sing"
	"github.com/sagernet/sing-box/experimental/libbox"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/batch"
	SJ "github.com/sagernet/sing/common/json"
	"github.com/xmdhs/clash2singbox/convert"
	clash2singmodel "github.com/xmdhs/clash2singbox/model"
	"github.com/xmdhs/clash2singbox/model/clash"
	"gopkg.in/yaml.v3"
)

//go:embed config.json.template
var configByte []byte

const MaxConfigBytes = 8 * 1024 * 1024

func ReadContent(ctx context.Context, opt *ReadOptions) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if opt.Content != "" {
		if len(opt.Content) > MaxConfigBytes {
			return nil, fmt.Errorf("configuration exceeds 8 MiB")
		}
		return []byte(opt.Content), nil
	}
	file, err := os.Open(opt.Path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, MaxConfigBytes+1))
	if err != nil {
		return nil, err
	}
	if len(content) > MaxConfigBytes {
		return nil, fmt.Errorf("configuration exceeds 8 MiB")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return content, nil
}

func ParseConfig(ctx context.Context, opt *ReadOptions, debug bool, configOpt *HiddifyOptions, fullConfig bool) (*option.Options, error) {
	content, err := ReadContent(ctx, opt)
	if err != nil {
		return nil, err
	}
	return parseConfigContent(ctx, content, debug, configOpt, fullConfig)
}

func ParseConfigBytes(ctx context.Context, opt *ReadOptions, debug bool, configOpt *HiddifyOptions, fullConfig bool) ([]byte, error) {

	options, err := ParseConfig(ctx, opt, debug, configOpt, fullConfig)
	if err != nil {
		return nil, err
	}

	return options.MarshalJSONContext(ctx)

}
func parseConfigContent(ctx context.Context, content []byte, debug bool, configOpt *HiddifyOptions, fullConfig bool) (*option.Options, error) {
	if configOpt == nil {
		configOpt = DefaultHiddifyOptions()
	}

	var jsonObj map[string]interface{} = make(map[string]interface{})

	var tmpJsonResult any
	jsonDecoder := json.NewDecoder(SJ.NewCommentFilter(bytes.NewReader(content)))
	if err := jsonDecoder.Decode(&tmpJsonResult); err == nil {
		var trailing any
		if err := jsonDecoder.Decode(&trailing); err != io.EOF {
			return nil, fmt.Errorf("[SingboxParser] trailing JSON content")
		}
		if tmpJsonObj, ok := tmpJsonResult.(map[string]interface{}); ok {
			if tmpJsonObj["outbounds"] == nil && tmpJsonObj["endpoints"] == nil {
				kind, _ := tmpJsonObj["type"].(string)
				if kind == "masque-client" || kind == "awg" || kind == "wireguard" {
					jsonObj["endpoints"] = []interface{}{tmpJsonObj}
				} else {
					jsonObj["outbounds"] = []interface{}{tmpJsonObj}
				}
			} else {
				if fullConfig || (configOpt != nil && configOpt.EnableFullConfig) {
					jsonObj = tmpJsonObj
				} else {
					if tmpJsonObj["outbounds"] != nil {
						jsonObj["outbounds"] = tmpJsonObj["outbounds"]
					}
					if tmpJsonObj["endpoints"] != nil {
						jsonObj["endpoints"] = tmpJsonObj["endpoints"]
					}
					stripPanelResolverReferences(jsonObj)
				}
			}
		} else if jsonArray, ok := tmpJsonResult.([]interface{}); ok {
			if fullConfig || (configOpt != nil && configOpt.EnableFullConfig) {
				for _, item := range jsonArray {
					if object, ok := item.(map[string]interface{}); ok && (object["outbounds"] != nil || object["endpoints"] != nil) {
						return nil, fmt.Errorf("multiple complete configurations cannot share one routing policy; disable full-config mode or import one configuration")
					}
				}
			}
			if err := mergePanelJSONArray(jsonObj, jsonArray); err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("[SingboxParser] incorrect json format: expected a json object or an array of outbound objects, got %T", tmpJsonResult)
		}

		// the legacy WireGuard outbound was removed from sing-box: use the WireGuard endpoint
		outbounds, _ := jsonObj["outbounds"].([]interface{})
		endpoints, _ := jsonObj["endpoints"].([]interface{})
		if outbounds, endpoints = ray2sing.MoveLegacyWireGuardOutbounds(outbounds, endpoints); len(endpoints) > 0 {
			jsonObj["outbounds"] = outbounds
			jsonObj["endpoints"] = endpoints
		}

		newContent, err := json.MarshalIndent(jsonObj, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("[SingboxParser] %w", err)
		}

		return patchConfigStr(ctx, newContent, "SingboxParser", configOpt)
	}

	fmt.Printf("Convert using clash\n")
	clashObj := clash.Clash{}
	if err := yaml.Unmarshal(content, &clashObj); err == nil && clashObj.Proxies != nil {
		if len(clashObj.Proxies) == 0 {
			return nil, fmt.Errorf("[ClashParser] no outbounds found")
		}
		converted, endpoints, err := convert.Clash2sing(clashObj, clash2singmodel.SINGLATEST)
		if err != nil {
			return nil, fmt.Errorf("[ClashParser] converting clash to sing-box error: %w", err)
		}
		output := configByte
		output, err = convert.Patch(output, converted, endpoints, "", "", nil)
		if err != nil {
			return nil, fmt.Errorf("[ClashParser] patching clash config error: %w", err)
		}
		return patchConfigStr(ctx, output, "ClashParser", configOpt)
	}

	if links, handled, err := parsePanelLinks(ctx, string(content)); handled {
		if err != nil {
			return nil, err
		}
		return patchConfigOptions(ctx, links, "PanelShareParser", configOpt)
	}
	v2ray, err := ray2sing.Ray2SingboxOptions(ctx, string(content), configOpt.UseXrayCoreWhenPossible)
	if err == nil {
		return patchConfigOptions(ctx, v2ray, "V2rayParser", configOpt)
	}

	return nil, fmt.Errorf("unable to determine config format")
}

func patchConfigStr(ctx context.Context, content []byte, name string, configOpt *HiddifyOptions) (*option.Options, error) {
	options := option.Options{}
	err := options.UnmarshalJSONContext(ctx, content)

	if err != nil {
		return nil, fmt.Errorf("[SingboxParser] unmarshal error: %w", err)
	}

	return patchConfigOptions(ctx, &options, name, configOpt)
}
func patchConfigOptions(ctx context.Context, options *option.Options, name string, configOpt *HiddifyOptions) (*option.Options, error) {
	// dns outbounds were removed in sing-box 1.13; drop them instead of keeping invalid placeholders
	options.Outbounds = slices.DeleteFunc(options.Outbounds, isDNSOutbound)
	b, _ := batch.New(ctx, batch.WithConcurrencyNum[*option.Endpoint](2))
	for _, base := range options.Endpoints {
		out := base
		b.Go(base.Tag, func() (*option.Endpoint, error) {
			err := patchWarp(&out, configOpt, false, nil)
			if err != nil {
				return nil, fmt.Errorf("[Warp] patch warp error: %w", err)
			}
			// options.Outbounds[i] = base
			return &out, nil
		})
	}
	if res, err := b.WaitAndGetResult(); err != nil {
		return nil, err
	} else {
		for i, base := range options.Endpoints {
			options.Endpoints[i] = *res[base.Tag].Value
		}
	}

	// fmt.Printf("%s\n", content)
	return validateResult(ctx, options, name)
}

func validateResult(ctx context.Context, options *option.Options, name string) (*option.Options, error) {
	err := libbox.CheckConfigOptions(options)
	if err != nil {
		return nil, fmt.Errorf("[%s] invalid sing-box config: %w", name, err)
	}
	return options, nil
}
