package chain

import (
	"os"
	"strings"
	"testing"
)

// #324 手动转存的「种子不在该下载器」出口从前是英文加一个裸编号
// （`torrent <hash> not found in downloader 2`），而它经 handler 的 userErrText 直接弹成 toast。

func TestManualTransferNotFoundReasonIsChinese(t *testing.T) {
	raw, err := os.ReadFile("transfer.go")
	if err != nil {
		t.Fatalf("读不到 transfer.go：%v", err)
	}
	code := stripCodeComments(string(raw))
	from := strings.Index(code, "func (c *TransferChain) resolveAndProcessResult")
	if from < 0 {
		t.Fatalf("找不到 resolveAndProcessResult（判据没有落点）")
	}
	tail := code[from:]
	if n := strings.Index(tail[1:], "\nfunc "); n > 0 {
		tail = tail[:1+n]
	}
	if strings.Contains(tail, `fmt.Errorf("torrent `) {
		t.Errorf("手动转存的「种子不在下载器」不许再回英文原文：%q", tailLinesWith(tail, "fmt.Errorf"))
	}
	if !strings.Contains(tail, "下载器（编号") || !strings.Contains(tail, "请在「下载器」页") {
		t.Errorf("要给出可操作的中文（哪台下载器、下一步去哪看）：%q", tailLinesWith(tail, "fmt.Errorf"))
	}
}

func stripCodeComments(s string) string {
	out := make([]string, 0, 128)
	for _, l := range strings.Split(s, "\n") {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "//") {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

func tailLinesWith(s, key string) string {
	var hit []string
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, key) {
			hit = append(hit, strings.TrimSpace(l))
		}
	}
	return strings.Join(hit, " | ")
}
