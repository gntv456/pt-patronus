// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package ocr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParsePaddle(t *testing.T) {
	got, err := parse([]byte(`{"results":[[{"text":"ab12","confidence":0.98}]]}`), "paddle")
	if err != nil || got != "ab12" {
		t.Fatalf("paddle parse: %q %v", got, err)
	}
}

func TestParsePaddleAltArray(t *testing.T) {
	got, err := parse([]byte(`[{"text":"xy9z"}]`), "paddle")
	if err != nil || got != "xy9z" {
		t.Fatalf("alt parse: %q %v", got, err)
	}
}

func TestParseSimpleAndDdddoc(t *testing.T) {
	if got, _ := parse([]byte(`{"text":"q1w2"}`), "simple"); got != "q1w2" {
		t.Fatalf("simple: %q", got)
	}
	if got, _ := parse([]byte(`{"data":{"data":"zz88"}}`), "ddddoc"); got != "zz88" {
		t.Fatalf("ddddoc: %q", got)
	}
}

func TestStripDataURL(t *testing.T) {
	if stripDataURL("data:image/png;base64,QUJD") != "QUJD" {
		t.Fatal("data url not stripped")
	}
	if stripDataURL("QUJD") != "QUJD" {
		t.Fatal("plain b64 must pass through")
	}
}

func TestRecognizeDisabled(t *testing.T) {
	c := New(Config{})
	if c.Enabled() {
		t.Fatal("empty endpoint must be disabled")
	}
	if _, err := c.Recognize(context.Background(), "x"); err == nil {
		t.Fatal("disabled client must error")
	}
}

func TestRecognizeHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.WriteHeader(401)
			return
		}
		_, _ = w.Write([]byte(`{"results":[[{"text":"ok77"}]]}`))
	}))
	defer srv.Close()
	c := New(Config{Endpoint: srv.URL, APIKey: "k"})
	got, err := c.Recognize(context.Background(), "data:image/png;base64,QUJD")
	if err != nil || got != "ok77" {
		t.Fatalf("recognize: %q %v", got, err)
	}
	// 无 key → 401 → error（调用方回退人工）
	c2 := New(Config{Endpoint: srv.URL})
	if _, err := c2.Recognize(context.Background(), "QUJD"); err == nil {
		t.Fatal("expect 401 error")
	}
}
