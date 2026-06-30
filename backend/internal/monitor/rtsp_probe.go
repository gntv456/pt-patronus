package monitor

import (
	"context"
	"fmt"
	"strings"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
)

// ProbeRTSP 用 gortsplib DESCRIBE 探活 RTSP 地址（成功即在线可达）。
func ProbeRTSP(ctx context.Context, rawURL string) (bool, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return false, fmt.Errorf("empty rtsp url")
	}
	if !strings.HasPrefix(strings.ToLower(rawURL), "rtsp") {
		return false, fmt.Errorf("not an rtsp url")
	}
	u, err := base.ParseURL(rawURL)
	if err != nil {
		return false, err
	}
	done := make(chan probeResult, 1)
	go func() {
		c := gortsplib.Client{Scheme: u.Scheme, Host: u.Host}
		defer c.Close()
		if err := c.Start(); err != nil {
			done <- probeResult{false, err}
			return
		}
		if _, _, err := c.Describe(u); err != nil {
			done <- probeResult{false, err}
			return
		}
		done <- probeResult{true, nil}
	}()
	select {
	case r := <-done:
		return r.ok, r.err
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

type probeResult struct {
	ok  bool
	err error
}
