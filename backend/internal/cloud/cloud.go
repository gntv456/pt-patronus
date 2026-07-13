// Package cloud 封装网盘（115 / 夸克 / 123云盘）原生 HTTP 客户端子集 + STRM 生成器。
//
// 设计：三家网盘协议各异但操作子集一致（列目录 / 配额 / 探活），各实现 Client 接口，
// chain 层按 Drive.Type dispatch，上层无需感知差异。cookie/token 明文由 chain 解密后传入
// Drive.Token；三家网盘均为公网站点，client 调用以 ssrf=true 走公网守卫（同站点抓取范式）。
//
// 列目录按「路径」语义（/电影/动作/xxx.mkv）暴露；内部网盘多按 id（cid/fid）寻址，故
// 各 client 实现 resolvePath：从根 id 出发按路径段名逐级解析到目标 id，再 listByID。
// 字段名/端点基于社区逆向契约，单测用 httptest mock 覆盖解析逻辑；真实联调留待用户。
package cloud

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// Drive 是构建 client 所需的连接信息（oper 解密 Token 后填入）。
type Drive struct {
	ID       uint
	Type     string // 115 | quark | 123
	Token    string // 解密后的 cookie(115/夸克) 或 accessToken(123)
	RootPath string // 浏览/扫描起点；空或 "/"=网盘根
}

// File 是网盘中的一个条目（文件或目录）。Path 为完整云盘路径（含根，以 / 开头）。
type File struct {
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	IsDir    bool      `json:"is_dir"`
	Size     int64     `json:"size"`
	ModTime  time.Time `json:"mod_time"`
	PickCode string    `json:"pick_code,omitempty"` // 115/夸克下载所需；123 无
}

// Quota 网盘容量。
type Quota struct {
	Used  int64 `json:"used"`
	Total int64 `json:"total"`
}

// Client 统一三家网盘的操作子集（STRM 生成仅需 List；Test/Quota 供 UI 展示与探活）。
type Client interface {
	Type() string
	// Test 探活（拉用户/配额信息）。
	Test(ctx context.Context) (info string, err error)
	// List 列出 dir 目录下一层条目（非递归；dir 以 / 起的云盘路径）。
	List(ctx context.Context, dir string) ([]File, error)
	// Quota 拉容量。
	Quota(ctx context.Context) (*Quota, error)
}

// NewClient 按 Drive.Type 构建对应客户端。type 不在白名单内返回错误。
func NewClient(d Drive, hc *httpclient.Client) (Client, error) {
	switch d.Type {
	case "115":
		return &client115{drive: d, hc: hc}, nil
	case "quark":
		return &clientQuark{drive: d, hc: hc}, nil
	case "123":
		return &client123{drive: d, hc: hc}, nil
	}
	return nil, fmt.Errorf("不支持的网盘类型: %s", d.Type)
}

// httpDoer 是 httpclient.Client 的最小行为接口（便于单测注入 fake）。
type httpDoer interface {
	Do(ctx context.Context, method, rawURL string, body []byte,
		headers map[string]string, cookieStr string, ssrf bool) (*httpclient.Response, error)
}

// cleanCloudPath 规范化云盘路径：统一正斜杠、清理 .. 与多余分隔，以 / 开头，无尾斜杠（根="/"）。
func cleanCloudPath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	p = path.Clean(p)
	if p == "." || p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

// maxPathSegments 云盘路径逐级解析的段数上限：防恶意超长路径触发逐段串行 API 调用（配额/延迟）。
const maxPathSegments = 32

// splitSegments 把云盘路径拆为非空段（用于逐级 resolvePath）。
func splitSegments(p string) []string {
	p = cleanCloudPath(p)
	if p == "/" {
		return nil
	}
	segs := strings.Split(strings.TrimPrefix(p, "/"), "/")
	if len(segs) > maxPathSegments {
		segs = segs[:maxPathSegments]
	}
	return segs
}

// joinCloud 拼接云盘路径段为标准路径。
func joinCloud(parent, name string) string {
	parent = cleanCloudPath(parent)
	if parent == "/" {
		return "/" + name
	}
	return parent + "/" + name
}

// MediaExts STRM 生成时识别为媒体文件的扩展名集合。
var mediaExts = map[string]bool{
	".mkv": true, ".mp4": true, ".ts": true, ".m2ts": true, ".avi": true, ".mov": true,
	".wmv": true, ".flv": true, ".m4v": true, ".webm": true, ".iso": true, ".rmvb": true, ".rm": true,
}

// IsMediaFile 按扩展名判定是否媒体文件（STRM 生成目标）。
func IsMediaFile(name string) bool {
	return mediaExts[strings.ToLower(path.Ext(name))]
}
