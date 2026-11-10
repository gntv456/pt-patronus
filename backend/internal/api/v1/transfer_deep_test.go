// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"context"
	"testing"
)

type transferSweepContextKey struct{}

func TestTransferSweepContextSurvivesRequestCancellation(t *testing.T) {
	requestCtx, cancel := context.WithCancel(context.WithValue(context.Background(), transferSweepContextKey{}, "request-value"))
	sweepCtx := transferSweepContext(requestCtx)
	cancel()

	if err := requestCtx.Err(); err == nil {
		t.Fatal("request context should be canceled")
	}
	if err := sweepCtx.Err(); err != nil {
		t.Fatalf("sweep context inherited request cancellation: %v", err)
	}
	if got := sweepCtx.Value(transferSweepContextKey{}); got != "request-value" {
		t.Fatalf("request values not preserved, got %v", got)
	}
}
