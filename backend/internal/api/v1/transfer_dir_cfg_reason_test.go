package v1

// #368 整理配置的两个写入口把链层写给用户的中文校验丢进日志，界面只剩一句通用话术。
//
// transfer 链层在 SaveDirTags / SaveDirSyncTasks 里逐条写好了「该改哪个」：
//
//	dirtags.go:49  "标签名不能为空"
//	dirtags.go:52  "标签名重复: %s"
//	dirtags.go:56  "标签 %s 类型非法（local|cloud）"
//	dirtags.go:59  "标签 %s 路径为空"
//	dirsync.go:75  "任务名空或重复: %s"
//	dirsync.go:81  "任务 %s 同步方式非法: %s"
//	dirsync.go:84  "任务 %s 源/目标目录为空"
//
// 两个接口出口都写成：
//
//	if err := transfer.SaveDirTags(h.ch.KV(), in.Tags); err != nil {
//		errcode.Respond(c, errcode.InvalidInput.WithErr(err))   // ← 原因全丢
//	}
//
// WithErr 只把 err 挂到 Err 上供日志，Message 仍是码的默认文案
// （errcode.go:56 WithDetail 才会覆盖 —— 两者差别就在这里）。
// 于是用户配了 5 个标签、其中第 3 个名字重复，界面只说
// 「请求有参数后端无法接受，但没能指出具体是哪一项」，得自己一个个试。
//
// 注意这与 oper 层的 InputReasonErr 不同：那边用 invalidInput 类型包装，
// respondRecordError 能接住；transfer 这边是裸 fmt.Errorf，类型信息没有，
// 所以本测试钉的是「响应体里能看到链层那句中文」这个行为。
//
// 同族：#335/#336/#360/#366/#367（原因在出口处被吞）。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/transfer"
)

// kvStub 极简 KVStore：写入口的校验发生在 kv.Set 之前，用不到写。
type kvStub struct{}

func (kvStub) Get(string) (string, error) { return "", nil }
func (kvStub) Set(string, string) error   { return nil }

// classifyDirCfg 走真实的分类出口 classifyDirCfgErr → respondDirCfgErr。
func classifyDirCfg(t *testing.T, err error) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/x", strings.NewReader("{}"))
	respondDirCfgErr(c, err)
	var env map[string]any
	if e := json.Unmarshal(rec.Body.Bytes(), &env); e != nil {
		t.Fatalf("响应不是 JSON：%q", rec.Body.String())
	}
	return rec.Code, env
}

// TestDirCfgReasonReachesClient 链层写给用户的中文校验必须原样抵达界面。
func TestDirCfgReasonReachesClient(t *testing.T) {
	// 用真实的链层校验（不是手搓错误），保证判据跟着链层走。
	cases := []struct {
		name string
		run  func(kv transfer.KVStore) error
		want string
	}{
		{"目录标签重名", func(kv transfer.KVStore) error {
			return transfer.SaveDirTags(kv, []transfer.DirTag{
				{Name: "电影", Kind: "local", Path: "/m"},
				{Name: "电影", Kind: "local", Path: "/n"},
			})
		}, "重复"},
		{"目录标签类型非法", func(kv transfer.KVStore) error {
			return transfer.SaveDirTags(kv, []transfer.DirTag{{Name: "x", Kind: "bogus", Path: "/p"}})
		}, "类型非法"},
		{"目录标签路径为空", func(kv transfer.KVStore) error {
			return transfer.SaveDirTags(kv, []transfer.DirTag{{Name: "x", Kind: "local", Path: "  "}})
		}, "路径为空"},
		{"目录同步任务重名", func(kv transfer.KVStore) error {
			// 源/目标目录要填全：链层的校验顺序上「目录为空」在「重名」之前，
			// 路径留空会先撞上那条，测的就不是重名了。
			return transfer.SaveDirSyncTasks(kv, []transfer.DirSyncTask{
				{Name: "t", Mode: "copy", SrcDir: "/a", DstDir: "/b"},
				{Name: "t", Mode: "copy", SrcDir: "/c", DstDir: "/d"},
			})
		}, "重复"},
		{"目录同步方式非法", func(kv transfer.KVStore) error {
			return transfer.SaveDirSyncTasks(kv, []transfer.DirSyncTask{{Name: "t", Mode: "bogus"}})
		}, "同步方式非法"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run(kvStub{})
			if err == nil {
				t.Fatalf("这条输入本该被链层拒绝（测试自己搭坏了）")
			}
			status, env := classifyDirCfg(t, err)
			msg, _ := env["message"].(string)
			if status == http.StatusOK {
				t.Error("配置写失败不许回 200")
			}
			if !strings.Contains(msg, tc.want) {
				t.Errorf("界面该看到链层那句具体原因（含 %q），实际 %q", tc.want, msg)
			}
		})
	}
}

// TestDirCfgOutletsUseClassifier 接线护栏：两个写入口都必须走 respondDirCfgErr。
//
// 这一条是被上一轮的教训逼出来的：分类器写对了、测试也全绿，但两个接口出口
// 还在用 `InvalidInput.WithErr(err)` —— 行为测试直接调分类器，从没驱动过 handler，
// 于是出口没接上这件事完全测不出来（新加的出口没找它的读者）。
// 这里钉住 handler 源码；实现挪走时同步改判据。
func TestDirCfgOutletsUseClassifier(t *testing.T) {
	src, err := os.ReadFile("transfer.go")
	if err != nil {
		t.Fatalf("读不到 transfer.go：%v", err)
	}
	code := stripWholeLineComments(string(src)) // 剥注释：注释里会引用旧形状的字面文本
	for _, fn := range []string{"SetDirTags", "SetDirSync"} {
		at := strings.Index(code, "func (h *TransferHandler) "+fn+"(")
		if at < 0 {
			t.Fatalf("找不到 %s（判据没有落点）", fn)
		}
		tail := code[at:]
		if n := strings.Index(tail[1:], "\nfunc "); n > 0 {
			tail = tail[:1+n]
		}
		// 只看链层保存那一处：同一函数里 ShouldBindJSON 的 InvalidInput.WithErr
		// 是**对的**（入参绑定错误本来就该只进日志、界面回通用话术），
		// 判据切到整个函数体就会连它一起骂 —— 过宽判据会逼下一个人去改坏代码。
		saveAt := strings.Index(tail, "SaveDir")
		if saveAt < 0 {
			t.Errorf("%s 找不到链层保存调用（判据没有落点）", fn)
			continue
		}
		// 保存调用之后、函数结束之前，不该再出现裸的 InvalidInput.WithErr。
		after := tail[saveAt:]
		if strings.Contains(after, "InvalidInput.WithErr(err)") {
			t.Errorf("%s 的链层保存失败不许用 WithErr（旧形状：原因丢进日志，界面只剩通用话术）", fn)
		}
		if !strings.Contains(after, "respondDirCfgErr(c, err)") {
			t.Errorf("%s 必须走 respondDirCfgErr（分类对了出口没接上是空转）", fn)
		}
		if !strings.Contains(tail, "respondDirCfgErr(c, err)") {
			t.Errorf("%s 必须走 respondDirCfgErr（分类对了出口没接上是空转）", fn)
		}
	}
}
