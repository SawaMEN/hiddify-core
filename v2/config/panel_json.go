package config

import "fmt"

// The panel's /singbox/ endpoint can return an array of full configurations,
// whereas most clients return an array of outbounds. Keep both forms valid.
func mergePanelJSONArray(target map[string]interface{}, entries []interface{}) error {
	if len(entries) == 0 {
		return fmt.Errorf("[SingboxParser] no outbounds found")
	}
	outbounds, endpoints := []interface{}{}, []interface{}{}
	for i, value := range entries {
		object, ok := value.(map[string]interface{})
		if !ok {
			return fmt.Errorf("subscription entry %d at index %d must be an object", i+1, i)
		}
		if object["outbounds"] == nil && object["endpoints"] == nil {
			kind, _ := object["type"].(string)
			if kind == "masque-client" || kind == "awg" || kind == "wireguard" {
				endpoints = append(endpoints, object)
			} else {
				outbounds = append(outbounds, object)
			}
			continue
		}
		tags := map[string]string{}
		for _, key := range []string{"outbounds", "endpoints"} {
			if object[key] == nil {
				continue
			}
			items, ok := object[key].([]interface{})
			if !ok {
				return fmt.Errorf("subscription entry %d has invalid %s", i+1, key)
			}
			for n, item := range items {
				proxy, ok := item.(map[string]interface{})
				if !ok {
					return fmt.Errorf("subscription entry %d contains an invalid proxy", i+1)
				}
				tag, _ := proxy["tag"].(string)
				if tag == "" {
					tag = fmt.Sprintf("%s-%d", key, n+1)
					proxy["tag"] = tag
				}
				if _, exists := tags[tag]; exists {
					return fmt.Errorf("subscription entry %d contains duplicate proxy tags", i+1)
				}
				tags[tag] = fmt.Sprintf("panel-%d/%s", i+1, tag)
			}
		}
		for _, key := range []string{"outbounds", "endpoints"} {
			items, _ := object[key].([]interface{})
			for _, item := range items {
				proxy := item.(map[string]interface{})
				proxy["tag"] = tags[proxy["tag"].(string)]
				remapPanelProxy(proxy, tags)
				// Importing proxy-only configs uses the application's DNS policy.
				// Per-profile resolver tags would point at discarded DNS servers.
				delete(proxy, "domain_resolver")
				if key == "endpoints" {
					endpoints = append(endpoints, proxy)
				} else {
					outbounds = append(outbounds, proxy)
				}
			}
		}
	}
	target["outbounds"], target["endpoints"] = outbounds, endpoints
	stripPanelResolverReferences(target)
	return nil
}

func remapPanelProxy(value interface{}, tags map[string]string) {
	switch object := value.(type) {
	case map[string]interface{}:
		for key, child := range object {
			switch key {
			case "detour", "default", "download_detour":
				if text, ok := child.(string); ok && tags[text] != "" {
					object[key] = tags[text]
				}
			case "outbounds":
				if values, ok := child.([]interface{}); ok {
					for i, value := range values {
						if text, ok := value.(string); ok && tags[text] != "" {
							values[i] = tags[text]
						}
					}
				}
			default:
				remapPanelProxy(child, tags)
			}
		}
	case []interface{}:
		for _, child := range object {
			remapPanelProxy(child, tags)
		}
	}
}

// Resolver tag references from a complete profile cannot survive proxy-only
// import, which intentionally uses the application's DNS configuration.
func stripPanelResolverReferences(value interface{}) {
	switch object := value.(type) {
	case map[string]interface{}:
		delete(object, "domain_resolver")
		for _, child := range object {
			stripPanelResolverReferences(child)
		}
	case []interface{}:
		for _, child := range object {
			stripPanelResolverReferences(child)
		}
	}
}
