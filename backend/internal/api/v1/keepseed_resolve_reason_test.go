package v1

// #369 辅种「处理候选」的三条人话原因被 respondKeepSeedErr 的兜底档丢掉，
// 其中一条还是纯英文。
//
// 链层写得很清楚：
//
//	keepseed_resolve.go:44  "MTeam 辅种不可用：未注入 mteam 客户端"
//	keepseed_resolve.go:55  "MTeam 站未配置 api_key"
//	keepseed_resolve.go:48  "load site %d: %w"     ← 英文句式 + %w 保留底层原因
//	keepseed_resolve.go:139 "invalid action: " + action   ← 纯英文
//
// 三条都经 Resolve 返回到 Resolve 出口的 respondKeepSeedErr，而它的最后一行是
//
//	respondInputError(c, err, errcode.InvalidInput)
//	  → respondRecordError(c, err, nil, InvalidInput, InvalidInput)
//	    → 兜底档 errcode.Respond(c, fallback.WithErr(err))
//
// WithErr 只挂 Err 不动 Message ⇒ 界面只剩「没能指出具体是哪一项」。
// 前两条尤其冤：它们说的都是**用户下一步该做什么**（配置 api_key / 检查注入），
// 而界面让用户去「对照接口文档核对必填项」—— 那是对着 HTTP 文档改表单的提示，
// 与此毫无关系。同族：#335/#336/#360/#366/#367/#368。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/downloader"
)

func classifyKeepSeed(t *testing.T, err error) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	respondKeepSeedErr(c, err)
	var env map[string]any
	if e := json.Unmarshal(rec.Body.Bytes(), &env); e != nil {
		t.Fatalf("响应不是 JSON：%q", rec.Body.String())
	}
	return rec.Code, env
}

// TestKeepSeedResolveReasonReachesClient 链层写给用户的原因必须原样抵达界面。
func TestKeepSeedResolveReasonReachesClient(t *testing.T) {
	// 喂**真实哨兵**，不是手搓的裸 errors.New：放行判定走的是 errors.Is，
	// 裸同文案不会被匹配上——那样这条测试会绿得毫无意义（第一版就是这么写的）。
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"MTeam 客户端没注入", chain.ErrNoMTeamClient, "未注入 mteam 客户端"},
		{"MTeam 站没配 api_key", chain.ErrMTeamNoAPIKey, "api_key"},
		{"操作填错", fmt.Errorf("%w（收到 %q）", chain.ErrBadAction, "bogus"), "只支持 add"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, env := classifyKeepSeed(t, tc.err)
			msg, _ := env["message"].(string)
			if !strings.Contains(msg, tc.want) {
				t.Errorf("界面该看到链层那句具体原因（含 %q），实际 %q", tc.want, msg)
			}
			if status == http.StatusOK {
				t.Error("处理失败不许回 200")
			}
		})
	}
}

// TestKeepSeedInvalidActionIsChinese 「非法 action」不许把英文原样丢给界面。
func TestKeepSeedInvalidActionIsChinese(t *testing.T) {
	// 链层现在会回 `不支持的操作 "bogus"：只支持 add（添加辅种）或 skip（跳过）`。
	// 断言界面看到的是这句，而不是从前那句纯英文 `invalid action: bogus`。
	_, env := classifyKeepSeed(t, fmt.Errorf("%w（收到 %q）", chain.ErrBadAction, "bogus"))
	msg, _ := env["message"].(string)
	if strings.Contains(msg, "invalid action") {
		t.Errorf("「invalid action」是纯英文，直接进界面等于没翻译：%q", msg)
	}
	if !strings.Contains(msg, "只支持 add") {
		t.Errorf("该告诉用户可填什么，实际 %q", msg)
	}
	// 兜底档也必须带上原话（含用户传的 action），否则用户不知道是自己参数写错了。
	if !strings.Contains(msg, "bogus") {
		t.Errorf("界面该带出用户传的那个 action，实际 %q", msg)
	}
}

// 反面对照：下载器连不上仍是 502 + 可操作中文（别把 #369 改成把所有原因都糊成 400）。
func TestKeepSeedDownloaderOfflineUnaffected(t *testing.T) {
	status, env := classifyKeepSeed(t, downloader.ErrCannotConnect)
	if status != http.StatusBadGateway {
		t.Errorf("下载器连不上该走 502，got %d", status)
	}
	msg, _ := env["message"].(string)
	if !strings.Contains(msg, "下载器") {
		t.Errorf("该说清是下载器那一侧的问题，实际 %q", msg)
	}
}

// TestKeepSeedChainReasonsAreChinese 链层自己产出的原因不许是英文原文。
//
// 这条是被负控逼出来的：把 resolveCandidateURL / Resolve 的文案还原成
// `load site %d` 与 `invalid action: ` 时，上面几条**依然全绿** —— 它们只走接口层，
// 从没读过链层那句。判定跑在接口层，链层回退成英文它就看不见了。
// 同族：#349（transfer 链层英文原因）、#363（Redo 那句 has no download hash）。
func TestKeepSeedChainReasonsAreChinese(t *testing.T) {
	// api/v1 与 chain 是兄弟包 ⇒ ../../chain/。go test 的工作目录就是包目录。
	src, err := os.ReadFile(filepath.Join("..", "..", "chain", "keepseed_resolve.go"))
	if err != nil {
		t.Fatalf("读不到 keepseed_resolve.go：%v", err)
	}
	// 剥掉整行注释：注释里会引用旧形状的字面文本。
	var lines []string
	for _, l := range strings.Split(string(src), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		lines = append(lines, l)
	}
	code := strings.Join(lines, "\n")
	for _, banned := range []string{
		`fmt.Errorf("load site %d`,
		`errors.New("invalid action: "`,
		`"invalid action: %s"`,
		`fmt.Errorf("invalid action`,
	} {
		if strings.Contains(code, banned) {
			t.Errorf("链层不许再产英文原文 %q：它会经 WithDetail 原样进界面（用户看到的是英文）", banned)
		}
	}
	// 两条自产原因都要挂哨兵，接口层才能与「内部故障」区分开。
	for _, want := range []string{"ErrNoMTeamClient", "ErrMTeamNoAPIKey", "ErrBadAction"} {
		if !strings.Contains(code, want) {
			t.Errorf("缺哨兵 %s：没有它，接口层无法把用户侧原因与内部故障区分（旧形状：要么被压成通用话术，要么把内部原文泄出去）", want)
		}
	}
	// 哨兵要真被返回路径用上，而不是定义了却没人用（那样接口层的 errors.Is 永远不命中）。
	// 只认两种返回形状：`return "", ErrX` 与 fmt.Errorf 的 "%w…", ErrX 形式。
	for _, fn := range []string{"ErrNoMTeamClient", "ErrMTeamNoAPIKey", "ErrBadAction"} {
		// `return "", ErrX` 或 `fmt.Errorf("%w…", ErrX, …)` —— 后者末尾可能还带别的参数，
		// 所以只要求「该哨兵出现在某个 return 语句里」，不匹配结尾括号。
		direct := strings.Contains(code, `return "", `+fn)
		wrapped := strings.Contains(code, `"%w`) && strings.Contains(code, fn+`,`)
		if !direct && !wrapped {
			t.Errorf("哨兵 %s 没被任何返回路径使用：接口层的 errors.Is 会永远不命中", fn)
		}
	}
}
