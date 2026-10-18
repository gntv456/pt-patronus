package chain

import (
	"os"
	"strings"
	"testing"
)

// 接线护栏：ratingGate 的跳过说明必须真的进本轮账面（卡片）并发一条去重通知。
// Check 的执行级用例要真站点搜索，替身层太薄证不到「卡片上有这句」，所以钉源码形状：
// gateSkip 必须流向 checkErr，而 checkErr 的读者就是 MarkCheckedWithResult。
func TestSubscribeCheckSurfacesRatingGateSkip(t *testing.T) {
	b, err := os.ReadFile("subscribe_check.go") // go test 的 CWD 即本包目录
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	for _, line := range strings.Split(string(b), "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		kept = append(kept, line)
	}
	src := strings.Join(kept, "\n")
	for _, want := range []string{"gateSkip", `"rating-guard-skipped"`, "checkErr += \"；\" + siteNote"} {
		if !strings.Contains(src, want) {
			t.Errorf("subscribe_check.go 里找不到 %s：跳过说明或站点失败并列没接上", want)
		}
	}
	if strings.Contains(src, "checkErr = fmt.Sprintf(\"%d/%d") {
		t.Errorf("站点失败那句又变成直接赋值：会顶掉闸门跳过的说明")
	}
}
