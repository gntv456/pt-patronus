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
	// HostCapabilityAPITools 是统一 Tool RPC 的伞能力（GET /tools、POST /tools/{name}、
	// Bridge tool.list/tool.call）。镜像 host.ui.bridge：一个能力，多个域级权限控制可见/可调的 tool。
	// 插件声明任意 *:read/*:write/tool 权限即获得此能力，经 hasPluginPermission 通配符匹配。
	HostCapabilityAPITools = "host.api.tools"
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
	// Tool RPC 域级权限：每个都授予统一伞能力 host.api.tools。
	// tool 级别的实际可见/可调由 registry.VisibleTools / Dispatch 经 hasPluginPermission 二次过滤。
	"downloader:read":   {HostCapabilityAPITools},
	"downloader:write":  {HostCapabilityAPITools},
	"mediaserver:read":  {HostCapabilityAPITools},
	"mediaserver:write": {HostCapabilityAPITools},
	"subscribe:read":    {HostCapabilityAPITools},
	"subscribe:write":   {HostCapabilityAPITools},
	"cloud:read":        {HostCapabilityAPITools},
	"cloud:write":       {HostCapabilityAPITools},
	"brushflow:read":    {HostCapabilityAPITools},
	"brushflow:write":   {HostCapabilityAPITools},
	"rss:read":          {HostCapabilityAPITools},
	"rss:write":         {HostCapabilityAPITools},
	"subtitle:read":     {HostCapabilityAPITools},
	"reseed:read":       {HostCapabilityAPITools},
	"reseed:write":      {HostCapabilityAPITools},
	"transfer:read":     {HostCapabilityAPITools},
	"transfer:write":    {HostCapabilityAPITools},
	"dashboard:read":    {HostCapabilityAPITools},
	"notify:send":       {HostCapabilityAPITools},
	"filter:read":       {HostCapabilityAPITools},
	"filter:write":      {HostCapabilityAPITools},
	"site:action":       {HostCapabilityAPITools},
	"media:read":        {HostCapabilityAPITools},
	"file:read":         {HostCapabilityAPITools},
	"file:write":        {HostCapabilityAPITools},
	"workflow:read":     {HostCapabilityAPITools},
	"workflow:write":    {HostCapabilityAPITools},
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
		HostCapabilityAPITools,
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
		caps = append(caps, permissionHostCapabilities[strings.TrimSpace(permission)]...)
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
