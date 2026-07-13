package chain

import "testing"

func TestClampCloudPath(t *testing.T) {
	cases := []struct {
		root string
		path string
		want string
	}{
		{"/media", "", "/media"},
		{"/media", "/", "/media"},
		{"/media", "/media/movies", "/media/movies"},
		{"/media", "/other", "/media"},
		{"/media", "/media/../../etc", "/media"},
		{"/", "/movies", "/movies"},
	}
	for _, tc := range cases {
		if got := clampCloudPath(tc.root, tc.path); got != tc.want {
			t.Errorf("clampCloudPath(%q, %q) = %q, want %q", tc.root, tc.path, got, tc.want)
		}
	}
}
