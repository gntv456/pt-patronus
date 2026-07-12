package plexsuite

import (
	"context"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// newTestClient 构造允许内网（httptest 用 127.0.0.1）的 httpclient.Client。
func newTestClient(t *testing.T) *httpclient.Client {
	t.Helper()
	c, err := httpclient.New(httpclient.Options{Timeout: 5 * time.Second, AllowPrivate: true})
	if err != nil {
		t.Fatalf("httpclient: %v", err)
	}
	return c
}

// testContext 返回带超时的 ctx。
func testContext() context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = cancel // 用例短，泄漏无所谓
	return ctx
}
