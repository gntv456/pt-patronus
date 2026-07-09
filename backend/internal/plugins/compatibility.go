package plugins

import (
	"fmt"
	"strings"

	appversion "github.com/hxpt/ptpatronus/internal/version"
)

type Compatibility struct {
	Compatible bool
	Reason     string
}

func currentAppVersion() string {
	return strings.TrimSpace(appversion.Version)
}

func evaluateManifestCompatibility(manifest Manifest) Compatibility {
	return evaluateCompatibility(manifest.APIVersion, manifest.AppVersion, effectiveManifestHostCapabilities(manifest))
}

func evaluateMarketCompatibility(item MarketPlugin) Compatibility {
	apiVersion := strings.TrimSpace(item.APIVersion)
	if apiVersion == "" {
		apiVersion = CurrentPluginAPIVersion
	}
	return evaluateCompatibility(apiVersion, item.AppVersion, effectiveMarketHostCapabilities(item))
}

func evaluateCompatibility(apiVersion, appVersion string, hostCapabilities []string) Compatibility {
	if err := validateAPIVersion(apiVersion); err != nil {
		return Compatibility{Reason: err.Error()}
	}
	if err := validateAppVersion(appVersion, currentAppVersion()); err != nil {
		return Compatibility{Reason: err.Error()}
	}
	if err := validateHostCapabilities(hostCapabilities, supportedHostCapabilities()); err != nil {
		return Compatibility{Reason: err.Error()}
	}
	return Compatibility{Compatible: true}
}

func validateAPIVersion(apiVersion string) error {
	apiVersion = strings.TrimSpace(apiVersion)
	if apiVersion == "" {
		return nil
	}
	if apiVersion != CurrentPluginAPIVersion {
		return fmt.Errorf("requires plugin API %s; current is %s", apiVersion, CurrentPluginAPIVersion)
	}
	return nil
}

func validateAppVersion(expr, current string) error {
	return validateVersionRequirement("app_version", expr, current)
}

func validateDependencyVersion(id, expr, current string) error {
	if strings.TrimSpace(id) == "" {
		id = "unknown"
	}
	if err := validateVersionRequirement("dependency "+id+" version", expr, current); err != nil {
		return err
	}
	return nil
}

func validateVersionRequirement(label, expr, current string) error {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil
	}
	current = strings.TrimSpace(current)
	if current == "" {
		return fmt.Errorf("current version is unknown for %s %s", label, expr)
	}
	clauses := strings.Split(expr, ",")
	for _, clause := range clauses {
		op, want, err := parseVersionConstraint(clause, expr, label)
		if err != nil {
			return err
		}
		if !matchVersionConstraint(compareLooseVersions(current, want), op) {
			return fmt.Errorf("requires %s %s; current is %s", label, expr, current)
		}
	}
	return nil
}

func validateHostCapabilities(required, supported []string) error {
	missing := missingHostCapabilities(required, supported)
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("requires host capabilities %s", strings.Join(missing, ", "))
}

func parseVersionConstraint(clause, fullExpr, label string) (string, string, error) {
	clause = strings.TrimSpace(clause)
	if clause == "" {
		return "", "", fmt.Errorf("invalid %s constraint %q", label, fullExpr)
	}
	for _, op := range []string{">=", "<=", "==", ">", "<", "="} {
		if strings.HasPrefix(clause, op) {
			want := strings.TrimSpace(strings.TrimPrefix(clause, op))
			if want == "" || strings.HasPrefix(want, ">") || strings.HasPrefix(want, "<") || strings.HasPrefix(want, "=") {
				return "", "", fmt.Errorf("invalid %s constraint %q", label, fullExpr)
			}
			if op == "==" {
				op = "="
			}
			return op, want, nil
		}
	}
	first := clause[0]
	if (first < '0' || first > '9') && first != 'v' && first != 'V' {
		return "", "", fmt.Errorf("invalid %s constraint %q", label, fullExpr)
	}
	return "=", clause, nil
}

func matchVersionConstraint(cmp int, op string) bool {
	switch op {
	case ">":
		return cmp > 0
	case ">=":
		return cmp >= 0
	case "<":
		return cmp < 0
	case "<=":
		return cmp <= 0
	case "=":
		return cmp == 0
	default:
		return false
	}
}

func compareLooseVersions(a, b string) int {
	sa := splitLooseVersion(a)
	sb := splitLooseVersion(b)
	n := len(sa)
	if len(sb) > n {
		n = len(sb)
	}
	for i := 0; i < n; i++ {
		var x, y int
		if i < len(sa) {
			x = sa[i]
		}
		if i < len(sb) {
			y = sb[i]
		}
		switch {
		case x < y:
			return -1
		case x > y:
			return 1
		}
	}
	return 0
}

func splitLooseVersion(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	parts := strings.FieldsFunc(v, func(r rune) bool {
		return r == '.' || r == '-' || r == '_' || r == '+'
	})
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		out = append(out, leadingLooseInt(part))
	}
	return out
}

func leadingLooseInt(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}
