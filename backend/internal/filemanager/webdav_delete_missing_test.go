package filemanager

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestWebDAVDeleteMissingIsReported 验证：DELETE 得到 404 时不能算成功。
// 旧实现 `resp.StatusCode >= 300 && resp.StatusCode != http.StatusNotFound` 豁免 404，
// 于是删一条已被别的标签页删掉的行，界面弹「已删除」而那行还在列表里。
func TestWebDAVDeleteMissingIsReported(t *testing.T) {
	newBackend := func(status int) *WebDAV {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodDelete {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			w.WriteHeader(status)
		}))
		t.Cleanup(srv.Close)
		return NewWebDAV(srv.URL, "", "", "")
	}

	err := newBackend(http.StatusNotFound).Delete(context.Background(), "/gone.mkv")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("404 应判为 fs.ErrNotExist（handler 靠它翻译成中文），got=%v", err)
	}

	// 对照：真删掉了（204）仍须算成功，别把这条链改成一律失败。
	if err := newBackend(http.StatusNoContent).Delete(context.Background(), "/real.mkv"); err != nil {
		t.Errorf("204 应成功, got %v", err)
	}
	// 对照：其他 3xx/4xx 仍是失败。
	if err := newBackend(http.StatusForbidden).Delete(context.Background(), "/locked.mkv"); err == nil {
		t.Error("403 应返回错误")
	}
}
