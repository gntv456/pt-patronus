package cloud

import "testing"

func TestCleanCloudPath(t *testing.T) {
	cases := map[string]string{
		"":              "/",
		"/":             "/",
		"电影":           "/电影",
		"电影/动作":        "/电影/动作",
		"/电影/":         "/电影",
		"\\电影\\动作":     "/电影/动作",
		"/a/../b":       "/b",
		"/a/./b":        "/a/b",
		"//a///b//":     "/a/b",
	}
	for in, want := range cases {
		if got := cleanCloudPath(in); got != want {
			t.Errorf("cleanCloudPath(%q)=%q want %q", in, got, want)
		}
	}
}

func TestSplitSegments(t *testing.T) {
	if ss := splitSegments("/"); len(ss) != 0 {
		t.Errorf("root should yield no segments, got %v", ss)
	}
	ss := splitSegments("/电影/动作/2024")
	if len(ss) != 3 || ss[0] != "电影" || ss[2] != "2024" {
		t.Errorf("segments mismatch: %v", ss)
	}
}

func TestJoinCloud(t *testing.T) {
	if got := joinCloud("/", "电影"); got != "/电影" {
		t.Errorf("joinCloud root: %q", got)
	}
	if got := joinCloud("/电影", "a.mkv"); got != "/电影/a.mkv" {
		t.Errorf("joinCloud nested: %q", got)
	}
}

func TestIsMediaFile(t *testing.T) {
	good := []string{"a.mkv", "A.MP4", "x.ts", "y.m2ts", "z.ISO", "movie.mov", "video.flv", "w.webm"}
	for _, n := range good {
		if !IsMediaFile(n) {
			t.Errorf("IsMediaFile(%q)=false want true", n)
		}
	}
	bad := []string{"a.srt", "b.jpg", "c.nfo", "d.txt", "e"}
	for _, n := range bad {
		if IsMediaFile(n) {
			t.Errorf("IsMediaFile(%q)=true want false", n)
		}
	}
}

func TestRenderTemplate(t *testing.T) {
	got := RenderTemplate("{mount}{cloudpath}", "/mnt/115", "/电影/a.mkv", "/a.mkv", "a.mkv")
	if got != "/mnt/115/电影/a.mkv" {
		t.Errorf("default template: %q", got)
	}
	got = RenderTemplate("{mount}{relpath}", "/mnt", "/x/a.mkv", "/a.mkv", "a.mkv")
	if got != "/mnt/a.mkv" {
		t.Errorf("relpath: %q", got)
	}
	got = RenderTemplate("{filename}", "", "", "", "a.mkv")
	if got != "a.mkv" {
		t.Errorf("filename: %q", got)
	}
}
