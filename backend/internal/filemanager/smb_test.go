package filemanager

import "testing"

func TestSplitShare(t *testing.T) {
	cases := []struct{ in, share, sub string }{
		{"", "", "/"},
		{"/", "", "/"},
		{"/Media", "Media", "/"},
		{"Media", "Media", "/"},
		{"/Media/Movies", "Media", "/Movies"},
		{"/Media/Movies/2024", "Media", "/Movies/2024"},
		{"/a/b/c", "a", "/b/c"},
	}
	for _, c := range cases {
		share, sub := splitShare(c.in)
		if share != c.share || sub != c.sub {
			t.Errorf("splitShare(%q) = (%q,%q), want (%q,%q)", c.in, share, sub, c.share, c.sub)
		}
	}
}

func TestNewSMBURL(t *testing.T) {
	s := NewSMB("smb://nas.local:4455", "u", "p", "/Media")
	if s.host != "nas.local" || s.port != 4455 {
		t.Errorf("host/port = %s/%d, want nas.local/4455", s.host, s.port)
	}
	s2 := NewSMB("smb://192.168.1.2", "", "", "/share")
	if s2.host != "192.168.1.2" || s2.port != 445 {
		t.Errorf("default port = %d, want 445", s2.port)
	}
	// rel 钳制：basePath 之外被拉回
	s3 := NewSMB("smb://h", "", "", "/Media")
	if got := s3.rel("../etc/passwd"); got != "etc/passwd" {
		t.Errorf("rel(../etc/passwd) = %q, want etc/passwd (clamped under /Media)", got)
	}
	if got := s3.rel("/"); got != "." {
		t.Errorf("rel(/) = %q, want .", got)
	}
}
