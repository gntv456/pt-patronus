// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

func newSubscribeUpdateTestHandler(t *testing.T) (*SubscribeHandler, *oper.SubscriptionOper, *models.Subscription) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "subscriptions.db")+"?_pragma=busy_timeout(5000)"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.Subscription{}, &models.SubscriptionHit{}); err != nil {
		t.Fatal(err)
	}
	op := oper.NewSubscriptionOper(db)
	s := &models.Subscription{
		Source: "tmdb", SourceID: "movie:1", Title: "Movie", Status: "subscribed",
		Resolution: "2160p", DownloaderID: 9, SavePath: "/old/path",
	}
	if err := op.Create(s); err != nil {
		t.Fatal(err)
	}
	ch := chain.NewSubscribeChain(nil, nil, nil, op, nil, nil, nil, nil)
	return NewSubscribeHandler(ch), op, s
}

func TestSubscriptionUpdateClearsOptionalValues(t *testing.T) {
	h, op, s := newSubscribeUpdateTestHandler(t)
	r := gin.New()
	r.PUT("/:id", h.Update)
	rec := doReq(t, r, http.MethodPut, "/1", `{"resolution":"","downloader_id":0,"save_path":""}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	got, err := op.Get(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Resolution != "" || got.DownloaderID != 0 || got.SavePath != "" {
		t.Fatalf("optional values not cleared: resolution=%q downloader_id=%d save_path=%q", got.Resolution, got.DownloaderID, got.SavePath)
	}
}

func TestSubscriptionUpdateKeepsOmittedValues(t *testing.T) {
	h, op, s := newSubscribeUpdateTestHandler(t)
	r := gin.New()
	r.PUT("/:id", h.Update)
	rec := doReq(t, r, http.MethodPut, "/1", `{"status":"paused"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	got, err := op.Get(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Resolution != "2160p" || got.DownloaderID != 9 || got.SavePath != "/old/path" {
		t.Fatalf("omitted values changed: resolution=%q downloader_id=%d save_path=%q", got.Resolution, got.DownloaderID, got.SavePath)
	}
	if got.Status != "paused" {
		t.Fatalf("status = %q, want paused", got.Status)
	}
}
