package cloud

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// signFn 测试用签名函数：与 CloudChain.SignDirectLink 同构（<exp>.<hmac>）。
func signFn(key []byte) func(uint, string) string {
	return func(driveID uint, cloudPath string) string {
		exp := time.Now().Add(time.Hour).Unix()
		mac := hmac.New(sha256.New, key)
		fmt.Fprintf(mac, "%d|%s|%d", driveID, cloudPath, exp)
		return fmt.Sprintf("%d.%s", exp, hex.EncodeToString(mac.Sum(nil)))
	}
}

// verifySign 端点侧校验的等价实现（解析 URL → 取 path/query → HMAC 比对）。
func verifySign(key []byte, rawURL string) bool {
	i := strings.Index(rawURL, "?sign=")
	if i < 0 {
		return false
	}
	sign := rawURL[i+len("?sign="):]
	const marker = "/cloud/direct/"
	j := strings.Index(rawURL, marker)
	rest := rawURL[j+len(marker):]
	if q := strings.Index(rest, "?"); q >= 0 {
		rest = rest[:q] // 剥 query（sign），路径段在 ? 之前
	}
	slash := strings.Index(rest, "/")
	drivePart := rest[:slash]
	cloudPath := rest[slash:]
	var driveID uint
	fmt.Sscanf(drivePart, "%d", &driveID)
	dot := strings.Index(sign, ".")
	var exp int64
	fmt.Sscanf(sign[:dot], "%d", &exp)
	if time.Now().Unix() > exp {
		return false
	}
	mac := hmac.New(sha256.New, key)
	fmt.Fprintf(mac, "%d|%s|%d", driveID, cloudPath, exp)
	want := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(want), []byte(sign[dot+1:]))
}

// TestSignPlacementE2E 二轮 P0-1 回归：签名必须落在渲染完成后的行尾，且
// 用「端点等价校验」验证 URL 可通过。覆盖三条写入路径 + 规则生成路径。
func TestSignPlacementE2E(t *testing.T) {
	key := []byte("e2e-sign-key")
	fn := signFn(key)

	t.Run("WriteStrmFile 默认模板 {direct}{cloudpath}", func(t *testing.T) {
		dir := t.TempDir()
		w := WriteStrmOptions{
			OutputDir: dir, CloudPath: "/媒体/电影/a.mkv", Template: "{direct}{cloudpath}",
			FileName: "a.mkv", DriveID: 9, DirectBase: "/api/v1/cloud/direct", SignFunc: fn,
		}
		if _, err := WriteStrmFile(w); err != nil {
			t.Fatal(err)
		}
		line := readSingleStrm(t, dir)
		if !strings.HasSuffix(line, "/媒体/电影/a.mkv?sign=") && !strings.Contains(line, "/媒体/电影/a.mkv?sign=") {
			t.Fatalf("sign 不在路径之后（P0-1 复发）。内容=%q", line)
		}
		if !verifySign(key, line) {
			t.Fatalf("端点等价校验失败。内容=%q", line)
		}
	})

	t.Run("renderStrmContent 规则路径 {direct}{cloudpath}", func(t *testing.T) {
		opts := GenerateOptions{
			DriveID: 3, CloudPath: "/media", Template: "{direct}{cloudpath}",
			LocalBaseURL: "http://nas:8080", SignFunc: fn,
		}
		f := File{Path: "/media/movie/b.mkv", Name: "b.mkv"}
		line := renderStrmContent(opts, f, "/movie/b.mkv")
		if !strings.HasSuffix(line, "/media/movie/b.mkv?sign=") && !strings.Contains(line, "/media/movie/b.mkv?sign=") {
			t.Fatalf("规则路径 sign 位置错。内容=%q", line)
		}
		if !verifySign(key, line) {
			t.Fatalf("规则路径端点校验失败。内容=%q", line)
		}
	})

	t.Run("兼容模式 nil SignFunc 无 sign", func(t *testing.T) {
		opts := GenerateOptions{DriveID: 3, CloudPath: "/media", Template: "{direct}{cloudpath}"}
		f := File{Path: "/media/movie/b.mkv", Name: "b.mkv"}
		line := renderStrmContent(opts, f, "/movie/b.mkv")
		if strings.Contains(line, "sign=") {
			t.Fatalf("兼容模式不应有 sign。内容=%q", line)
		}
	})
}

func readSingleStrm(t *testing.T, dir string) string {
	t.Helper()
	var out string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, werr error) error {
		if werr != nil || info.IsDir() || !strings.HasSuffix(path, ".strm") {
			return werr
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		out = strings.TrimSpace(string(b))
		return nil
	})
	if err != nil || out == "" {
		t.Fatalf("no strm: %v", err)
	}
	return out
}
