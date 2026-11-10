// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package manager

import (
	"context"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

func newTestManager(t *testing.T) (*Manager, *oper.DownloaderOper) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/manager.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Downloader{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	op := oper.NewDownloaderOper(db, security.NewCrypto("manager-test-seed"))
	return NewManager(op), op
}

func TestBuildClient(t *testing.T) {
	tests := []struct {
		typ      string
		wantType string
		wantErr  bool
	}{
		{typ: "qb", wantType: "qb"},
		{typ: "qbittorrent", wantType: "qb"},
		{typ: "tr", wantType: "tr"},
		{typ: "transmission", wantType: "tr"},
		{typ: "ut", wantType: "ut"},
		{typ: "utorrent", wantType: "ut"},
		{typ: "dl", wantType: "dl"},
		{typ: "deluge", wantType: "dl"},
		{typ: "ar", wantType: "ar"},
		{typ: "aria2", wantType: "ar"},
		{typ: "rt", wantType: "rt"},
		{typ: "rutorrent", wantType: "rt"},
		{typ: "fl", wantType: "fl"},
		{typ: "flood", wantType: "fl"},
		{typ: "unknown", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.typ, func(t *testing.T) {
			client, err := BuildClient(&models.Downloader{Type: tt.typ, URL: "http://127.0.0.1:8080"}, "secret")
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if client.Type() != tt.wantType {
				t.Fatalf("type = %q, want %q", client.Type(), tt.wantType)
			}
		})
	}
}

func TestManagerGetCacheInvalidateAndMissing(t *testing.T) {
	manager, op := newTestManager(t)
	record, err := op.Create(oper.DownloaderInput{
		Name: "qB", Type: "qb", URL: "http://127.0.0.1:8080", Username: "user", Password: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}

	first, err := manager.Get(context.Background(), record.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Get(context.Background(), record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("cached Get returned a different client")
	}

	manager.Invalidate(record.ID)
	third, err := manager.Get(context.Background(), record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if first == third {
		t.Fatal("Invalidate did not rebuild client")
	}

	if _, err := manager.Get(context.Background(), record.ID+999); err == nil {
		t.Fatal("missing downloader should return an error")
	}
}

func TestManagerConcurrentGetUsesSingleClient(t *testing.T) {
	manager, op := newTestManager(t)
	record, err := op.Create(oper.DownloaderInput{Name: "TR", Type: "tr", URL: "http://127.0.0.1:9091", Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}

	const workers = 32
	clients := make(chan any, workers)
	errs := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			client, err := manager.Get(context.Background(), record.ID)
			if err != nil {
				errs <- err
				return
			}
			clients <- client
		}()
	}
	wait.Wait()
	close(clients)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	var first any
	for client := range clients {
		if first == nil {
			first = client
			continue
		}
		if first != client {
			t.Fatal("concurrent Get built multiple clients")
		}
	}
}
