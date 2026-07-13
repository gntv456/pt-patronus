package cloud

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeClient 实现固定目录树，供 STRM 生成器测试（无需真实 HTTP）。
type fakeClient struct {
	tree map[string][]File
}

func (f *fakeClient) Type() string                          { return "fake" }
func (f *fakeClient) Test(context.Context) (string, error)  { return "fake", nil }
func (f *fakeClient) Quota(context.Context) (*Quota, error) { return &Quota{}, nil }
func (f *fakeClient) List(_ context.Context, dir string) ([]File, error) {
	return f.tree[cleanCloudPath(dir)], nil
}

func newFakeTree() *fakeClient {
	return &fakeClient{tree: map[string][]File{
		"/": {
			{Name: "电影", Path: "/电影", IsDir: true},
			{Name: "readme.txt", Path: "/readme.txt", Size: 10}, // 非媒体，应跳过
		},
		"/电影": {
			{Name: "Inception.2010.1080p.mkv", Path: "/电影/Inception.2010.1080p.mkv", Size: 1000},
		},
	}}
}

func TestGenerateStrmBasic(t *testing.T) {
	tmp := t.TempDir()
	cli := newFakeTree()
	var written []string
	res, err := Generate(context.Background(), cli, GenerateOptions{
		CloudPath: "/", OutputDir: tmp, MountRoot: "/mnt/115", Recursive: true, KeepExt: true,
		OnFile: func(w bool, cloudPath, strmPath string, size int64) {
			if w {
				written = append(written, strmPath)
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 1 || res.Skipped != 0 {
		t.Errorf("generated=%d skipped=%d, want 1/0", res.Generated, res.Skipped)
	}
	strm := filepath.Join(tmp, "电影", "Inception.2010.1080p.mkv.strm")
	if len(written) != 1 || written[0] != strm {
		t.Errorf("written=%v want [%s]", written, strm)
	}
	body, err := os.ReadFile(strm)
	if err != nil {
		t.Fatal(err)
	}
	want := "/mnt/115/电影/Inception.2010.1080p.mkv\n"
	if string(body) != want {
		t.Errorf("strm content=%q want %q", body, want)
	}
}

func TestGenerateStrmIdempotent(t *testing.T) {
	tmp := t.TempDir()
	cli := newFakeTree()
	opts := GenerateOptions{CloudPath: "/", OutputDir: tmp, MountRoot: "/mnt", Recursive: true, KeepExt: true}
	if _, err := Generate(context.Background(), cli, opts); err != nil {
		t.Fatal(err)
	}
	res, err := Generate(context.Background(), cli, opts)
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 0 || res.Skipped != 1 {
		t.Errorf("second run generated=%d skipped=%d, want 0/1", res.Generated, res.Skipped)
	}
}

func TestGenerateStrmNonRecursive(t *testing.T) {
	tmp := t.TempDir()
	cli := newFakeTree()
	res, err := Generate(context.Background(), cli, GenerateOptions{
		CloudPath: "/", OutputDir: tmp, MountRoot: "/mnt", Recursive: false, KeepExt: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Generated != 0 {
		t.Errorf("non-recursive should not recurse into /电影, generated=%d", res.Generated)
	}
}

func TestGenerateStrmStripExt(t *testing.T) {
	tmp := t.TempDir()
	cli := newFakeTree()
	_, err := Generate(context.Background(), cli, GenerateOptions{
		CloudPath: "/", OutputDir: tmp, MountRoot: "/mnt", Recursive: true, KeepExt: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	stripped := filepath.Join(tmp, "电影", "Inception.2010.1080p.strm")
	if _, err := os.Stat(stripped); err != nil {
		t.Errorf("stripped-ext strm not created: %v", err)
	}
	withExt := filepath.Join(tmp, "电影", "Inception.2010.1080p.mkv.strm")
	if _, err := os.Stat(withExt); err == nil {
		t.Errorf("keep-ext strm should NOT exist when KeepExt=false")
	}
}

// TestWriteStrmRejectsTraversal 防御纵深：云条目路径含 ".." 时，.strm 不得逃出 OutputDir。
func TestWriteStrmRejectsTraversal(t *testing.T) {
	tmp := t.TempDir()
	// 恶意云路径：CloudPath 下相对路径含 ..，试图逃出 OutputDir 写到上级。
	f := File{Name: "x.mkv", Path: "/电影/../../pwned.mkv", Size: 1}
	_, _, err := writeStrm(GenerateOptions{
		CloudPath: "/电影", OutputDir: tmp, MountRoot: "/mnt", KeepExt: true,
	}, f)
	if err == nil {
		t.Fatal("writeStrm should reject path escaping OutputDir")
	}
	// 确认没有文件被写到 OutputDir 之外。
	if _, err := os.Stat(filepath.Join(tmp, "..", "pwned.mkv.strm")); err == nil {
		t.Fatal("traversal wrote a file outside OutputDir")
	}
}

func TestValidateStrmOutputDirRejectsFilesystemRoots(t *testing.T) {
	root := string(os.PathSeparator)
	if runtime.GOOS == "windows" {
		root = filepath.VolumeName(t.TempDir()) + string(os.PathSeparator)
	}
	if err := validateStrmOutputDir(root); err == nil {
		t.Fatalf("filesystem root %q should be rejected", root)
	}
}

func TestValidateStrmOutputDirWindowsCaseInsensitive(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows path semantics")
	}
	path := strings.ToLower(`C:\Windows\Temp\ptpatronus`)
	if err := validateStrmOutputDir(path); err == nil {
		t.Fatalf("case-variant sensitive path %q should be rejected", path)
	}
}
