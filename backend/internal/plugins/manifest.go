package plugins

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const CurrentPluginAPIVersion = "1"

var pluginIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{1,127}$`)

func validateExternalManifest(manifest Manifest) error {
	var errs []error
	if strings.TrimSpace(manifest.ID) == "" {
		errs = append(errs, errors.New("id is required"))
	} else if !pluginIDPattern.MatchString(manifest.ID) {
		errs = append(errs, fmt.Errorf("id %q must match %s", manifest.ID, pluginIDPattern.String()))
	}
	if strings.TrimSpace(manifest.Name) == "" {
		errs = append(errs, errors.New("name is required"))
	}
	if strings.TrimSpace(manifest.Version) == "" {
		errs = append(errs, errors.New("version is required"))
	}
	if compatibility := evaluateManifestCompatibility(manifest); !compatibility.Compatible {
		errs = append(errs, errors.New(compatibility.Reason))
	}
	if manifest.Runtime != RuntimeExternalHTTP {
		errs = append(errs, fmt.Errorf("runtime must be %q", RuntimeExternalHTTP))
	}
	if manifest.Entry == nil {
		errs = append(errs, errors.New("entry is required"))
	} else if strings.TrimSpace(manifest.Entry.Command) == "" && strings.TrimSpace(manifest.Entry.BaseURL) == "" {
		errs = append(errs, errors.New("entry.command or entry.base_url is required"))
	}
	for i, perm := range manifest.Permissions {
		perm = strings.TrimSpace(perm)
		if perm == "" {
			errs = append(errs, fmt.Errorf("permissions[%d] is empty", i))
		}
		if strings.ContainsAny(perm, " \t\r\n") {
			errs = append(errs, fmt.Errorf("permissions[%d] %q must not contain whitespace", i, perm))
		}
		// 白名单校验：拒绝无具体能力的裸通配（* / ** / 裸 host: / host:* / plugin:*）。
		// host:* 与 plugin:* 在 ACL(external.go hasPluginPermission) 中被特判为全局超级授权，
		// 等价 *，声明即静默获得全部权限——故禁止；具体能力（config:read）与域级通配
		// （downloader:*）允许。
		if perr := validatePermissionString(perm); perr != nil {
			errs = append(errs, fmt.Errorf("permissions[%d]: %w", i, perr))
		}
	}
	seenDeps := make(map[string]bool, len(manifest.Dependencies))
	for i, dep := range manifest.Dependencies {
		id := strings.TrimSpace(dep.ID)
		if id == "" {
			errs = append(errs, fmt.Errorf("dependencies[%d].id is required", i))
			continue
		}
		if id == manifest.ID {
			errs = append(errs, fmt.Errorf("dependencies[%d].id must not reference the plugin itself", i))
		}
		if !pluginIDPattern.MatchString(id) {
			errs = append(errs, fmt.Errorf("dependencies[%d].id %q must match %s", i, id, pluginIDPattern.String()))
		}
		if seenDeps[id] {
			errs = append(errs, fmt.Errorf("dependency %q is duplicated", id))
		}
		seenDeps[id] = true
		if dep.Version != "" {
			if _, _, err := parseVersionConstraint(dep.Version, dep.Version, "dependency version"); err != nil {
				errs = append(errs, fmt.Errorf("dependencies[%d].version: %w", i, err))
			}
		}
	}
	for i, capability := range manifest.OptionalHostCapabilities {
		if strings.TrimSpace(capability) == "" {
			errs = append(errs, fmt.Errorf("optional_host_capabilities[%d] is empty", i))
		}
	}
	actionNames := make(map[string]bool, len(manifest.Actions))
	for i, action := range manifest.Actions {
		name := strings.TrimSpace(action.Name)
		if name == "" {
			errs = append(errs, fmt.Errorf("actions[%d].name is required", i))
			continue
		}
		if actionNames[name] {
			errs = append(errs, fmt.Errorf("action %q is duplicated", name))
		}
		actionNames[name] = true
	}
	for i, ev := range manifest.Events {
		if strings.TrimSpace(ev.Type) == "" {
			errs = append(errs, fmt.Errorf("events[%d].type is required", i))
		}
	}
	for i, schedule := range manifest.Schedule {
		if strings.TrimSpace(schedule.ID) == "" {
			errs = append(errs, fmt.Errorf("schedule[%d].id is required", i))
		}
		if strings.TrimSpace(schedule.Action) == "" {
			errs = append(errs, fmt.Errorf("schedule[%d].action is required", i))
		} else if !actionNames[schedule.Action] {
			errs = append(errs, fmt.Errorf("schedule[%d].action %q is not declared in actions", i, schedule.Action))
		}
	}
	if manifest.Contributes != nil {
		for i, cmd := range manifest.Contributes.Commands {
			if strings.TrimSpace(cmd.ID) == "" {
				errs = append(errs, fmt.Errorf("contributes.commands[%d].id is required", i))
			}
			if strings.TrimSpace(cmd.Action) != "" && !actionNames[cmd.Action] {
				errs = append(errs, fmt.Errorf("contributes.commands[%d].action %q is not declared in actions", i, cmd.Action))
			}
		}
		for i, view := range manifest.Contributes.Views {
			if strings.TrimSpace(view.ID) == "" {
				errs = append(errs, fmt.Errorf("contributes.views[%d].id is required", i))
			}
			if strings.TrimSpace(view.Title) == "" {
				errs = append(errs, fmt.Errorf("contributes.views[%d].title is required", i))
			}
		}
		for i, menu := range manifest.Contributes.Menus {
			if strings.TrimSpace(menu.Location) == "" {
				errs = append(errs, fmt.Errorf("contributes.menus[%d].location is required", i))
			}
			if strings.TrimSpace(menu.Command) == "" {
				errs = append(errs, fmt.Errorf("contributes.menus[%d].command is required", i))
			}
		}
	}
	// resource_access 是声明（非强制）；只校验非空字符串，帮开发者尽早发现笔误。
	if ra := manifest.ResourceAccess; ra != nil {
		for i, p := range ra.FsPaths {
			if strings.TrimSpace(p) == "" {
				errs = append(errs, fmt.Errorf("resource_access.fs_paths[%d] is empty", i))
			}
		}
		for i, b := range ra.ExecBinaries {
			if strings.TrimSpace(b) == "" {
				errs = append(errs, fmt.Errorf("resource_access.exec_binaries[%d] is empty", i))
			}
		}
		for i, h := range ra.NetHosts {
			if strings.TrimSpace(h) == "" {
				errs = append(errs, fmt.Errorf("resource_access.net_hosts[%d] is empty", i))
			}
		}
	}
	for i, req := range manifest.Requirements {
		if strings.TrimSpace(req) == "" {
			errs = append(errs, fmt.Errorf("requirements[%d] is empty", i))
		}
	}
	return errors.Join(errs...)
}

// validatePermissionString 校验单个权限字符串，拒绝无具体能力的裸通配。
// 合法形式为 <domain>:<capability>：
//   - 具体能力：config:read、kv:write、site:cookie
//   - 域级通配：downloader:*（仅授予 downloader 域）
//
// 拒绝（在 ACL 中等价全局超级授权或为空，声明即静默获得全部权限）：
//   - 纯通配：* / ** / ***
//   - 裸前缀：host: / plugin:（冒号后为空）
//   - 超级授权：host:* / plugin:*（hasPluginPermission 特判为全局授权）
//
// host/plugin 两域的通配被特别拒绝，因其等价 *；其余域的 :* 保留为合法域级通配。
func validatePermissionString(perm string) error {
	perm = strings.TrimSpace(perm)
	if perm == "" {
		return errors.New("is empty")
	}
	// 纯通配（无域）：*、**、***……
	if strings.Trim(perm, "*") == "" {
		return fmt.Errorf("%q is a bare global wildcard; declare specific <domain>:<capability> instead", perm)
	}
	domain, capability, found := strings.Cut(perm, ":")
	domain = strings.TrimSpace(domain)
	capability = strings.TrimSpace(capability)
	if !found || domain == "" || capability == "" {
		return fmt.Errorf("%q must be in <domain>:<capability> form", perm)
	}
	// host:* / plugin:* 在 ACL 中被特判为全局超级授权（等价 *），拒绝。
	if (domain == "host" || domain == "plugin") && strings.Trim(capability, "*") == "" {
		return fmt.Errorf("%q is a global super-grant; use specific %s:<capability> instead", perm, domain)
	}
	return nil
}

func PermissionSetEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int, len(a))
	for _, item := range a {
		seen[strings.TrimSpace(item)]++
	}
	for _, item := range b {
		key := strings.TrimSpace(item)
		if seen[key] == 0 {
			return false
		}
		seen[key]--
	}
	return true
}
