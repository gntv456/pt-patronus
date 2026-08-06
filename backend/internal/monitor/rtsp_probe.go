package monitor

import (
	"context"
	"fmt"
	"strings"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/hxpt/ptpatronus/internal/core/safego"
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
	stop := make(chan struct{})
	safego.Go("rtsp_probe:27", func() {
		c := gortsplib.Client{Scheme: u.Scheme, Host: u.Host}
		defer c.Close()
		// 上层 ctx 取消时强制关 client，让阻塞中的 Start/Describe 返回错误，
		// 否则 goroutine 持续阻塞在 RTSP 握手 → 每次超时探活泄漏一个。
		safego.Go("rtsp_probe:32", func() {
			select {
			case <-ctx.Done():
				c.Close()
			case <-stop:
			}
		})
		if err := c.Start(); err != nil {
			done <- probeResult{false, err}
			return
		}
		if _, _, err := c.Describe(u); err != nil {
			done <- probeResult{false, err}
			return
		}
		done <- probeResult{true, nil}
	})
	select {
	case r := <-done:
		close(stop)
		return r.ok, r.err
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

type probeResult struct {
	ok  bool
	err error
}
