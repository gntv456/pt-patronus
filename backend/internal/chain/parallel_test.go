package chain

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAcquireSemaphoreHonorsCancellation(t *testing.T) {
	sem := make(chan struct{}, 1)
	sem <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	started := time.Now()
	err := acquireSemaphore(ctx, sem)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, want context canceled", err)
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("canceled acquire blocked for %s", elapsed)
	}
}
