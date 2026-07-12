package chain

import (
	"testing"

	"github.com/hxpt/ptpatronus/internal/trakt"
)

// fakeTraktStore 是 SystemConfigOper 的内存替身。
type fakeTraktStore struct{ m map[string]string }

func (f *fakeTraktStore) Get(key string) (string, error) { return f.m[key], nil }
func (f *fakeTraktStore) Set(key, value string) error {
	if f.m == nil {
		f.m = map[string]string{}
	}
	f.m[key] = value
	return nil
}

func TestTraktSettingsRoundTrip(t *testing.T) {
	store := &fakeTraktStore{}
	c := &TraktChain{store: store, client: trakt.NewClient("")}

	in := defaultTraktSettings()
	in.ClientID = "cid"
	in.ClientSecret = "sec"
	in.Enabled = true
	in.SyncMovies = true
	in.SyncShows = false
	if err := c.SaveSettings(in); err != nil {
		t.Fatalf("save: %v", err)
	}
	out := c.GetSettings()
	if out.ClientID != "cid" || out.ClientSecret != "sec" || !out.Enabled || out.SyncShows != false || out.SyncMovies != true {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
	// 默认值在空 store 上仍生效
	store2 := &fakeTraktStore{}
	c2 := &TraktChain{store: store2, client: trakt.NewClient("")}
	d := c2.GetSettings()
	if !d.SyncMovies || !d.SyncShows || d.RedirectURI == "" {
		t.Fatalf("defaults lost: %+v", d)
	}
}

func TestTraktSettingsMappingDefaults(t *testing.T) {
	d := defaultTraktSettings()
	if d.SyncMovies != true || d.SyncShows != true || d.RedirectURI != "urn:ietf:wg:oauth:2.0:oob" {
		t.Fatalf("bad defaults: %+v", d)
	}
}
