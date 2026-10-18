package cloud

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// 夸克 / 123 的写接口与 115 同族：HTTP 200，成败写在正文里。
// 旧判据 `json.Unmarshal(...) == nil && 失败位` 有两个洞：解析不出来（HTML 拦截页、
// 空 body、网关改写）被短路成「成功」；失败位成立但原因字段为空时留下「失败: 」半句话。

func TestJudgeQuarkActionBody(t *testing.T) {
	if err := judgeQuarkActionBody("move", []byte(`{"status":200,"code":0}`), 200, 0, ""); err != nil {
		t.Errorf("status=200 code=0 应判成功, got %v", err)
	}
	err := judgeQuarkActionBody("move", []byte("<html>风控拦截</html>"), 0, 0, "")
	if err == nil {
		t.Fatal("非 JSON 正文不能算成功（旧行为正是如此）")
	}
	if !strings.Contains(err.Error(), "未确认成功") || !strings.Contains(err.Error(), "风控拦截") {
		t.Errorf("要说明未能确认并带原文片段, got %q", err.Error())
	}
	// 失败位成立且没有 message 字段：回退到 status/code，不能是空原因
	if msg := judgeQuarkActionBody("move", []byte(`{"status":404}`), 404, 0, "").Error(); strings.HasSuffix(msg, "失败: ") {
		t.Errorf("空原因文案复现：%q", msg)
	} else if !strings.Contains(msg, "status=404") {
		t.Errorf("原因要能定位, got %q", msg)
	}
}

func TestJudge123Body(t *testing.T) {
	if err := judge123Body("delete", []byte(`{"code":0,"message":"成功"}`), 0, "成功"); err != nil {
		t.Errorf("code=0 应判成功, got %v", err)
	}
	err := judge123Body("delete", []byte(""), 0, "")
	if err == nil {
		t.Fatal("空 body 不能算成功")
	}
	if !strings.Contains(err.Error(), "未确认成功") {
		t.Errorf("got %q", err.Error())
	}
	// code!=0 且没有 message：回退成「错误码 N」
	if msg := judge123Body("delete", []byte(`{"code":3106}`), 3106, "").Error(); !strings.Contains(msg, "3106") {
		t.Errorf("空 message 时要回退到错误码, got %q", msg)
	}
}

// htmlDoer123 让 123 的所有应答都是 HTML：验的是 call123Query 这条**出口**，
// 不只是判据函数（判据绿不等于出口接上了）。
type htmlDoer123 struct{ n int }

func (d *htmlDoer123) Do(_ context.Context, _, _ string, _ []byte, _ map[string]string, _ string, _ bool) (*httpclient.Response, error) {
	d.n++
	return &httpclient.Response{StatusCode: 200, Body: []byte(`<html>网关改写</html>`)}, nil
}

func Test123QueryOutletRejectsHTML(t *testing.T) {
	doer := &htmlDoer123{}
	c := &client123{drive: Drive{Type: "123", Token: "t"}, hc: doer}
	err := c.call123Query(context.Background(), "/v2/file/delete", url.Values{"file_id": {"9"}}, "delete")
	if err == nil {
		t.Fatal("写接口收到 HTML 被报成成功（缺陷原形）")
	}
	if !strings.Contains(err.Error(), "123 delete") {
		t.Fatalf("红必须来自这条出口, got %q", err.Error())
	}
	if doer.n == 0 {
		t.Error("根本没打到 123 接口")
	}
}

// TestQuarkFileActionOutletRejectsHTML 补夸克那条出口的同一件事：Move/Copy/Rename/Delete
// 全走 fileAction，从前解析失败就 return nil，界面弹「已重命名」而网盘上没变。
// 判据函数测到了不等于出口接上了——撤掉接线这条就该红。
func TestQuarkFileActionOutletRejectsHTML(t *testing.T) {
	c := &clientQuark{drive: Drive{Type: "quark", Token: "ck"}, hc: &htmlDoer123{}}
	err := c.fileAction(context.Background(), "rename", map[string]any{"file_name": "b.mkv"})
	if err == nil {
		t.Fatal("夸克写接口收到 HTML 被报成成功（缺陷原形：Unmarshal 失败即视为完成）")
	}
	if !strings.Contains(err.Error(), "quark rename") {
		t.Fatalf("红必须来自 fileAction 这条出口, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "未确认成功") {
		t.Errorf("要说清未能确认并带原文片段, got %q", err.Error())
	}
}
