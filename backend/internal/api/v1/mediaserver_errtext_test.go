package v1

import (
	"errors"
	"net"
	"net/url"
	"testing"

	"github.com/hxpt/ptpatronus/internal/errtext"
	"gorm.io/gorm"
)

// 「测试连接」失败会以 200+{ok:false,error} 原样弹到界面：传输层失败必须是
// errtext 的中文口径（与站点/下载器测试一致），不能是 dial tcp/connectex 原文。
func TestMediaServerErrTextMapsTransport(t *testing.T) {
	dialErr := &url.Error{
		Op:  "Get",
		URL: "http://127.0.0.1:19997/System/Info",
		Err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connectex: No connection could be made")},
	}
	if got := mediaServerErrText(dialErr); got != errtext.Unreachable {
		t.Fatalf("连接被拒应翻成中文可操作提示, got %q", got)
	}
	if got := mediaServerErrText(gorm.ErrRecordNotFound); got != "媒体服务器不存在" {
		t.Fatalf("record not found 应保持既有中文, got %q", got)
	}
	// 认不出的上游原文要透传：那是用户唯一的线索，不做二次加工。
	raw := errors.New("Emby 返回 401 Unauthorized")
	if got := mediaServerErrText(raw); got != raw.Error() {
		t.Fatalf("未识别的上游错误应原样透传, got %q", got)
	}
}
