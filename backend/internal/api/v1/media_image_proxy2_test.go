// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// 海报代理的两个出口：超限必须拒（而不是 200 + 半张图），正常尺寸必须照抄字节。
// 上游起在 127.0.0.1（httptest），因此测试期临时放行私网 + 把该 host:port 加进白名单，
// 两项都在用例结束时还原（不污染同包其它用例的 SSRF 判据）。
// 注：白名单按 host 精确/后缀匹配，带端口的 host 命不中 ⇒ 这里登记的键是 host:port 本体。
func withLocalImageUpstream(t *testing.T, fn func(base string)) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/big" {
			// 声明 6MB 但只写几字节：判据只看 Content-Length，不读 body
			w.Header().Set("Content-Type", "image/jpeg")
			w.Header().Set("Content-Length", strconv.Itoa(6<<20))
			_, _ = w.Write([]byte("prefix"))
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("PNGDATA-1234"))
	}))
	host := strings.TrimPrefix(srv.URL, "http://")
	mediaImageHosts[host] = ""
	prevClient := mediaClient
	mediaClient = newMediaClient(true, true)
	defer func() {
		delete(mediaImageHosts, host)
		mediaClient = prevClient
		srv.Close()
	}()
	fn(srv.URL)
}

func callImageProxy(t *testing.T, rawURL string) (int, string, []byte) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/media/image?url="+rawURL, nil)
	(&MediaHandler{}).Image(c)
	return rec.Code, rec.Header().Get("Content-Type"), rec.Body.Bytes()
}

func TestImageProxyRejectsOversizedUpstream(t *testing.T) {
	withLocalImageUpstream(t, func(base string) {
		code, ct, body := callImageProxy(t, base+"/big")
		if code != http.StatusBadGateway {
			t.Fatalf("超限图片状态码 = %d，期望 502（body=%q）", code, body)
		}
		var env map[string]string
		if err := json.Unmarshal(body, &env); err != nil {
			t.Fatalf("超限响应不是错误信封: %q", string(body))
		}
		if env["code"] != "media_image_too_large" {
			t.Fatalf("错误码 = %v，期望 media_image_too_large", env["code"])
		}
		if env["message"] == "" {
			t.Error("用户侧没有可读文案")
		}
		// 关键：不得把半张图当 image/* 发给客户端（那正是「图裂了」的成因）
		if strings.HasPrefix(ct, "image/") {
			t.Errorf("超限响应带了图片类型 %q，说明半张图仍然被发出去了", ct)
		}
	})
}

// 正控：正常尺寸照旧流式转发，字节一个不少（判据不能把小图也拒了）。
func TestImageProxyForwardsNormalSize(t *testing.T) {
	withLocalImageUpstream(t, func(base string) {
		code, ct, body := callImageProxy(t, base+"/small")
		if code != http.StatusOK {
			t.Fatalf("小图状态码 = %d body=%q", code, body)
		}
		if ct != "image/png" {
			t.Errorf("Content-Type = %q，期望原样透传 image/png", ct)
		}
		if string(body) != "PNGDATA-1234" {
			t.Errorf("转发的 body = %q，期望 PNGDATA-1234", string(body))
		}
	})
}
