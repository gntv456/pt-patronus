package transfer

import "testing"

type kvTag map[string]string

func (k kvTag) Get(key string) (string, error) { return k[key], nil }
func (k kvTag) Set(key, val string) error      { k[key] = val; return nil }

func TestDirTagsRoundtripAndResolve(t *testing.T) {
	kv := kvTag{}
	ts := []DirTag{
		{Name: "movies", Kind: "local", Path: "/media/movies"},
		{Name: "cloud115", Kind: "cloud", DriveID: 3, Path: "/媒体库"},
	}
	if err := SaveDirTags(kv, ts); err != nil {
		t.Fatal(err)
	}
	got := LoadDirTags(kv)
	if len(got) != 2 || got[0].Name != "movies" || got[1].DriveID != 3 {
		t.Errorf("roundtrip: %+v", got)
	}
	r, err := ResolveDirTag(kv, "cloud115")
	if err != nil || r.Path != "/媒体库" {
		t.Errorf("resolve: %+v err=%v", r, err)
	}
	if _, err := ResolveDirTag(kv, "nope"); err == nil {
		t.Error("missing tag should error")
	}
}

func TestDirTagsValidation(t *testing.T) {
	kv := kvTag{}
	if err := SaveDirTags(kv, []DirTag{{Name: "a", Kind: "bad", Path: "/x"}}); err == nil {
		t.Error("bad kind should fail")
	}
	if err := SaveDirTags(kv, []DirTag{{Name: "a", Kind: "local"}, {Name: "a", Kind: "local", Path: "/x"}}); err == nil {
		t.Error("duplicate name should fail")
	}
	if err := SaveDirTags(kv, []DirTag{{Name: "", Kind: "local", Path: "/x"}}); err == nil {
		t.Error("empty name should fail")
	}
	// 脏数据回空表（不 panic）
	kv[dirTagsKey] = "{bad"
	if got := LoadDirTags(kv); got != nil {
		t.Errorf("bad json should be nil, got %+v", got)
	}
}
