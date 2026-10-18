package web

// HTTP 层：把 DataDir/jessibuca 下的播放器资源直出，缺失时自动下载。
//
// 挂在 NoRoute 之前的专用路由上，而不是走 SPA fallback——后者对任何未命中路径
// 都返回 index.html，浏览器拿到 200 + HTML 却以为是 wasm，会报难以定位的解码错误。
//
// 下载是同步的（最长 5 分钟）：首次预览时用户等一次，之后读本地文件。
// 之所以不改成异步 + 轮询，是因为 1.4MB 在正常网络下就一两秒，同步足够简单；
// 且浏览器对 wasm 的加载本就是一次阻塞 fetch。

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

// contentType 按扩展名给静态资源定 MIME。
// wasm 必须发 application/wasm：浏览器才会走 WebAssembly.instantiateStreaming，
// 发成 application/octet-stream 会退回 ArrayBuffer 路径（能跑但慢且多一次拷贝）。
func contentType(name string) string {
	switch {
	case strings.HasSuffix(name, ".wasm"):
		return "application/wasm"
	case strings.HasSuffix(name, ".js"):
		return "text/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".html"):
		return "text/html; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

// AssetHandler 返回播放器资源的处理器。
func AssetHandler(store *AssetStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 只接受 GET/HEAD：这是纯静态资源，其余方法没有意义。
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		name := SafeName(r.URL.Path)
		f, err := store.Open(name)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				// 清单外的名字一律 404，绝不回落成 index.html。
				http.NotFound(w, r)
				return
			}
			slog.Warn("jessibuca 资源获取失败", "file", name, "err", err)
			// 503 + Retry-After：前端据此提示「稍后重试」，区别于 404 的「没部署」。
			w.Header().Set("Retry-After", "30")
			http.Error(w, "player assets unavailable: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		defer f.Close()

		fi, err := f.Stat()
		if err != nil {
			http.Error(w, "stat failed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", contentType(name))
		w.Header().Set("Content-Length", strconv.FormatInt(fi.Size(), 10))
		// 这些是版本锁定的不可变产物（URL 不含内容哈希，但版本固定且校验和保证
		// 不变），可长缓存；否则用户每次升级都要重下 1.4MB。
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		// 跨源隔离：wasm 多线程解码需要 SharedArrayBuffer。
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")

		if r.Method == http.MethodHead {
			return
		}
		http.ServeContent(w, r, name, fi.ModTime(), f)
	})
}

// AssetsReady 供健康检查/设置页查询是否已就位。
func AssetsReady(store *AssetStore) bool { return store.Ready() }
