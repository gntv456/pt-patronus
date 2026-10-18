package schemas

import (
	"context"
	"testing"

	"github.com/hxpt/ptpatronus/internal/errtext"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// 站免的失败是「转成一条 failed 通知」而不是返回错误，所以通知里的文案
// 就是用户在卡片上看到的那句——从前直出 err.Error()，界面出现
// "ssrf: blocked private/internal address: 127.0.0.1" 这种英文原文。
func TestFetchFreeleechFailedNoticeTextIsChinese(t *testing.T) {
	hc, err := httpclient.New(httpclient.Options{})
	if err != nil {
		t.Fatal(err)
	}
	n := NewNexusPHP(hc)

	ns, err := n.FetchFreeleech(context.Background(), &site.SiteMetadata{}, "http://127.0.0.1:9/", "uid=1; passkey=k")
	if err != nil {
		t.Fatalf("站免设计上是把失败转成通知（不返回错误），got %v", err)
	}
	if len(ns) != 1 || ns[0].Status != site.FreeleechFailed {
		t.Fatalf("应回一条 failed 通知，got %+v", ns)
	}
	if ns[0].Error != errtext.SSRFHint {
		t.Errorf("通知文案必须是可操作中文，got %q", ns[0].Error)
	}
}
