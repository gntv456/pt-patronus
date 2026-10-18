package v1

// #354 文件管理的「不能动存储根目录」9 处出口全回一句「不允许的操作」。
//
// filemanager.go / filemanager_extra.go 里 9 处 `if isRootPath(…) { Respond(OperationForbidden) }`
// 覆盖的是写/删/改名/移动/复制五类动作，但用户看到的永远是同一句「不允许的操作」：
// 既不知道被拒的是哪个动作，也不知道为什么——实际上原因很明确也很友好：
// 这是文件浏览器的**根**，操作它没有意义（也不安全）。
//
// 第 9 处 media.go 的 OperationForbidden 是另一件事（图片代理的来源校验），
// 共用同一句文案就更是把两件无关的事混成一句。
//
// 修法：根目录这一类给专属码 RootPathProtected（说清「根目录」+ 下一步「请选择根下的具体文件或目录」），
// media 那处换成本域专属码。裸 Respond 在这里是对的——isRootPath 是自产判断，没有 err 可带。

import (
	"os"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/api/errcode"
)

// TestRootPathGuardHasOwnCode 根目录守卫要有专属码，而不是借用 OperationForbidden。
func TestRootPathGuardHasOwnCode(t *testing.T) {
	for _, f := range []string{"filemanager.go", "filemanager_extra.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("读不到 %s：%v", f, err)
		}
		code := stripCommentsForTest(string(src))
		n := strings.Count(code, "errcode.OperationForbidden")
		if n > 0 {
			t.Errorf("%s 还有 %d 处根目录守卫在用通用码 OperationForbidden（「不允许的操作」说不出被拒的是什么）", f, n)
		}
		if !strings.Contains(code, "RootPathProtected") {
			t.Errorf("%s 的根目录守卫必须用专属码 RootPathProtected", f)
		}
	}
}

// TestRootPathProtectedTextIsActionable 专属码要说清「根目录」并给出下一步。
func TestRootPathProtectedTextIsActionable(t *testing.T) {
	msg := errcode.RootPathProtected.Message
	if !strings.Contains(msg, "根") {
		t.Errorf("文案要说清是「根目录」（那才是真正的原因），got %q", msg)
	}
	if !strings.Contains(msg, "请") && !strings.Contains(msg, "可") {
		t.Errorf("文案要给出下一步，got %q", msg)
	}
	// 专属码必须与通用码文案不同，否则换码等于没换。
	if msg == errcode.OperationForbidden.Message {
		t.Errorf("RootPathProtected 与 OperationForbidden 文案相同：%q", msg)
	}
}
