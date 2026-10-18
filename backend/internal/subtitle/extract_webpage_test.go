package subtitle

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func zipWithFiles(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const srtBody = "1\n00:00:01,000 --> 00:00:03,000\n你好，世界\n"

// TestExtractRejectsWebPage 字幕站返回 HTTP 200 + 登录墙/积分墙网页时，
// 不能当作字幕字节返回：此前会被原样写成 .srt 落进媒体库，
// 播放器没字幕，而健康中心按「字幕已存在」判定，从此不再提示缺失。
func TestExtractRejectsWebPage(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"纯 html", []byte("<!DOCTYPE html>\n<html><head><title>登录</title></head><body>请先登录</body></html>")},
		{"带 BOM 与空白", []byte("\ufeff\n   <html><body>积分不足</body></html>")},
		{"zip 里只有网页", zipWithFiles(t, map[string]string{"index.html": "<html><body>请登陆</body></html>"})},
	}
	for _, c := range cases {
		data, name, err := Extract(c.data, "subhd-123")
		if err == nil {
			t.Errorf("%s: 网页内容应报错，got name=%q len=%d", c.name, name, len(data))
			continue
		}
		if !strings.Contains(err.Error(), "网页") {
			t.Errorf("%s: 错误应说明原因，got %v", c.name, err)
		}
		if data != nil || name != "" {
			t.Errorf("%s: 拒绝时不得返回可写入内容，got name=%q len=%d", c.name, name, len(data))
		}
	}

	// 正控一：真字幕照常返回。
	data, name, err := Extract([]byte(srtBody), "movie.chs.srt")
	if err != nil || string(data) != srtBody || name != "movie.chs.srt" {
		t.Fatalf("正常字幕应原样返回: err=%v name=%q", err, name)
	}

	// 正控二：字幕正文里出现 html 字样（教程片/关于网页的台词）不得被误杀——
	// 判定只看「开头就是 HTML 文档」。
	bodyWithHTML := "1\n00:00:01,000 --> 00:00:03,000\n<!DOCTYPE html> 这个标签怎么写\n"
	if _, _, err := Extract([]byte(bodyWithHTML), "webdev.srt"); err != nil {
		t.Fatalf("正文含 html 字样的字幕被误杀: %v", err)
	}

	// 正控三：zip 里的真字幕照常取出。
	zipOK := zipWithFiles(t, map[string]string{"readme.txt": "说明", "movie.srt": srtBody})
	if b, nm, err := Extract(zipOK, ""); err != nil || nm != "movie.srt" || !strings.Contains(string(b), "你好") {
		t.Fatalf("zip 内字幕应取出: err=%v name=%q", err, nm)
	}
}

// TestLooksLikeWebPage 判定器的边界：只认开头的 HTML 文档标记。
func TestLooksLikeWebPage(t *testing.T) {
	yes := []string{"<html lang=\"zh\">", "<!doctype html>", "<!DOCTYPE HTML>\n<html>"}
	no := []string{srtBody, "[Script Info]\nTitle: x", "WEBVTT\n\n00:00.000 --> 00:01.000\nhi",
		"Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,<html> 标签", ""}
	for _, s := range yes {
		if !looksLikeWebPage([]byte(s)) {
			t.Errorf("应判为网页: %q", s)
		}
	}
	for _, s := range no {
		if looksLikeWebPage([]byte(s)) {
			t.Errorf("不应判为网页: %q", s)
		}
	}
}
