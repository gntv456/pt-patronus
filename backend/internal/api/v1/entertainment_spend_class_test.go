// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/chain"
	"gorm.io/gorm"
)

// #276：娱乐域「花钱」与「读数据」的失败从前只有「重复提交→429」被区分，
// 其余一律 400「请求参数有误，请检查后重试」——而链层本来就写着人话原因
// （该游戏不支持连抽/购卡/合成…），站点失败与扣费语义更是两回事。

func callClassifier(t *testing.T, fn func(*gin.Context, error), err error) (int, map[string]string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/entertainment/1/games/wheel/play", nil)
	fn(c, err)
	// 信封可能带非字符串的标记字段（如 specific=true），先宽松解再只取字符串项：
	// 按 map[string]string 硬解会让"新增一个布尔字段"这种兼容改动整包报错。
	var raw map[string]any
	if e := json.Unmarshal(rec.Body.Bytes(), &raw); e != nil {
		t.Fatalf("响应不是 JSON：%q", rec.Body.String())
	}
	env := map[string]string{}
	for k, v := range raw {
		if s, ok := v.(string); ok {
			env[k] = s
		}
	}
	return rec.Code, env
}

func TestSpendErrSeparatesThreeSituations(t *testing.T) {
	t.Run("不支持是人话原因，必须抵达界面", func(t *testing.T) {
		status, env := callClassifier(t, spendErr, fmt.Errorf("%w连抽", chain.ErrGameUnsupported))
		if status != http.StatusConflict || env["code"] != errcode.EntertainmentGameUnsupported.Code {
			t.Fatalf("status=%d env=%v，期望 409 + %q", status, env, errcode.EntertainmentGameUnsupported.Code)
		}
		if !stringsContains(env["message"], "连抽") {
			t.Errorf("原因被压掉了：message=%q", env["message"])
		}
		if stringsContains(env["message"], "参数") {
			t.Errorf("仍在让用户去检查输入：%q", env["message"])
		}
	})

	t.Run("站点失败要说清可能已扣费", func(t *testing.T) {
		status, env := callClassifier(t, spendErr, errors.New("dial tcp: i/o timeout"))
		if status != http.StatusBadGateway || env["code"] != errcode.EntertainmentSpendFailed.Code {
			t.Fatalf("status=%d env=%v，期望 502 + %q", status, env, errcode.EntertainmentSpendFailed.Code)
		}
		if !stringsContains(env["message"], "扣费") || !stringsContains(env["message"], "余额") {
			t.Errorf("花钱失败必须交代「币扣没扣」：%q", env["message"])
		}
	})

	t.Run("读侧同一件事不谈扣费", func(t *testing.T) {
		status, env := callClassifier(t, entReadErr, errors.New("dial tcp: i/o timeout"))
		if status != http.StatusBadGateway || env["code"] != errcode.EntertainmentDataUnreadable.Code {
			t.Fatalf("status=%d env=%v，期望 502 + %q", status, env, errcode.EntertainmentDataUnreadable.Code)
		}
		if stringsContains(env["message"], "扣费") {
			t.Errorf("读数据失败谈扣费会误导（这一项什么都没消耗）：%q", env["message"])
		}
	})

	// 正控：既有的重复提交档不许被我的分类改坏。
	t.Run("正控：重复提交仍是 429", func(t *testing.T) {
		status, env := callClassifier(t, spendErr, chain.ErrDuplicateSpend)
		if status != http.StatusTooManyRequests || env["code"] != errcode.DuplicateSpend.Code {
			t.Fatalf("status=%d env=%v，期望 429 + %q", status, env, errcode.DuplicateSpend.Code)
		}
	})

	t.Run("引用失效是 404，不是上游故障", func(t *testing.T) {
		status, env := callClassifier(t, spendErr, chain.ErrGameNotFound)
		if status != http.StatusNotFound || env["code"] != errcode.EntertainmentGameNotFound.Code {
			t.Fatalf("status=%d env=%v，期望 404 + %q", status, env, errcode.EntertainmentGameNotFound.Code)
		}
		// 站点被删：链层抛 gorm 记录不存在，同样是引用失效而非 502。
		status, env = callClassifier(t, entReadErr, gorm.ErrRecordNotFound)
		if status != http.StatusNotFound || env["code"] != errcode.SiteNotFound.Code {
			t.Fatalf("status=%d env=%v，期望 404 + %q", status, env, errcode.SiteNotFound.Code)
		}
	})
}

func stringsContains(s, sub string) bool { return strings.Contains(s, sub) }

// TestEntertainmentClassifiersWiredEverywhere 是「判据接满」的结构护栏：分类器只接在
// 一部分出口上等于假覆盖——娱乐页开着的面板有几十个，用户点哪个都可能撞上失败。
// 护栏只扫真源码（先剥 // 注释，免得我自己写的说明文字造假红）。
func TestEntertainmentClassifiersWiredEverywhere(t *testing.T) {
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

	// 1) 链层「不支持」必须全部挂哨兵：裸 errors.New 的那 26 处会被分类器漏成 502。
	//    （哨兵本体在 entertainment_capability.go，不在扫描范围内。）
	for _, f := range []string{"../../chain/entertainment_actions.go", "../../chain/entertainment.go"} {
		if n := strings.Count(strip(f), `errors.New("该游戏不支持`); n != 0 {
			t.Errorf("%s 里裸「该游戏不支持」= %d, want 0（要写成 fmt.Errorf(\"%%w…\", ErrGameUnsupported)，否则接口层认不出这一档）", f, n)
		}
	}

	// 2) 娱乐域 handler 里「链调用之后回 400 参数有误」的出口必须清零。
	//    绑定失败（if err := c.ShouldBindJSON…; err != nil）不在此列——那是真的参数问题。
	handlers, _ := filepath.Glob("entertainment*.go")
	var stale []string
	for _, f := range handlers {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		lines := strings.Split(strip(f), "\n")
		for i, ln := range lines {
			if i == 0 || !strings.Contains(ln, "errcode.InvalidInput.WithErr(err)") {
				continue
			}
			if strings.TrimSpace(lines[i-1]) == "if err != nil {" && !strings.Contains(lines[i-2], "if err :=") {
				stale = append(stale, fmt.Sprintf("%s:%d", f, i+1))
			}
		}
	}
	if len(stale) != 0 {
		t.Errorf("链调用失败被回成 400「请求参数有误」的出口还剩 %d 处：%v（该走 entReadErr/spendErr）", len(stale), stale)
	}

	// 3) 兜底不许是裸 BadGateway：那会把「该换游戏」「凭据解不开」都说成「上游不可用，稍后重试」。
	for _, f := range []string{"entertainment_gobang.go"} {
		if n := strings.Count(strip(f), "errcode.Respond(c, errcode.BadGateway.WithErr(err))"); n != 0 {
			t.Errorf("%s 裸 BadGateway 出口 = %d, want 0（兜底要走公共判据）", f, n)
		}
	}
}
