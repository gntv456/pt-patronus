// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package filemanager

import (
	"context"
	"testing"
)

func TestRemoteBackendsRejectRootVariantsBeforeIO(t *testing.T) {
	ctx := context.Background()
	rclone, err := NewRclone("", "remote", "", "/vault")
	if err != nil {
		t.Fatal(err)
	}
	backends := []struct {
		name    string
		backend StorageBackend
	}{
		{"alist", NewAlist("http://127.0.0.1:1", "", "", "/vault")},
		{"webdav", NewWebDAV("http://127.0.0.1:1", "", "", "/vault")},
		{"rclone", rclone},
		{"smb", NewSMB("smb://127.0.0.1:1", "", "", "/vault")},
		{"agent", NewAgentBackend(nil, 1, "/vault")},
	}
	for _, item := range backends {
		t.Run(item.name, func(t *testing.T) {
			for _, root := range []string{"/", "/.", "/..", `\\.`, "/vault", "/vault/."} {
				if err := item.backend.Delete(ctx, root); err == nil {
					t.Errorf("Delete(%q) should reject storage root", root)
				}
				if err := item.backend.Rename(ctx, root, "renamed"); err == nil {
					t.Errorf("Rename(%q) should reject storage root", root)
				}
				if err := item.backend.Move(ctx, root, "/target"); err == nil {
					t.Errorf("Move(%q) should reject storage root", root)
				}
				if err := item.backend.Copy(ctx, root, "/target"); err == nil {
					t.Errorf("Copy(%q) should reject storage root", root)
				}
			}
		})
	}
}
