package plugins

import (
	"strings"
)

const (
	HostCapabilityRuntimeExternalHTTPCommand = "host.runtime.external-http.command"
	HostCapabilityRuntimeExternalHTTPBaseURL = "host.runtime.external-http.base-url"
	HostCapabilityEventSubscription          = "host.event.subscription"
	HostCapabilityScheduleCron               = "host.schedule.cron"
	HostCapabilityAPIConfigRead              = "host.api.config.read"
	HostCapabilityAPIConfigWrite             = "host.api.config.write"
	HostCapabilityAPILogWrite                = "host.api.log.write"
	HostCapabilityAPINoticeWrite             = "host.api.notice.write"
	HostCapabilityAPIEventPublish            = "host.api.event.publish"
	HostCapabilityAPIKVRead                  = "host.api.kv.read"
	HostCapabilityAPIKVWrite                 = "host.api.kv.write"
	HostCapabilityAPIKVDelete                = "host.api.kv.delete"
	HostCapabilityAPISiteRead                = "host.api.site.read"
	HostCapabilityAPISiteCookie              = "host.api.site.cookie"
	HostCapabilityAPIMediaSearch             = "host.api.media.search"
	HostCapabilityUIView                     = "host.ui.view"
	HostCapabilityUIBridge                   = "host.ui.bridge"
)

var permissionHostCapabilities = map[string][]string{
	"config:read":   {HostCapabilityAPIConfigRead},
	"config:write":  {HostCapabilityAPIConfigWrite},
	"log:write":     {HostCapabilityAPILogWrite},
	"notice:write":  {HostCapabilityAPINoticeWrite},
	"event:publish": {HostCapabilityAPIEventPublish},
	"kv:read":       {HostCapabilityAPIKVRead},
	"kv:write":      {HostCapabilityAPIKVWrite},
	"kv:delete":     {HostCapabilityAPIKVDelete},
	"site:read":     {HostCapabilityAPISiteRead},
	"site:cookie":   {HostCapabilityAPISiteCookie},
	"media:search":  {HostCapabilityAPIMediaSearch},
}

func supportedHostCapabilities() []string {
	return []string{
		HostCapabilityRuntimeExternalHTTPCommand,
		HostCapabilityRuntimeExternalHTTPBaseURL,
		HostCapabilityEventSubscription,
		HostCapabilityScheduleCron,
		HostCapabilityAPIConfigRead,
		HostCapabilityAPIConfigWrite,
		HostCapabilityAPILogWrite,
		HostCapabilityAPINoticeWrite,
		HostCapabilityAPIEventPublish,
		HostCapabilityAPIKVRead,
		HostCapabilityAPIKVWrite,
		HostCapabilityAPIKVDelete,
		HostCapabilityAPISiteRead,
		HostCapabilityAPISiteCookie,
		HostCapabilityAPIMediaSearch,
		HostCapabilityUIView,
		HostCapabilityUIBridge,
	}
}

func effectiveManifestHostCapabilities(manifest Manifest) []string {
	caps := append([]string{}, manifest.HostCapabilities...)
	if manifest.Entry != nil {
		if strings.TrimSpace(manifest.Entry.Command) != "" {
			caps = append(caps, HostCapabilityRuntimeExternalHTTPCommand)
		}
		if strings.TrimSpace(manifest.Entry.BaseURL) != "" {
			caps = append(caps, HostCapabilityRuntimeExternalHTTPBaseURL)
		}
	}
	if len(manifest.Events) > 0 {
		caps = append(caps, HostCapabilityEventSubscription)
	}
	if len(manifest.Schedule) > 0 {
		caps = append(caps, HostCapabilityScheduleCron)
	}
	if manifest.Contributes != nil && len(manifest.Contributes.Views) > 0 {
		caps = append(caps, HostCapabilityUIView, HostCapabilityUIBridge)
	}
	return appendPermissionHostCapabilities(caps, manifest.Permissions)
}

func effectiveMarketHostCapabilities(item MarketPlugin) []string {
	caps := append([]string{}, item.HostCapabilities...)
	if len(item.Events) > 0 {
		caps = append(caps, HostCapabilityEventSubscription)
	}
	if item.Contributes != nil && len(item.Contributes.Views) > 0 {
		caps = append(caps, HostCapabilityUIView, HostCapabilityUIBridge)
	}
	return appendPermissionHostCapabilities(caps, item.Permissions)
}

func appendPermissionHostCapabilities(caps, permissions []string) []string {
	for _, permission := range permissions {
		for _, capability := range permissionHostCapabilities[strings.TrimSpace(permission)] {
			caps = append(caps, capability)
		}
	}
	return normalizeHostCapabilities(caps)
}

func normalizeHostCapabilities(capabilities []string) []string {
	if len(capabilities) == 0 {
		return []string{}
	}
	seen := make(map[string]bool, len(capabilities))
	out := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		capability = strings.TrimSpace(capability)
		if capability == "" || seen[capability] {
			continue
		}
		seen[capability] = true
		out = append(out, capability)
	}
	return out
}

func missingHostCapabilities(required, supported []string) []string {
	if len(required) == 0 {
		return nil
	}
	supportedSet := make(map[string]bool, len(supported))
	for _, capability := range supported {
		supportedSet[strings.TrimSpace(capability)] = true
	}
	var missing []string
	for _, capability := range normalizeHostCapabilities(required) {
		if !supportedSet[capability] {
			missing = append(missing, capability)
		}
	}
	return missing
}
