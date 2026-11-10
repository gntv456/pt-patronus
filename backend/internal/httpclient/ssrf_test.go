// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package httpclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"
)

func TestIsPublicIP(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"8.8.8.8", true},
		{"1.1.1.1", true},
		{"127.0.0.1", false},
		{"10.0.0.1", false},
		{"192.168.1.1", false},
		{"172.16.0.1", false},
		{"172.31.255.255", false},
		{"172.32.0.1", true}, // 刚好超出 172.16/12
		{"169.254.169.254", false},
		{"100.64.0.1", false}, // CGNAT
		{"100.127.255.255", false},
		{"100.128.0.1", true},
		{"0.0.0.0", false},
		{"::1", false},
		{"fc00::1", false},
		{"fe80::1", false},
		{"2606:4700:4700::1111", true},
	}
	for _, c := range cases {
		ip := net.ParseIP(c.ip)
		if ip == nil {
			t.Fatalf("parse %s", c.ip)
		}
		if got := IsPublicIP(ip); got != c.want {
			t.Errorf("IsPublicIP(%s) = %v, want %v", c.ip, got, c.want)
		}
	}
}

// 非法 scheme 是确定的拒绝（不可重试），包裹 ErrBlocked。
func TestAssertPublicURLScheme(t *testing.T) {
	for _, raw := range []string{"file:///etc/passwd", "ftp://example.com"} {
		if err := AssertPublicURL(raw); err == nil {
			t.Errorf("%q: scheme should be blocked", raw)
		} else if !errors.Is(err, ErrBlocked) {
			t.Errorf("%q: scheme rejection must be ErrBlocked, got %v", raw, err)
		}
	}
}

// 字面私有 IP 直接命中（不触发 DNS，无网络依赖）。
func TestAssertPublicURLLiteralPrivateBlocked(t *testing.T) {
	for _, raw := range []string{
		"http://127.0.0.1:8080/x",
		"http://10.1.2.3/",
		"http://169.254.169.254/latest/meta-data/", // 云元数据
		"http://[::1]/",
	} {
		if err := AssertPublicURL(raw); !errors.Is(err, ErrBlocked) {
			t.Errorf("%q: expected ErrBlocked, got %v", raw, err)
		}
	}
}

// 字面公网 IP 应放行（不触发 DNS，无网络依赖）。
func TestAssertPublicURLLiteralPublicOK(t *testing.T) {
	if err := AssertPublicURL("http://8.8.8.8/"); err != nil {
		t.Fatalf("literal public IP should pass, got %v", err)
	}
}

// 空 host 是确定的拒绝（不可重试）。
func TestAssertPublicURLEmptyHostBlocked(t *testing.T) {
	if err := AssertPublicURL("http:///path"); !errors.Is(err, ErrBlocked) {
		t.Fatalf("empty host must be ErrBlocked, got %v", err)
	}
}

// 注入 lookup 验证分类：解析到私有 IP → ErrBlocked；解析到公网 IP → nil；
// 任一解析结果为私有 → ErrBlocked；解析失败 → 非 ErrBlocked（可重试）。
func TestAssertPublicURLKernelClassification(t *testing.T) {
	cases := []struct {
		name    string
		lookup  lookupIPFunc
		blocked bool // 是否应判定为 ErrBlocked
		wantErr bool // 是否应有错（nil 除外）
	}{
		{
			name: "resolved private ip",
			lookup: func(ctx context.Context, host string) ([]net.IP, error) {
				return []net.IP{net.ParseIP("10.1.2.3")}, nil
			},
			blocked: true,
			wantErr: true,
		},
		{
			name:   "resolved public ip",
			lookup: func(ctx context.Context, host string) ([]net.IP, error) { return []net.IP{net.ParseIP("8.8.8.8")}, nil },
		},
		{
			name: "resolved mix incl private",
			lookup: func(ctx context.Context, host string) ([]net.IP, error) {
				return []net.IP{net.ParseIP("8.8.8.8"), net.ParseIP("169.254.169.254")}, nil
			},
			blocked: true,
			wantErr: true,
		},
		{
			name:    "dns timeout transient",
			lookup:  func(ctx context.Context, host string) ([]net.IP, error) { return nil, errors.New("i/o timeout") },
			wantErr: true, // blocked=false → 可重试
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := assertPublicURL("http://example.com/", tc.lookup)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected nil, got %v", err)
			}
			if tc.blocked && !errors.Is(err, ErrBlocked) {
				t.Fatalf("expected ErrBlocked, got %v", err)
			}
			if !tc.blocked && tc.wantErr && errors.Is(err, ErrBlocked) {
				t.Fatalf("transient error must NOT be ErrBlocked, got %v", err)
			}
		})
	}
}

// ErrBlocked 必须立即返回——不退避、不重试（也不给攻击者重试探针的机会）。
func TestRetryAssertNoRetryOnBlocked(t *testing.T) {
	c, _ := New(Options{})
	var calls int
	start := time.Now()
	err := c.retryAssert(func() error {
		calls++
		return fmt.Errorf("%w: 127.0.0.1", ErrBlocked)
	})
	if calls != 1 {
		t.Fatalf("ErrBlocked must not retry, calls=%d", calls)
	}
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Fatalf("ErrBlocked must return without backoff, took %v", d)
	}
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("expected ErrBlocked, got %v", err)
	}
}

// 瞬时解析错误应重试 maxAttempts 次（1 次首检 + 2 次重试），最终返回非 ErrBlocked 错误。
func TestRetryAssertRetriesTransient(t *testing.T) {
	c, _ := New(Options{})
	var calls int
	err := c.retryAssert(func() error {
		calls++
		return errors.New("lookup www.example.com on 127.0.0.11:53: i/o timeout")
	})
	if calls != 3 {
		t.Fatalf("expected 3 attempts (1+2 retries), got %d", calls)
	}
	if err == nil {
		t.Fatal("expected final error after retries")
	}
	if errors.Is(err, ErrBlocked) {
		t.Fatalf("transient error must not be classified ErrBlocked, got %v", err)
	}
}

// 重试期间恢复（第 2 次成功）应正常返回 nil。
func TestRetryAssertSucceedsOnRetry(t *testing.T) {
	c, _ := New(Options{})
	var calls int
	err := c.retryAssert(func() error {
		calls++
		if calls < 2 {
			return errors.New("dns timeout")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected success on retry, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}
