package chain

import "testing"

// TestAllowedDirectHeader 直链响应头白名单（P0 回归）：
// DirectLink.Header 可能携带账号凭据（夸克 Cookie=drive.Token、115 开放平台 Bearer），
// 302 时只允许 Referer/UA/Range/Accept/Content-Type 以 X-Direct-* 透出，
// 凭据类头必须被白名单挡下，且新出现的未知头默认不放行（fail-closed）。
func TestAllowedDirectHeader(t *testing.T) {
	allowed := []string{
		"Referer", "referer", "User-Agent", "user-agent",
		"Range", "range", "Accept", "accept", "Content-Type", "content-type",
	}
	for _, k := range allowed {
		if !allowedDirectHeader(k) {
			t.Errorf("头 %q 应在白名单内", k)
		}
	}
	blocked := []string{
		"Cookie", "cookie", "Authorization", "authorization",
		"X-Api-Key", "Set-Cookie", "Proxy-Authorization", "Token", "x-custom-new",
	}
	for _, k := range blocked {
		if allowedDirectHeader(k) {
			t.Errorf("凭据/未知头 %q 必须被白名单挡下", k)
		}
	}
}
