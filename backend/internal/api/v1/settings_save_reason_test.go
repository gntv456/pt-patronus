package v1

// #353 OIDC 与 Trakt 的配置保存出口是裸 SaveFailed（连 err 都没带），文案又是「保存失败」。
//
// 同一个 SaveFailed 码有三种用法，语义天差地别：
//
//	respondRecordError(…, SaveFailed)  fallback 档，会 WithErr(err) 带原因 —— 文案短不影响
//	SaveFailed.WithErr(err)            带原因（ocr.go 三处）—— 同上
//	errcode.Respond(c, errcode.SaveFailed)   ← 裸的，err 整个丢掉（本文件这两处）
//
// 裸的两处是配置保存：auth_oidc.go 的 oidc.SaveSettings、trakt.go 的设置写入，
// 底层都是系统配置表写库（store.Set）。失败原因是具体的（主密钥换了 / 配置项
// 写不进 / JSON 编不过），却被压成「保存失败」三个字。
//
// 修法：改走 WithErr，带上原因；文案补一句能指路的（去检查哪类原因）。

import (
	"os"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/api/errcode"
)

// TestSettingsSaveOutletsCarryReason 配置保存出口必须带上原因。
func TestSettingsSaveOutletsCarryReason(t *testing.T) {
	for _, f := range []string{"auth_oidc.go", "trakt.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("读不到 %s：%v", f, err)
		}
		code := stripCommentsForTest(string(src))
		if n := strings.Count(code, "errcode.Respond(c, errcode.SaveFailed)"); n != 0 {
			t.Errorf("%s 还有 %d 处裸 Respond(SaveFailed)：底层原因被丢掉，界面只剩「保存失败」", f, n)
		}
	}
}

// TestSaveFailedTextIsNotBare 文案本身也要能指路：光「保存失败」三个字，用户不知道该查哪。
func TestSaveFailedTextIsNotBare(t *testing.T) {
	msg := errcode.SaveFailed.Message
	if len([]rune(msg)) <= 5 {
		t.Errorf("SaveFailed 文案太短（%q），用户无从判断该查主密钥、配置格式还是数据库", msg)
	}
	if !strings.Contains(msg, "请") && !strings.Contains(msg, "可") {
		t.Errorf("SaveFailed 文案没给下一步，got %q", msg)
	}
}
