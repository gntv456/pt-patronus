package subtitle

import (
	"archive/zip"
	"bytes"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func TestNormalizeLang(t *testing.T) {
	cases := map[string]string{
		"简体中文":            "chs",
		"繁體中文":            "cht",
		"繁体中文":            "cht",
		"English":         "eng",
		"中英双语":            "bi",
		"chinese(简)":      "chs",
		"zh-TW":           "cht",
		"":                "und",
		"unknown lang":    "und",
	}
	for in, want := range cases {
		if got := normalizeLang(in); got != want {
			t.Errorf("normalizeLang(%q)=%q want %q", in, got, want)
		}
	}
}

func TestIsSubtitleExt(t *testing.T) {
	for _, good := range []string{"a.srt", "b.ASS", "c.vtt", "d.ssa", "e.sub"} {
		if !IsSubtitleExt(good) {
			t.Errorf("IsSubtitleExt(%q)=false want true", good)
		}
	}
	for _, bad := range []string{"a.mp4", "b.zip", "c", "d.txt"} {
		if IsSubtitleExt(bad) {
			t.Errorf("IsSubtitleExt(%q)=true want false", bad)
		}
	}
}

func TestExtractPassthrough(t *testing.T) {
	data := []byte("1\n00:00:01 --> 00:00:02\nHello\n")
	out, name, err := Extract(data, "movie.srt")
	if err != nil {
		t.Fatal(err)
	}
	if name != "movie.srt" || !bytes.Equal(out, data) {
		t.Errorf("passthrough wrong: name=%q", name)
	}
}

func TestExtractZip(t *testing.T) {
	// 构造含一个 .srt 的 zip
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	fw, _ := w.Create("subs/inner.srt")
	fw.Write([]byte("1\n00:00:01 --> 00:00:02\n你好\n"))
	fw2, _ := w.Create("readme.txt") // 非字幕，应被忽略
	fw2.Write([]byte("ignore me"))
	w.Close()

	out, name, err := Extract(buf.Bytes(), "fallback.zip")
	if err != nil {
		t.Fatal(err)
	}
	if name != "inner.srt" {
		t.Errorf("zip extract name=%q want inner.srt", name)
	}
	if !bytes.Contains(out, []byte("你好")) {
		t.Errorf("zip extract content wrong: %q", out)
	}
}

func TestDecodeUTF8(t *testing.T) {
	// 把 UTF-8 字符串编码为 GB18030，再解码应还原
	orig := "简体中文测试"
	gbk, err := simplifiedchinese.GB18030.NewEncoder().Bytes([]byte(orig))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(gbk, []byte(orig)) {
		t.Skip("编码环境异常，跳过")
	}
	decoded := DecodeUTF8(gbk)
	if string(decoded) != orig {
		t.Errorf("DecodeUTF8(GBK)=%q want %q", decoded, orig)
	}
	// 已是 UTF-8 原样
	if string(DecodeUTF8([]byte(orig))) != orig {
		t.Error("UTF-8 passthrough broken")
	}
}

func TestConvertT2S(t *testing.T) {
	got, err := Convert("繁體中文測試", "chs")
	if err != nil {
		t.Fatal(err)
	}
	if got != "繁体中文测试" {
		t.Errorf("t2s: %q want 繁体中文测试", got)
	}
}

func TestConvertS2T(t *testing.T) {
	got, err := Convert("简体中文测试", "cht")
	if err != nil {
		t.Fatal(err)
	}
	if got != "簡體中文測試" {
		t.Errorf("s2t: %q want 簡體中文測試", got)
	}
}

func TestConvertNoop(t *testing.T) {
	// target="" 不转换
	got, _ := Convert("繁體", "")
	if got != "繁體" {
		t.Errorf("noop convert altered input: %q", got)
	}
}
