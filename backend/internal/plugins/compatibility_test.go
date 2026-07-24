package plugins

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/event"
)

func TestValidateAppVersion(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		expr    string
		current string
		wantErr string
	}{
		{name: "blank", expr: "", current: "0.81.01"},
		{name: "minimum", expr: ">=0.1.0", current: "0.81.01"},
		{name: "range", expr: ">=0.1.0, <1.0.0", current: "0.81.01"},
		{name: "exact", expr: "0.81.01", current: "0.81.01"},
		{name: "too_new", expr: ">=0.90.0", current: "0.81.01", wantErr: "requires app_version"},
		{name: "invalid_double_operator", expr: ">>0.1.0", current: "0.81.01", wantErr: "invalid app_version constraint"},
		{name: "invalid_prefix", expr: "!0.1.0", current: "0.81.01", wantErr: "invalid app_version constraint"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateAppVersion(tc.expr, tc.current)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("validateAppVersion(%q, %q) unexpected error: %v", tc.expr, tc.current, err)
			}
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("validateAppVersion(%q, %q) expected error containing %q", tc.expr, tc.current, tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("validateAppVersion(%q, %q) error %q does not contain %q", tc.expr, tc.current, err.Error(), tc.wantErr)
				}
			}
		})
	}
}

func TestValidateExternalManifestCompatibility(t *testing.T) {
	t.Parallel()

	base := Manifest{
		ID:         "compat-demo",
		Name:       "Compat Demo",
		Version:    "1.0.0",
		APIVersion: CurrentPluginAPIVersion,
		Runtime:    RuntimeExternalHTTP,
		Entry:      &RuntimeEntry{BaseURL: "http://127.0.0.1:19090", Health: "/health"},
	}

	if err := validateExternalManifest(base); err != nil {
		t.Fatalf("expected compatible manifest, got %v", err)
	}

	apiMismatch := base
	apiMismatch.APIVersion = "2"
	if err := validateExternalManifest(apiMismatch); err == nil || !strings.Contains(err.Error(), "requires plugin API 2") {
		t.Fatalf("expected api version error, got %v", err)
	}

	appMismatch := base
	appMismatch.AppVersion = ">=9.0.0"
	if err := validateExternalManifest(appMismatch); err == nil || !strings.Contains(err.Error(), "requires app_version >=9.0.0") {
		t.Fatalf("expected app version error, got %v", err)
	}

	hostCapabilityMismatch := base
	hostCapabilityMismatch.HostCapabilities = []string{"host.ui.command"}
	if err := validateExternalManifest(hostCapabilityMismatch); err == nil || !strings.Contains(err.Error(), "requires host capabilities host.ui.command") {
		t.Fatalf("expected host capability error, got %v", err)
	}
}

func TestValidatePermissionString(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		perm    string
		wantErr bool
	}{
		{"specific capability", "config:read", false},
		{"domain wildcard", "downloader:*", false},
		{"kv wildcard", "kv:*", false},
		{"global star", "*", true},
		{"double star", "**", true},
		{"bare host prefix", "host:", true},
		{"bare plugin prefix", "plugin:", true},
		{"host super grant", "host:*", true},
		{"plugin super grant", "plugin:*", true},
		{"no colon", "network", true},
		{"empty capability", "config:", true},
		{"whitespace only", "  ", true},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			err := validatePermissionString(c.perm)
			if c.wantErr && err == nil {
				t.Errorf("validatePermissionString(%q) expected error, got nil", c.perm)
			}
			if !c.wantErr && err != nil {
				t.Errorf("validatePermissionString(%q) unexpected error: %v", c.perm, err)
			}
		})
	}
}

// TestValidateExternalManifest_RejectsWildcardPermissions 验证 manifest 权限白名单：
// 含裸通配（*/host:*/plugin:*）的 manifest 被 validateExternalManifest 拒绝，具体能力放行。
func TestValidateExternalManifest_RejectsWildcardPermissions(t *testing.T) {
	t.Parallel()
	base := Manifest{
		ID:         "perm-demo",
		Name:       "Perm Demo",
		Version:    "1.0.0",
		APIVersion: CurrentPluginAPIVersion,
		Runtime:    RuntimeExternalHTTP,
		Entry:      &RuntimeEntry{BaseURL: "http://127.0.0.1:19090", Health: "/health"},
	}
	good := base
	good.Permissions = []string{"config:read", "downloader:*"}
	if err := validateExternalManifest(good); err != nil {
		t.Fatalf("specific permissions should pass, got %v", err)
	}
	for _, bad := range []string{"*", "host:*", "plugin:*", "host:"} {
		m := base
		m.Permissions = []string{"config:read", bad}
		if err := validateExternalManifest(m); err == nil {
			t.Errorf("manifest with permission %q should be rejected", bad)
		}
	}
}

func TestEffectiveManifestHostCapabilities(t *testing.T) {
	t.Parallel()

	manifest := Manifest{
		Entry: &RuntimeEntry{Command: "python", BaseURL: "http://127.0.0.1:19090"},
		Permissions: []string{
			"config:read",
			"kv:write",
		},
		Events: []EventSpec{{Type: "plugin.test"}},
		Schedule: []ScheduleSpec{{
			ID:     "daily",
			Action: "ping",
			Cron:   "0 0 * * *",
		}},
		Contributes: &ContributionSpec{
			Views: []ViewContribution{{ID: "dashboard", Title: "Dashboard"}},
		},
		HostCapabilities: []string{"host.custom.future"},
	}

	got := effectiveManifestHostCapabilities(manifest)
	want := []string{
		"host.custom.future",
		HostCapabilityRuntimeExternalHTTPCommand,
		HostCapabilityRuntimeExternalHTTPBaseURL,
		HostCapabilityEventSubscription,
		HostCapabilityScheduleCron,
		HostCapabilityUIView,
		HostCapabilityUIBridge,
		HostCapabilityAPIConfigRead,
		HostCapabilityAPIKVWrite,
	}
	for _, capability := range want {
		if !strings.Contains(strings.Join(got, ","), capability) {
			t.Fatalf("expected host capability %s in %#v", capability, got)
		}
	}
}

func TestFetchMarketCompatibilityAndInstallGuard(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name             string
		apiVersion       string
		appVersion       string
		hostCapabilities []string
		wantReason       string
	}{
		{
			name:       "api_mismatch",
			apiVersion: "2",
			wantReason: "requires plugin API 2",
		},
		{
			name:       "app_mismatch",
			apiVersion: CurrentPluginAPIVersion,
			appVersion: ">=9.0.0",
			wantReason: "requires app_version >=9.0.0",
		},
		{
			name:             "host_capability_mismatch",
			apiVersion:       CurrentPluginAPIVersion,
			hostCapabilities: []string{"host.ui.command"},
			wantReason:       "requires host capabilities host.ui.command",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			index := MarketIndex{
				Version: 1,
				Plugins: []MarketPlugin{{
					ID:               "market-incompatible",
					Name:             "Market Incompatible",
					Version:          "1.0.0",
					APIVersion:       tc.apiVersion,
					AppVersion:       tc.appVersion,
					HostCapabilities: tc.hostCapabilities,
					Runtime:          RuntimeExternalHTTP,
					Archive:          "archives/market-incompatible.zip",
					Permissions:      []string{"event:read"},
				}},
			}
			rawIndex, _ := json.Marshal(index)
			indexPath := filepath.Join(root, "plugin-market.json")
			if err := os.WriteFile(indexPath, rawIndex, 0o644); err != nil {
				t.Fatalf("write index: %v", err)
			}

			db := newPluginTestDB(t)
			m := NewManager(db, event.New(), nil, nil, nil)
			m.SetPluginRoot(filepath.Join(root, "plugins"))

			market, err := m.FetchMarket(context.Background(), indexPath)
			if err != nil {
				t.Fatalf("fetch market: %v", err)
			}
			if market.APIVersion != CurrentPluginAPIVersion {
				t.Fatalf("expected market api version %s, got %s", CurrentPluginAPIVersion, market.APIVersion)
			}
			if market.AppVersion != currentAppVersion() {
				t.Fatalf("expected market app version %s, got %s", currentAppVersion(), market.AppVersion)
			}
			item := market.Plugins[0]
			if item.Compatible {
				t.Fatalf("expected incompatible market item, got %#v", item)
			}
			if !strings.Contains(item.CompatibilityReason, tc.wantReason) {
				t.Fatalf("expected reason %q, got %q", tc.wantReason, item.CompatibilityReason)
			}
			if _, err := m.InstallFromMarket(context.Background(), indexPath, item.ID, item.Permissions); err == nil || !strings.Contains(err.Error(), "cannot be installed") {
				t.Fatalf("expected incompatible install error, got %v", err)
			}
		})
	}
}
