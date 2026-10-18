package chain

import (
	"context"
	"fmt"
	"testing"

	"github.com/hxpt/ptpatronus/internal/errtext"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// errCheckinAdapter 按注入的错误失败的签到适配器。
type errCheckinAdapter struct {
	*fakeAdapter
	err error
}

func (a *errCheckinAdapter) Checkin(context.Context, *site.SiteMetadata, string, string) (*site.CheckinResult, error) {
	return nil, a.err
}

// TestCheckinAllErrorColumnIsUserFacing 一键签到的结果表每站一行，error 列直接进界面。
// 修复前这里写的是 err.Error()，内网站点（本工具的常规用法）看到的是
// "ssrf: ssrf: blocked private/internal address: 127.0.0.1"——既看不懂也没说该怎么做；
// 而站点自己给的中文原文必须一字不改地透传，那是用户唯一的线索。
func TestCheckinAllErrorColumnIsUserFacing(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"SSRF 拦截", fmt.Errorf("fetch: %w", httpclient.ErrBlocked), errtext.SSRFHint},
		{"站点原文照传", fmt.Errorf("该站点登录态已失效，请更新 Cookie"), "该站点登录态已失效，请更新 Cookie"},
	}
	for i, tc := range cases {
		dom := fmt.Sprintf("case%d.example", i)
		ch, siteOp := newSearchChain(t, &errCheckinAdapter{
			fakeAdapter: &fakeAdapter{schema: "nexusphp"},
			err:         tc.err,
		})
		createActive(t, siteOp, dom, "nexusphp")

		results, _ := ch.CheckinAll(context.Background())
		if len(results) != 1 {
			t.Fatalf("%s: results len=%d want 1 (%+v)", tc.name, len(results), results)
		}
		if results[0].Success {
			t.Fatalf("%s: 适配器失败却被记成成功", tc.name)
		}
		if results[0].Error != tc.want {
			t.Fatalf("%s: error 列应为用户可读文案\n got:  %q\nwant: %q", tc.name, results[0].Error, tc.want)
		}
	}
}
