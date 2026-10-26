package chain

import (
	"os"
	"strings"
	"testing"
)

// #337 链层自造的英文原因会**原样进 MCP 工具的错误面**：app 层 mcpErr 走 errtext.Text，
// 识别得了的哨兵能翻成人话，识别不了的自产原文一字不改地交给模型
// （模型只看到 "cloud task store not wired" 这类三个英文词，既无字段名也无下一步，只能反复瞎试）。
// 判据取「反向清单」：旧英文串不许回来，新中文必须在——两条各钉一侧，扫描本身坏掉时会一起红。

func TestChainEnglishReasonsAreGone(t *testing.T) {
	cases := []struct {
		file   string
		banned string
		want   string
	}{
		{"cloud.go", `errors.New("cloud task store not wired")`, "云盘任务存储未启用"},
		{"entertainment.go", `errors.New("game not found for site")`, "该站点没有这个游戏"},
		{"message.go", `errors.New("reply url host does not match site base url")`, "回复地址与站点首页不同域"},
		{"message.go", `errors.New("delete url host does not match site base url")`, "删除地址与站点首页不同域"},
		{"site_capability.go", `errors.New("detail url host does not match site base url")`, "详情页地址与站点首页不同域"},
	}
	seen := map[string]string{}
	for _, c := range cases {
		code, ok := seen[c.file]
		if !ok {
			src, err := os.ReadFile(c.file)
			if err != nil {
				t.Fatalf("读不到 %s：%v（这条扫描本身就没打到位）", c.file, err)
			}
			code = stripStatComments(string(src))
			// 正控前置：该文件里确实有 errors.New( 构造，否则「旧串不见了」可能是读错文件。
			if !strings.Contains(code, "errors.New(") {
				t.Fatalf("%s 里没有 errors.New( 构造，判据是空的", c.file)
			}
			seen[c.file] = code
		}
		if strings.Contains(code, c.banned) {
			t.Errorf("%s 里旧英文原因又回来了：%s", c.file, c.banned)
		}
		if !strings.Contains(code, c.want) {
			t.Errorf("%s 必须带可操作中文「%s」（MCP 面唯一的线索就是这句）", c.file, c.want)
		}
	}
}

// TestSentinelIdentityKept 哨兵只改文案不改身份：娱乐出口靠 errors.Is 归类成
// 「该站未注册此游戏」，一旦有人把它换成 fmt.Errorf 包一层，分类就退化。
func TestSentinelIdentityKept(t *testing.T) {
	src, err := os.ReadFile("entertainment.go")
	if err != nil {
		t.Fatalf("读不到 entertainment.go：%v", err)
	}
	code := stripStatComments(string(src))
	if !strings.Contains(code, "var ErrGameNotFound = errors.New(") {
		t.Errorf("ErrGameNotFound 必须保持 errors.New 哨兵（不能被 fmt.Errorf 包装，否则 errors.Is 失效）")
	}
}
