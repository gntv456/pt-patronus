// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hxpt/ptpatronus/internal/site"
)

func TestDefaultDefinitionsDirFallsBackToBackendDefinitions(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	tmp := t.TempDir()
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	})
	if err := os.MkdirAll(filepath.Join(tmp, "backend", "definitions"), 0o755); err != nil {
		t.Fatalf("mkdir backend definitions: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}

	got := filepath.Clean(defaultDefinitionsDir())
	want := filepath.Clean("./backend/definitions")
	if got != want {
		t.Fatalf("defaultDefinitionsDir() = %q, want %q", got, want)
	}
}

func TestDefaultDefinitionsDirPrefersWorkingDefinitions(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	tmp := t.TempDir()
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	})
	for _, dir := range []string{"definitions", filepath.Join("backend", "definitions")} {
		if err := os.MkdirAll(filepath.Join(tmp, dir), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}

	got := filepath.Clean(defaultDefinitionsDir())
	want := filepath.Clean("./definitions")
	if got != want {
		t.Fatalf("defaultDefinitionsDir() = %q, want %q", got, want)
	}
}

func TestLoadSiteDefinitionsUsesBundledDefinitions(t *testing.T) {
	reg := site.NewRegistry()

	loadSiteDefinitions(reg, filepath.Join(t.TempDir(), "missing"), false, "", "")

	def, ok := reg.Definition("pthome")
	if !ok {
		t.Fatal("bundled pthome definition missing")
	}
	if def.Name != "铂金家" {
		t.Fatalf("pthome name = %q, want 铂金家", def.Name)
	}
	assertHasURL(t, def.URLs, "https://pthome.net")
}

func TestLoadSiteDefinitionsAllowsExternalOverrides(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pthome.yaml"), []byte(`
id: pthome
name: External PTHome
schema: nexusphp
urls:
  - https://external-pthome.example
`), 0o644); err != nil {
		t.Fatalf("write external definition: %v", err)
	}
	reg := site.NewRegistry()

	loadSiteDefinitions(reg, dir, true, "", "")

	def, ok := reg.Definition("pthome")
	if !ok {
		t.Fatal("pthome definition missing")
	}
	if def.Name != "External PTHome" {
		t.Fatalf("pthome name = %q, want external override", def.Name)
	}
	assertHasURL(t, def.URLs, "https://external-pthome.example")
}

func assertHasURL(t *testing.T, got []string, want string) {
	t.Helper()
	for _, u := range got {
		if u == want {
			return
		}
	}
	t.Fatalf("urls = %#v, want %s", got, want)
}
