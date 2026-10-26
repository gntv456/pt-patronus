package v1

// 护栏：自产的中文原因必须有「人工载体」（WithDetail），不许只交给 WithErr。
// WithErr 只把原因写进日志，界面拿到的是错误码的固定文案 —— 于是「动作 join
// 缺少必要参数」在用户眼里就是一句「请求参数有误」，既不知道哪个动作、也不知道
// 补什么（#69/#310 同一族的第 N 次复发，这里钉死这一种形状）。
// 正控：把 entertainment_gobang.go 那一处退回 WithErr，本用例必须红。

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

var (
	// 任意码 + WithErr(fmt.Errorf("…中文…"))：只可能出现在 handler 自己造原因的地方。
	selfAuthoredCJK = regexp.MustCompile(
		`errcode\.\w+\.WithErr\(\s*fmt\.Errorf\(\s*"([^"]*[\p{Han}][^"]*)"`)
	commentLineRE = regexp.MustCompile(`^\s*(//|\*)`)
)

func TestSelfAuthoredChineseReasonsNeedACarrier(t *testing.T) {
	// go test 的工作目录就是本包目录（api/v1），不必猜绝对路径。
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("打不开本包目录：%v", err)
	}
	hits, checked := 0, 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		checked++
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("读不了 %s：%v", name, err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			m := selfAuthoredCJK.FindStringSubmatch(line)
			if m == nil || commentLineRE.MatchString(line) {
				continue
			}
			hits++
			t.Errorf("%s 把自产中文原因「%s」只交给 WithErr，界面只剩固定码文案",
				name, m[1])
		}
	}
	if checked < 40 {
		t.Fatalf("基数可疑：只扫到 %d 个源文件，多半没在 api/v1 目录里跑", checked)
	}
}
