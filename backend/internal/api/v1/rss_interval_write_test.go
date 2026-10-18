package v1

import (
	"os"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// #315 编辑路径不许把 interval_min=0 写进库。
// 0 不会把抓取节奏改坏（chain.CheckAll 与 app/plugin_host_workflow 各自兜底成 30 分钟），
// 但这一行会被两个客户端原样回显：Web Rss.vue「间隔 {n} 分钟」、移动 rss_page.dart
// 「间隔 ${intervalMin} 分钟」⇒ 界面写「间隔 0 分钟」而实际每 30 分钟抓一次。
// 归一此前只在 Create 做一次，Update 走同一个 applyRssReq 却没有它。

func TestApplyRssReqNormalizesInterval(t *testing.T) {
	cases := []struct {
		name string
		in   int
		want int
	}{
		{"PUT 不带这个键（零值）", 0, 30},
		{"负数", -5, 30},
		{"用户明确填 15 要照原样", 15, 15},
		{"界面下限 5 分钟仍按用户值", 5, 5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var r models.RssSubscription
			in := rssReq{Name: "源A", URL: "https://rss.example/feed", IntervalMin: c.in}
			applyRssReq(&r, in)
			if r.IntervalMin != c.want {
				t.Errorf("入库的 interval_min 要等于生效值：入参 %d 期望存 %d，实际存 %d（界面会照着这句回显给用户）",
					c.in, c.want, r.IntervalMin)
			}
			// 归一不许把别的字段带坏
			if r.Name != in.Name || r.URL != in.URL {
				t.Errorf("其余字段要照原样写入：name=%q url=%q", r.Name, r.URL)
			}
		})
	}
}

// 同一判定只许有一份（新建与编辑共用 applyRssReq）。抄第二遍的那份会先漂移——
// 本轮的缺陷本体正是「Create 归一了、Update 没有」。
func TestIntervalNormalizationLivesInOnePlace(t *testing.T) {
	src, err := os.ReadFile("rss.go")
	if err != nil {
		t.Fatalf("读不到 rss.go（go test 的工作目录是包目录）：%v", err)
	}
	body := string(src)
	for i, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "//") {
			body = strings.ReplaceAll(body, line+"\n", "\n") // 注释里提到这条判据不算重复
			_ = i
		}
	}
	if n := strings.Count(body, "in.IntervalMin <= 0"); n != 1 {
		t.Errorf("归一判据在出口层只许出现一次（收在 applyRssReq），实际 %d 次", n)
	}
	if !strings.Contains(body, "func applyRssReq") || !strings.Contains(strings.SplitAfter(body, "func applyRssReq")[1], "in.IntervalMin <= 0") {
		t.Errorf("归一必须在 applyRssReq 内（Create/Update 共用），实际不在")
	}
}

// #318 出口归类：oper 的「写给人看」输入原因（地址协议不对）必须走分类层，
// 不能像从前那样一律 errcode.Internal ⇒ 用户笔误被报成 500「服务器内部错误」，中文原因只进日志。
func TestRssWriteOutletsClassifyInputReason(t *testing.T) {
	src, err := os.ReadFile("rss.go")
	if err != nil {
		t.Fatalf("读不到 rss.go：%v", err)
	}
	code := stripGoComments(string(src))
	for _, fn := range []string{"func (h *RssHandler) Create", "func (h *RssHandler) Update"} {
		i := strings.Index(code, fn)
		if i < 0 {
			t.Fatalf("找不到 %s（判据没法落点）", fn)
		}
		j := strings.Index(code[i+1:], "\nfunc ")
		body := code[i : i+1+j]
		if strings.Contains(body, "errcode.Internal.WithErr") {
			t.Errorf("%s 把输入原因当内部故障回 500：还留着 errcode.Internal.WithErr", fn)
		}
		if !strings.Contains(body, "respondInputError(c, err, errcode.Internal)") {
			t.Errorf("%s 必须走 respondInputError（有输入原因 → 400 人话，其余才 500）", fn)
		}
	}
}

// stripGoComments 剥掉整行 // 注释（我自己的补丁注释里就写着 errcode.Internal，不剥会造假红）。
func stripGoComments(s string) string {
	out := make([]string, 0, 64)
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}
