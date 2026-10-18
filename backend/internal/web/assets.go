package web

// 摄像头预览资源（jessibuca）的按需获取与分发。
//
// 背景：jessibuca 是 GPL-3.0，把 decoder.wasm 打进镜像等于对外发布 GPL 产物。
// 故镜像与仓库都不含该二进制（见 .dockerignore / .gitignore），改为运行时获取。
//
// 但「让用户自己跑 bash 脚本」不成立——docker compose 用户往往不知道项目根目录在哪，
// 更不会进容器执行命令。因此取而代之：由后端负责下载。
//
// 存放位置：DataDir/jessibuca/。选它而不是容器内固定路径，有两个原因：
//  1. DataDir 是用户唯一会挂载持久化的目录（compose 默认 ./data:/data），
//     放这里重启/升级镜像都不必重下；
//  2. 容器可能是只读 rootfs，写到 /app 会直接失败。
//
// 安全：只允许清单内的固定文件名（防路径穿越），下载后逐个校验 sha256。
// 落盘文件名固定、内容由校验和保证，故不存在「用户可控内容被当脚本执行」。

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// AssetVersion 锁定的上游版本（tag）。tag 不可变，产物随之确定。
const AssetVersion = "v3.3.28"

// assetBaseURL 指向上游产物目录。做成包级变量而非 const，便于测试用 httptest 替换
// （不打真实外网：CI 无外网，且测试不该依赖 GitHub 可用性）。
var assetBaseURL = "https://raw.githubusercontent.com/langhuihui/jessibuca/" + AssetVersion + "/dist"

// Asset 是一个必须下载的播放器资源。
type Asset struct {
	File   string
	SHA256 string
	Bytes  int64
}

// Assets 是完整的必需清单。少任何一个，摄像头预览都起不来
// （jessibuca.js 播放器、decoder.js wasm 胶水、decoder.wasm ffmpeg 解码器）。
//
// wasm 内含 HEVC decoder，故 H.265（小米的默认输出）可解；播放器自带
// {7:"H264(AVC)", 12:"H265(HEVC)"} 编码表，从 annexB NAL 头自动判别。
var Assets = []Asset{ //nolint:gochecknoglobals // 测试需替换为小清单
	{"jessibuca.js", "be5880c00e253c1085c7332ca72a639ec526c07f2799a3d9f6dd30e9e4b6c9e9", 219743},
	{"decoder.js", "0f9c325d8a0a5c07d893d64007b3a4b9b29eb6fac752e7591fbe135b3051ed8c", 110237},
	{"decoder.wasm", "2f8638a5db551b5ab7df828522713f673e0104e2fe1be70b3d93857e487d7115", 1050314},
}

// ErrNotFound 表示请求的文件不在清单内（拒绝服务任意路径）。
var ErrNotFound = errors.New("asset not in manifest")

// AssetStore 管理 DataDir/jessibuca 下的播放器资源：按需下载 + 校验 + 落盘 + 提供读取。
type AssetStore struct {
	dir    string
	client *http.Client

	mu       sync.Mutex
	inflight map[string]*sync.WaitGroup // 同文件并发下载时只跑一次
}

const downloadTimeout = 5 * time.Minute

// NewAssetStore 构造。dataDir 为空时退化为相对路径（本地开发场景）。
func NewAssetStore(dataDir string) *AssetStore {
	if dataDir == "" {
		dataDir = "./data"
	}
	return &AssetStore{
		dir:      filepath.Join(dataDir, "jessibuca"),
		client:   &http.Client{Timeout: downloadTimeout},
		inflight: map[string]*sync.WaitGroup{},
	}
}

func assetByName(name string) (Asset, bool) {
	for _, a := range Assets {
		if a.File == name {
			return a, true
		}
	}
	return Asset{}, false
}

// pathFor 返回文件落盘路径，并拒绝清单外的名字。
func (s *AssetStore) pathFor(name string) (string, Asset, error) {
	a, ok := assetByName(name)
	if !ok {
		return "", Asset{}, ErrNotFound
	}
	// name 已在白名单内，filepath.Join 不会越出 dir（无分隔符、无 ..）。
	return filepath.Join(s.dir, a.File), a, nil
}

// Ready 报告三个资源是否都已就位且校验通过。
func (s *AssetStore) Ready() bool {
	for _, a := range Assets {
		p, _, err := s.pathFor(a.File)
		if err != nil || !s.validOnDisk(p, a) {
			return false
		}
	}
	return true
}

// Status 返回每个文件的就位状态，供设置页/健康检查展示。
type AssetStatus struct {
	File  string `json:"file"`
	Bytes int64  `json:"bytes"`
	Ready bool   `json:"ready"`
	Error string `json:"error,omitempty"`
}

// Status 逐文件状态。
func (s *AssetStore) Status() []AssetStatus {
	out := make([]AssetStatus, 0, len(Assets))
	for _, a := range Assets {
		p, _, err := s.pathFor(a.File)
		st := AssetStatus{File: a.File, Bytes: a.Bytes}
		if err != nil {
			st.Error = err.Error()
		} else if s.validOnDisk(p, a) {
			st.Ready = true
		}
		out = append(out, st)
	}
	return out
}

// validOnDisk 校验磁盘文件：存在 + 大小一致 + sha256 一致。
// sha256 是每次都算（三个文件共 1.4MB，开销可忽略），而不是只看大小——
// 大小相同但内容被改的情况（截断/拼接）只有摘要能挡。
func (s *AssetStore) validOnDisk(path string, a Asset) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || fi.Size() != a.Bytes {
		return false
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false
	}
	return hex.EncodeToString(h.Sum(nil)) == a.SHA256
}

// Ensure 确保某文件就位；缺失或损坏则重新下载。
func (s *AssetStore) Ensure(name string) error {
	path, a, err := s.pathFor(name)
	if err != nil {
		return err
	}
	if s.validOnDisk(path, a) {
		return nil
	}

	// 同名并发只下载一次：后到的等前一个完成后再各自复验。
	s.mu.Lock()
	if wg, ok := s.inflight[name]; ok {
		s.mu.Unlock()
		wg.Wait()
		if s.validOnDisk(path, a) {
			return nil
		}
		return fmt.Errorf("资源 %s 下载未成功", name)
	}
	wg := &sync.WaitGroup{}
	wg.Add(1)
	s.inflight[name] = wg
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.inflight, name)
		s.mu.Unlock()
		wg.Done()
	}()

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("创建目录失败：%w", err)
	}
	return s.download(a, path)
}

// EnsureAll 确保全部资源就位。
func (s *AssetStore) EnsureAll() error {
	for _, a := range Assets {
		if err := s.Ensure(a.File); err != nil {
			return err
		}
	}
	return nil
}

// download 下载单个文件到 tmp，校验通过才原子改名——避免中途失败留下半个文件
// 被后续请求当成「已就位」而持续花屏。
func (s *AssetStore) download(a Asset, dst string) error {
	url := assetBaseURL + "/" + a.File
	resp, err := s.client.Get(url) //nolint:noctx // 一次性后台下载，无请求上下文可继承
	if err != nil {
		return fmt.Errorf("下载 %s 失败：%w", a.File, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载 %s 失败：HTTP %d", a.File, resp.StatusCode)
	}

	tmp := dst + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("写入失败：%w", err)
	}
	h := sha256.New()
	n, cerr := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, a.Bytes+1))
	f.Close()
	if cerr != nil {
		os.Remove(tmp)
		return fmt.Errorf("写入 %s 失败：%w", a.File, cerr)
	}
	if n != a.Bytes {
		os.Remove(tmp)
		return fmt.Errorf("%s 大小不符：期望 %d 实际 %d", a.File, a.Bytes, n)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != a.SHA256 {
		os.Remove(tmp)
		return fmt.Errorf("%s 校验失败：期望 %s 实际 %s", a.File, a.SHA256[:12], got[:12])
	}
	// 校验通过后才落到最终名（同目录 rename 是原子操作）。
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("落盘失败：%w", err)
	}
	return nil
}

// Open 打开某文件供 HTTP 服务读取；不存在时先尝试自动下载。
func (s *AssetStore) Open(name string) (*os.File, error) {
	path, _, err := s.pathFor(name)
	if err != nil {
		return nil, err
	}
	f, ferr := os.Open(path)
	if ferr == nil {
		return f, nil
	}
	if !errors.Is(ferr, os.ErrNotExist) {
		return nil, ferr
	}
	if derr := s.Ensure(name); derr != nil {
		return nil, derr
	}
	return os.Open(path)
}

// SafeName 清洗出清单内的文件名（去掉目录部分），供 HTTP 层调用。
func SafeName(p string) string {
	p = strings.TrimPrefix(p, "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		p = p[i+1:]
	}
	return p
}
