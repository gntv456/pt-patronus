package automation

// #365 VIP 复查里「cookie 解不开」被当成「验证不通过」静默跳过，用户因此被锁死且无人知情。
//
// runVIPRecheck 的循环体：
//
//	cookie, err := s.crypto.Decrypt(u.HxptCookieEnc)
//	if err != nil || cookie == "" {
//		continue          // ← 这里
//	}
//
// 同一个函数里，上面 8 行的读列表失败专门加了通知，注释写得很清楚：
//
//	// 静默 = VIP 宽限复查整轮漏跑，用户可能在窗口外被锁死，读失败必须出声。
//
// 但下面这处静默没跟上。后果链：
//
//	解不开 cookie（主密钥换过 / 密文损坏 / 算法升级）
//	  → continue，这轮不刷新 VipVerifiedAt
//	  → auth/guard.go:60 time.Since(VipVerifiedAt) > grace ⇒ VIPExpired 403
//	  → 用户被锁在门外，而管理员在界面上看不到任何提示
//
// 这与「好学不可达 / cookie 过期 / 非 VIP」是**完全不同**的三种情况：
// 后两者该自然过期（用户自己会重新登录），前者是**本地读不到数据**，
// 是该由管理员处理的事故（重新保存 cookie），不该混进「不更新」里悄悄消化。
//
// 判据用真 Decrypt 失败驱动（换过主密钥的密文），并断言通知确实发出去。

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/auth"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/notification"
	"gorm.io/gorm"
)

// vipNotifySpy 记录发出去的通知。
type vipNotifySpy struct{ msgs []notification.Message }

func (n *vipNotifySpy) Notify(_ context.Context, msg notification.Message) []notification.SendResult {
	n.msgs = append(n.msgs, msg)
	return nil
}

// vipAuthStub 好学验证器：嵌入接口只覆写 Verify（其余方法 nil panic 即可，本测试不该碰到）。
// 解不开 cookie 时本就不该发请求，真被调也算错。
type vipAuthStub struct {
	auth.HxptAuthenticator
	calls int
}

func (a *vipAuthStub) Verify(context.Context, string) *auth.VIPStatus {
	a.calls++
	return &auth.VIPStatus{Verified: false}
}

func newVIPRecheckService(t *testing.T, cookieEnc string, spy *vipNotifySpy, auth *vipAuthStub) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/vip.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.User{}); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-30 * 24 * time.Hour) // 早于复查阈值，确保会被扫到
	if err := db.Create(&models.User{
		Username: "u1", HxptCookieEnc: cookieEnc, VipVerifiedAt: &old,
	}).Error; err != nil {
		t.Fatal(err)
	}
	return &Service{
		db:      db,
		cfg:     Config{VIPGraceDays: 7},
		rootCtx: context.Background(),
		// 用另一把密钥 ⇒ Decrypt 必然失败（等价于生产里主密钥换过）。
		crypto:   security.NewCrypto("rotated-key"),
		hxptAuth: auth,
		notif:    spy,
	}
}

// TestVIPRecheckNotifiesWhenCookieUndecryptable cookie 解不开必须出声，不能静默 continue。
func TestVIPRecheckNotifiesWhenCookieUndecryptable(t *testing.T) {
	spy := &vipNotifySpy{}
	auth := &vipAuthStub{}
	// 用别的密钥产出的密文（或损坏串）当用户 cookie ⇒ 解不开。
	s := newVIPRecheckService(t, "garbage-or-other-key-ciphertext", spy, auth)

	s.runVIPRecheck()

	if len(spy.msgs) == 0 {
		t.Fatalf("cookie 解不开却一条通知都没发：用户会被 guard.go 的 VIPExpired 锁在门外而无人知情")
	}
	joined := ""
	for _, m := range spy.msgs {
		joined += m.Title + " " + m.Body
	}
	// 通知必须说清是「读不出凭据」，而不是泛泛的复查失败——管理员据此知道要重新保存 cookie。
	for _, want := range []string{"凭据", "解"} {
		if !strings.Contains(joined, want) {
			t.Errorf("通知要点明「凭据解不开」（管理员据此去重新保存 cookie），实际 %q", joined)
		}
	}
	// 解不开就不该去请求好学：没有 cookie 可拿。
	if auth.calls != 0 {
		t.Errorf("解不开 cookie 时不该调 Verify（没有 cookie 可用），实际调用 %d 次", auth.calls)
	}
}

// TestVIPRecheckSilentWhenEverythingHealthy 正控：解得开时不许发通知（否则会被噪声淹没）。
func TestVIPRecheckSilentWhenEverythingHealthy(t *testing.T) {
	spy := &vipNotifySpy{}
	auth := &vipAuthStub{}
	// 用同一把密钥加密，解得开。
	enc := security.NewCrypto("rotated-key")
	cipher, err := enc.Encrypt("uid=7; pass=secret")
	if err != nil {
		t.Fatalf("造密文失败：%v", err)
	}
	s := newVIPRecheckService(t, cipher, spy, auth)

	s.runVIPRecheck()

	if len(spy.msgs) != 0 {
		t.Errorf("一切正常时不该发通知（否则真事故会被噪声淹没）：%v", spy.msgs)
	}
}
