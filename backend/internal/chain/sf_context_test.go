package chain

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/sync/singleflight"
)

func TestSFDoContextHonorsCancellation(t *testing.T) {
	var group singleflight.Group
	release := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	started := time.Now()
	_, err := sfDoContext(ctx, &group, "key", func() (string, error) {
		<-release
		return "value", nil
	})
	close(release)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, want context canceled", err)
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("canceled singleflight wait blocked for %s", elapsed)
	}
}
