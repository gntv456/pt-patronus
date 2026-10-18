package v1

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// #286 五个域的增改出口（凭据 / 监控目标 / 运维配置 / STRM 规则 / Agent）从前两种事一起犯：
//   - oper 写着「凭据名称不能为空」「规则需要 name、drive_id…必填」这种人话，
//     但用的是裸 fmt.Errorf/errors.New，接口层又 WithErr ⇒ 界面只剩通用「请求参数有误」；
//   - 库写失败（磁盘满 / 连接池关 / 唯一索引冲突）也被回成 400「参数有误」，
//     用户去改一个本来没问题的表单，反复重试。

func classify(t *testing.T, err error, notFound *errcode.Error) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	respondRecordError(c, err, notFound, errcode.InvalidInput, errcode.SaveFailed)
	var env map[string]any
	if e := json.Unmarshal(rec.Body.Bytes(), &env); e != nil {
		t.Fatalf("响应不是 JSON：%q", rec.Body.String())
	}
	return rec.Code, env
}

// oper 自产的中文原因必须原样抵达界面（WithErr 只进日志＝用户看不到下一步）。
func TestInputReasonReachesClient(t *testing.T) {
	status, env := classify(t, oper.InputReasonf("凭据名称不能为空"), errcode.OpsCredentialNotFound)
	if status != http.StatusBadRequest || env["code"] != errcode.InvalidInput.Code {
		t.Fatalf("输入原因应 400：status=%d 整包=%v", status, env)
	}
	if msg, _ := env["message"].(string); !strings.Contains(msg, "凭据名称不能为空") {
		t.Errorf("界面拿不到「哪个字段没填」：%q", msg)
	}
}

// 库故障不许再被说成参数问题：用户没有可检查的输入，重试也不会好。
func TestDBFailureIsNotParamError(t *testing.T) {
	closed := errors.New("sql: database is closed")
	status, env := classify(t, closed, errcode.OpsCredentialNotFound)
	if status != http.StatusInternalServerError || env["code"] != errcode.SaveFailed.Code {
		t.Fatalf("status=%d 整包=%v，期望 500 + %q（不能再是 400 参数有误）",
			status, env, errcode.SaveFailed.Code)
	}
	if msg, _ := env["message"].(string); strings.Contains(msg, "参数有误") {
		t.Errorf("仍让人去检查表单：%q", msg)
	}

	// 记录不存在仍然是 404（本域码），不能被新的兜底带成 500。
	status, env = classify(t, gorm.ErrRecordNotFound, errcode.OpsCredentialNotFound)
	if status != http.StatusNotFound || env["code"] != errcode.OpsCredentialNotFound.Code {
		t.Fatalf("status=%d 整包=%v，期望 404 + %q", status, env, errcode.OpsCredentialNotFound.Code)
	}
}

// 载体真的换上了：凭据/规则的空名与缺字段错误必须能被 InputReasonErr 取出来。
// （从前是裸 fmt.Errorf/errors.New，取出来是 nil ⇒ 分类层只能给通用文案。）
func TestOperValidationsCarryHumanReason(t *testing.T) {
	op := oper.NewCredentialOper(nil, nil)
	if _, err := op.Create(oper.CredentialInput{Type: "ssh"}); err == nil {
		t.Fatal("空名称必须被拒")
	} else if oper.InputReasonErr(err) == nil {
		t.Errorf("「凭据名称不能为空」没有走载体，界面读不到：%v", err)
	}
}

// 结构护栏：这五个域里「链调用之后回 400 参数有误」的出口必须清零，
// 且兜底一律 SaveFailed（分类器只接一部分出口＝假覆盖，抄仓内 docker 护栏的先例）。
func TestCrudOutletsAllClassified(t *testing.T) {
	strip := func(path string) string {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读不到 %s: %v", path, err)
		}
		var code strings.Builder
		for _, line := range strings.Split(string(src), "\n") {
			if i := strings.Index(line, "//"); i >= 0 {
				line = line[:i]
			}
			code.WriteString(line)
			code.WriteByte('\n')
		}
		return code.String()
	}
	files := []string{"credential.go", "monitor.go", "ops_config.go", "cloud.go", "agent.go"}
	var stale []string
	wired := 0
	for _, f := range files {
		code := strip(f)
		lines := strings.Split(code, "\n")
		for i, ln := range lines {
			if i == 0 || !strings.Contains(ln, "errcode.InvalidInput.WithErr(err)") {
				continue
			}
			if strings.TrimSpace(lines[i-1]) == "if err != nil {" && !strings.Contains(lines[i-2], "if err :=") {
				stale = append(stale, f+":"+itoaLine(i+1))
			}
		}
		wired += strings.Count(code, "errcode.InvalidInput, errcode.SaveFailed)")
	}
	if len(stale) != 0 {
		t.Errorf("链调用失败仍被回成 400「请求参数有误」的出口剩 %d 处：%v", len(stale), stale)
	}
	if wired < 8 {
		t.Errorf("走 SaveFailed 兜底的出口 = %d, want ≥8（凭据/监控/运维配置/规则/Agent 的增改口都要接上）", wired)
	}
}

func itoaLine(n int) string {
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}
