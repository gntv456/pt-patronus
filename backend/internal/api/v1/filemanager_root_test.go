// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import "testing"

func TestIsRootPathCanonicalVariants(t *testing.T) {
	for _, input := range []string{"", " ", "/", "///", "/.", "/..", "/a/..", `\\.`, `\\..\\`} {
		if !isRootPath(input) {
			t.Errorf("isRootPath(%q) = false", input)
		}
	}
	for _, input := range []string{"/a", "/../a", "a/.. /b"} {
		if isRootPath(input) {
			t.Errorf("isRootPath(%q) = true", input)
		}
	}
}
