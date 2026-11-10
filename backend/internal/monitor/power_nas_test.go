// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package monitor

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"golang.org/x/crypto/ssh"
)

// NAS 的电源动作与 SSH 同族：设备真的开始重启时，先把正在等回执的 HTTP 连接掐断。
// 旧实现把这类传输错误原样交回 → 界面「电源操作失败」，而 NAS 其实已经在重启；
// 反过来，鉴权失败/压根连不上必须照实报错，否则真失败也被抹成已生效。

func nasPowerClient(t *testing.T) *httpclient.Client {
	t.Helper()
	hc, err := httpclient.New(httpclient.Options{AllowPrivate: true, Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return hc
}

// dropOnPower 造一个假 NAS：登录等常规请求正常应答，命中 dropMarker 的请求直接掐断连接
// （不给任何响应），模拟「指令已收到、设备随即重启」。
func dropOnPower(t *testing.T, dropMarker string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.String(), dropMarker) {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatalf("httptest server 不支持 hijack，无法模拟设备掉线")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatalf("hijack: %v", err)
			}
			_ = conn.Close()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("api") == "SYNO.API.Auth" {
			fmt.Fprint(w, `{"success":true,"data":{"sid":"SID-TEST"}}`)
			return
		}
		fmt.Fprint(w, `{"success":true,"data":{}}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func nasTarget(vendor, host string) *models.MonitorTarget {
	return &models.MonitorTarget{Name: "nas-" + vendor, Type: "nas", Vendor: vendor, Host: host, TimeoutSec: 3}
}

// TestNASPowerTrueNASSessionDropIsSuccess TrueNAS 重启接口把连接掐断 = 已生效。
func TestNASPowerTrueNASSessionDropIsSuccess(t *testing.T) {
	srv := dropOnPower(t, "/api/v2.0/system/reboot")
	c := NewNASCollector(nasPowerClient(t))
	err := c.Power(context.Background(), nasTarget("truenas", srv.URL), Credentials{Password: "tok"}, PowerActionReboot)
	if err != nil {
		t.Fatalf("已生效的 TrueNAS 重启被报成失败：%v", err)
	}
}

// TestNASPowerSynologySessionDropIsSuccess 群晖同理：Login 拿 sid 成功，reboot 请求发出后掉线。
func TestNASPowerSynologySessionDropIsSuccess(t *testing.T) {
	srv := dropOnPower(t, "method=reboot")
	c := NewNASCollector(nasPowerClient(t))
	err := c.Power(context.Background(), nasTarget("", srv.URL), Credentials{Username: "admin", Password: "pw"}, PowerActionReboot)
	if err != nil {
		t.Fatalf("已生效的群晖重启被报成失败：%v", err)
	}
}

// TestNASPowerAuthFailureStillReports 反面对照：TrueNAS 回 401（Token 无效）不是掉线，
// 必须照实报错——否则一律判「已生效」会把鉴权配置错误洗成成功。
func TestNASPowerAuthFailureStillReports(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"detail":"token invalid"}`)
	}))
	t.Cleanup(srv.Close)
	c := NewNASCollector(nasPowerClient(t))
	err := c.Power(context.Background(), nasTarget("truenas", srv.URL), Credentials{Password: "bad"}, PowerActionShutdown)
	if err == nil {
		t.Fatal("401 必须报错，got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("报错要带上状态码证据，got %v", err)
	}
}

// TestNASPowerUnreachableStillReports 连不上（端口已关）= 请求根本没发出去，仍算失败。
// 这条判据把「设备在重启」与「地址/防火墙不对」分开：后者绝不能被洗成已生效。
func TestNASPowerUnreachableStillReports(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := dead.URL
	dead.Close()
	c := NewNASCollector(nasPowerClient(t))
	if err := c.Power(context.Background(), nasTarget("truenas", url), Credentials{Password: "tok"}, PowerActionReboot); err == nil {
		t.Fatal("连不上必须报错，got nil")
	}
}

// TestPowerDispatchConfirmedTaxonomy 判据表的逐出口核对（一张表钉住「哪些算已生效、哪些不算」）。
func TestPowerDispatchConfirmedTaxonomy(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"ssh 没交出退出状态", &ssh.ExitMissingError{}, true},
		{"ssh 非零退出", &ssh.ExitError{}, false},
		{"eof", io.EOF, true},
		{"连接被重置", &net.OpError{Op: "read", Err: syscall.ECONNRESET}, true},
		{"连接被拒绝（没发出去）", &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}, false},
		{"本地超时", context.DeadlineExceeded, false},
		{"包装过的 eof", fmt.Errorf("synology reboot: %w（v2 重试亦失败：%v）", io.EOF, io.EOF), true},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := powerDispatchConfirmed(tc.err); got != tc.want {
				t.Errorf("powerDispatchConfirmed(%v)=%v want %v", tc.err, got, tc.want)
			}
		})
	}
}
