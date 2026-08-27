package cloud

import (
	"context"
	"fmt"
)

// Writer 网盘写侧能力：上传 / 建目录 / 解析直链。三家客户端全部实现（123 上传走
// 官方开放 API，115/夸克走 web 契约）。STRM 只读生成不需要它；自动上传与 302 直链
// 端点经类型断言使用，保持 Client 小接口不变。
type Writer interface {
	// MkdirP 逐级创建目录（已存在视为成功），返回目标目录 ID。
	MkdirP(ctx context.Context, dir string) (string, error)
	// Upload 将本地文件 src 上传为云盘路径 dst（含文件名）。分片+秒传，已存在不重传。
	Upload(ctx context.Context, dst string, src *LocalSource) error
	// DownloadURL 解析云盘文件的限时下载直链。ua 为播放器 User-Agent——夸克/115 的
	// 直链与请求方 UA/Cookie 绑定，需原样随下载请求携带（DownloadURL 返回所需头）。
	DownloadURL(ctx context.Context, cloudPath, ua string) (*DirectLink, error)
}

// LocalSource 待上传的本地文件（路径 + 预计算摘要；SHA1 为 115 秒传所需，按需惰性计算）。
type LocalSource struct {
	Path  string
	Size  int64
	MD5   string
	SHA1  string
	sha1f func() (string, error) // 惰性 SHA1（未预计算且驱动需要时调用）
}

// NewLocalSource 构造，按需惰性计算摘要（避免未用 SHA1 的驱动白算全文件哈希）。
func NewLocalSource(path string, size int64) *LocalSource {
	return &LocalSource{Path: path, Size: size, sha1f: func() (string, error) {
		s, err := fileSHA1(path)
		if err != nil {
			return "", fmt.Errorf("sha1 %s: %w", path, err)
		}
		return s, nil
	}}
}

// SHA1OrCalc 返回预计算或惰性计算的 SHA1（大写十六进制）。
func (s *LocalSource) SHA1OrCalc() (string, error) {
	if s.SHA1 != "" {
		return s.SHA1, nil
	}
	if s.sha1f == nil {
		return "", fmt.Errorf("no sha1 source for %s", s.Path)
	}
	return s.sha1f()
}

// DirectLink 限时下载直链及其访问所需头。
type DirectLink struct {
	URL    string            `json:"url"`
	Header map[string]string `json:"header,omitempty"` // 下载时须随请求携带（Cookie/UA/Referer）
}

// TokenRefresher 开放平台类网盘的令牌自愈能力（chain 注入回写钩子）。
type TokenRefresher interface {
	// SetTokenRefreshHook 刷新成功后回调（newToken 为可直接持久化的存储串）。
	SetTokenRefreshHook(fn func(newToken string))
}
