package assessment

import (
	"context"
	"strings"
	"testing"
)

func mustParseMTeam(t *testing.T, body []byte) []Task {
	t.Helper()
	tasks, err := parseMTeamExamine(body)
	if err != nil {
		t.Fatalf("parseMTeamExamine: %v", err)
	}
	return tasks
}

func mustParseSunnypt(t *testing.T, body []byte) []Task {
	t.Helper()
	tasks, err := parseSunnyptAssessments(body)
	if err != nil {
		t.Fatalf("parseSunnyptAssessments: %v", err)
	}
	return tasks
}

// 考核信封的两类失败（响应不是 JSON / code!=0）必须报错。
// 旧写法一律 return nil，考核页于是显示「暂无考核」——正处考核期的用户
// 看不到任何提醒，而漏达标在 PT 站是直接降级/封号的后果。

var envelopeFailures = []struct {
	name    string
	mteam   string
	sunnypt string
	wantSub string
}{
	{"错误页 HTML", `<html><body>502 Bad Gateway</body></html>`, `<html><body>请重新登录</body></html>`, "不是 JSON"},
	{"业务错误码", `{"code":40706,"message":"cookie is invalid"}`, `{"code":401,"msg":"token 已失效"}`, "code="},
}

func TestMTeamExamineRejectsEnvelopeErrors(t *testing.T) {
	for _, tc := range envelopeFailures {
		t.Run(tc.name, func(t *testing.T) {
			tasks, err := parseMTeamExamine([]byte(tc.mteam))
			if err == nil {
				t.Fatalf("必须报错，got %d 条考核（=暂无考核）", len(tasks))
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("错误应包含 %q，got %v", tc.wantSub, err)
			}
			if tasks != nil {
				t.Errorf("报错时不应返回任务，got %+v", tasks)
			}
		})
	}
}

func TestSunnyptAssessmentsRejectsEnvelopeErrors(t *testing.T) {
	for _, tc := range envelopeFailures {
		t.Run(tc.name, func(t *testing.T) {
			tasks, err := parseSunnyptAssessments([]byte(tc.sunnypt))
			if err == nil {
				t.Fatalf("必须报错，got %d 条考核（=暂无考核）", len(tasks))
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("错误应包含 %q，got %v", tc.wantSub, err)
			}
			if tasks != nil {
				t.Errorf("报错时不应返回任务，got %+v", tasks)
			}
		})
	}
}

// TestExamineEmptyListIsNotAnError 正控（关键边界）：code=0 且列表真为空 = 「确实没有进行中的考核」，
// 必须照常返回 0 条无错误；否则没在考核的用户会永远看到读取失败。
func TestExamineEmptyListIsNotAnError(t *testing.T) {
	tasks, err := parseMTeamExamine([]byte(`{"code":0,"message":"success","data":[]}`))
	if err != nil || len(tasks) != 0 {
		t.Fatalf("馒头空列表应 0 条无错误，got tasks=%d err=%v", len(tasks), err)
	}
	tasks, err = parseSunnyptAssessments([]byte(`{"code":0,"data":{"list":[],"total":0},"msg":"ok"}`))
	if err != nil || len(tasks) != 0 {
		t.Fatalf("sunnypt 空列表应 0 条无错误，got tasks=%d err=%v", len(tasks), err)
	}
}

type fixedDoer struct {
	status int
	body   string
	dos    int
}

func (f *fixedDoer) Get(context.Context, string, string, bool) (*Response, error) {
	return &Response{StatusCode: f.status, Body: []byte(f.body)}, nil
}

func (f *fixedDoer) Do(context.Context, string, string, []byte, map[string]string, string, bool) (*Response, error) {
	f.dos++
	return &Response{StatusCode: f.status, Body: []byte(f.body)}, nil
}

// TestMTeamAdapterPropagatesEnvelopeError 适配器层：HTTP 200 + code!=0 必须把错误上抛，
// 而不是回「没有考核」。
func TestMTeamAdapterPropagatesEnvelopeError(t *testing.T) {
	hc := &fixedDoer{status: 200, body: `{"code":40706,"message":"cookie is invalid","data":[]}`}
	tasks, err := MTeamAdapter{}.FetchAssessments(context.Background(), hc, Def{}, "https://kp.m-team.cc", "apikey", "1")
	if err == nil {
		t.Fatalf("必须报错，got %d 条考核", len(tasks))
	}
	if !strings.Contains(err.Error(), "cookie is invalid") {
		t.Errorf("错误应带上服务端消息，got %v", err)
	}
	if hc.dos != 1 {
		t.Errorf("应只发一次请求，got %d", hc.dos)
	}
}
