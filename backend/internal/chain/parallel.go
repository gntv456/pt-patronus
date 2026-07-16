package chain

import "context"

// acquireSemaphore waits for a shared concurrency slot without ignoring cancellation.
func acquireSemaphore(ctx context.Context, sem chan struct{}) error {
	if sem == nil {
		return nil
	}
	select {
	case sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
