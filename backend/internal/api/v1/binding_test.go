package v1

import (
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/hxpt/ptpatronus/internal/media"
	"github.com/hxpt/ptpatronus/internal/update"
)

// newBindValidator 镜像 gin 的 binding 校验器：tag 名 = "binding"。
func newBindValidator(t *testing.T) *validator.Validate {
	t.Helper()
	v := validator.New()
	v.SetTagName("binding")
	return v
}

func mustBind(t *testing.T, v *validator.Validate, s any, wantErr bool) {
	t.Helper()
	err := v.Struct(s)
	if (err != nil) != wantErr {
		t.Errorf("Struct(%+v) err=%v, wantErr=%v", s, err, wantErr)
	}
}

// TestSubReqBindingMax 验证 API 层 subReq 的 binding max 生效（代表性，其余 rssReq/webhookReq/wfReq 同模式）。
func TestSubReqBindingMax(t *testing.T) {
	v := newBindValidator(t)
	if err := v.Struct(subReq{Source: "tmdb", SourceID: "123", Title: "正常标题", Type: "movie"}); err != nil {
		t.Fatalf("normal subReq should pass: %v", err)
	}
	cases := []struct {
		name string
		in   subReq
	}{
		{"Title>255", subReq{Title: strings.Repeat("a", 256)}},
		{"Source>16", subReq{Source: strings.Repeat("a", 17)}},
		{"PosterURL>512", subReq{PosterURL: strings.Repeat("a", 513)}},
	}
	for _, tc := range cases {
		if err := v.Struct(tc.in); err == nil {
			t.Errorf("%s: expected validation error, got nil", tc.name)
		}
	}
}

// TestRssReqBindingMax rssReq 每个带 max= 的字段越界必拒。
func TestRssReqBindingMax(t *testing.T) {
	v := newBindValidator(t)
	mustBind(t, v, rssReq{Name: "n", URL: "http://x"}, false) // 正常通过
	cases := []struct {
		name string
		in   rssReq
	}{
		{"Name>128", rssReq{Name: strings.Repeat("a", 129)}},
		{"URL>512", rssReq{URL: strings.Repeat("a", 513)}},
		{"TitleInclude>2000", rssReq{TitleInclude: strings.Repeat("a", 2001)}},
		{"TitleExclude>2000", rssReq{TitleExclude: strings.Repeat("a", 2001)}},
		{"Resolution>32", rssReq{Resolution: strings.Repeat("a", 33)}},
		{"SavePath>255", rssReq{SavePath: strings.Repeat("a", 256)}},
		{"Category>64", rssReq{Category: strings.Repeat("a", 65)}},
		{"Tags>500", rssReq{Tags: strings.Repeat("a", 501)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mustBind(t, v, tc.in, true)
		})
	}
}

// TestWfReqBindingMax wfReq 字段越界必拒。
func TestWfReqBindingMax(t *testing.T) {
	v := newBindValidator(t)
	mustBind(t, v, wfReq{Name: "wf", TriggerType: "manual"}, false)
	cases := []struct {
		name string
		in   wfReq
	}{
		{"Name>128", wfReq{Name: strings.Repeat("a", 129)}},
		{"Description>2000", wfReq{Description: strings.Repeat("a", 2001)}},
		{"TriggerType>16", wfReq{TriggerType: strings.Repeat("a", 17)}},
		{"Timer>64", wfReq{Timer: strings.Repeat("a", 65)}},
		{"EventType>64", wfReq{EventType: strings.Repeat("a", 65)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mustBind(t, v, tc.in, true)
		})
	}
}

// TestWebhookReqBindingMax webhookReq Name/Secret 越界必拒。
func TestWebhookReqBindingMax(t *testing.T) {
	v := newBindValidator(t)
	mustBind(t, v, webhookReq{Name: "hk"}, false)
	cases := []struct {
		name string
		in   webhookReq
	}{
		{"Name>128", webhookReq{Name: strings.Repeat("a", 129)}},
		{"Secret>256", webhookReq{Secret: strings.Repeat("a", 257)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mustBind(t, v, tc.in, true)
		})
	}
}

// TestRequiredBindings required 标注的结构体：必填字段空必拒、有值通过。
func TestRequiredBindings(t *testing.T) {
	v := newBindValidator(t)
	cases := []struct {
		name  string
		valid any // 必填字段已赋值
		empty any // 必填字段为零值
	}{
		{"addReq.URL", addReq{URL: "http://x"}, addReq{}},
		{"activateReq.Code", activateReq{Code: "abc"}, activateReq{}},
		{"composeReq.MedalID", composeReq{MedalID: "m1"}, composeReq{}},
		{"playReq.Game", playReq{Game: "lucky"}, playReq{}},
		{"findReq.Name", findReq{Name: "foo"}, findReq{}},
		{"resolveReq.Action", resolveReq{Action: "add"}, resolveReq{}},
		{"notifyReq.Title", notifyReq{Title: "t"}, notifyReq{}},
		{"inspectReq.URL", inspectReq{URL: "http://x"}, inspectReq{}},
		{"cookieReq.Cookie", cookieReq{Cookie: "c=1"}, cookieReq{}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name+"/valid", func(t *testing.T) {
			t.Parallel()
			mustBind(t, v, tc.valid, false)
		})
		t.Run(tc.name+"/empty", func(t *testing.T) {
			t.Parallel()
			mustBind(t, v, tc.empty, true)
		})
	}
}

// TestMaxHardenedBounds 验证本会话新补的 max=/dive,max= 硬化生效——超长串/超大元素必拒，
// 合法长度通过。这是审计「缺 max=」缺口的回归保护。
func TestMaxHardenedBounds(t *testing.T) {
	v := newBindValidator(t)

	t.Run("mediaConfigReq secrets bounded", func(t *testing.T) {
		mustBind(t, v, mediaConfigReq{TMDBAPIKey: "k", BangumiToken: "t"}, false)
		mustBind(t, v, mediaConfigReq{TMDBAPIKey: strings.Repeat("k", 129)}, true)
		mustBind(t, v, mediaConfigReq{TMDBAPIDomain: strings.Repeat("d", 256)}, true)
		mustBind(t, v, mediaConfigReq{DoubanAppKey: strings.Repeat("a", 65)}, true)
	})

	t.Run("hashesReq dive caps element length", func(t *testing.T) {
		mustBind(t, v, hashesReq{Hashes: []string{"abcdef"}}, false)            // 40-hex 合法
		mustBind(t, v, hashesReq{Hashes: []string{strings.Repeat("a", 65)}}, true) // 单元素 >64 拒
	})

	t.Run("commandRunRequest bounded", func(t *testing.T) {
		mustBind(t, v, commandRunRequest{Command: "ls", Source: "manual"}, false)
		mustBind(t, v, commandRunRequest{Command: strings.Repeat("a", 2001)}, true)
		mustBind(t, v, commandRunRequest{Source: strings.Repeat("s", 33)}, true)
	})

	t.Run("configReq/settingReq bounded", func(t *testing.T) {
		mustBind(t, v, configReq{IYUUToken: "tok"}, false)
		mustBind(t, v, configReq{IYUUToken: strings.Repeat("t", 257)}, true)
		mustBind(t, v, settingReq{DownloaderID: 1, SavePath: "/x"}, false)
		mustBind(t, v, settingReq{DownloaderID: 1, SavePath: strings.Repeat("x", 513)}, true)
	})

	t.Run("hxptLoginReq bounded", func(t *testing.T) {
		mustBind(t, v, hxptLoginReq{Username: "u", Password: "p"}, false)
		mustBind(t, v, hxptLoginReq{Username: strings.Repeat("u", 65)}, true)
		mustBind(t, v, hxptLoginReq{Password: strings.Repeat("p", 129)}, true)
	})

	t.Run("plugin slices dive,max", func(t *testing.T) {
		mustBind(t, v, pluginMarketSourcesReq{Sources: []string{"https://x"}}, false)
		mustBind(t, v, pluginMarketSourcesReq{Sources: []string{strings.Repeat("u", 257)}}, true)
		mustBind(t, v, pluginTrustConfigReq{TrustedPublicKeys: []string{"pk"}}, false)
		mustBind(t, v, pluginActionReq{Action: "run"}, false)
		mustBind(t, v, pluginActionReq{Action: strings.Repeat("a", 65)}, true)
	})
}

// TestMaxHardenedBoundsRound2 验证第二轮「缺 max=」补全（2026-07-12）的硬化生效：
// entertainment/medalwall/message(匿名struct同构)/monitor/auth/notification/keepseed/
// plugin/ops 的新增 string max、[]string/map dive、数字 min/max 均越界必拒、合法通过。
// message.go 的 Reply/Delete/MarkRead 为内联匿名 struct，由各自 handler 的 ShouldBindJSON
// 反射生效（与命名 struct 同构），这里覆盖命名 struct。
func TestMaxHardenedBoundsRound2(t *testing.T) {
	v := newBindValidator(t)

	t.Run("notifyReq body/level bounded", func(t *testing.T) {
		mustBind(t, v, notifyReq{Title: "t", Body: "b", Level: "info"}, false)
		mustBind(t, v, notifyReq{Title: "t", Body: strings.Repeat("b", 2001)}, true)
		mustBind(t, v, notifyReq{Title: "t", Level: strings.Repeat("L", 17)}, true)
	})

	t.Run("activateReq/verifyVIPReq bounded", func(t *testing.T) {
		mustBind(t, v, activateReq{Code: "c", DeviceID: "d"}, false)
		mustBind(t, v, activateReq{Code: strings.Repeat("c", 129)}, true)
		mustBind(t, v, activateReq{Code: "c", DeviceID: strings.Repeat("d", 65)}, true)
		mustBind(t, v, verifyVIPReq{Cookie: "sid=1"}, false)
		mustBind(t, v, verifyVIPReq{Cookie: strings.Repeat("c", 8193)}, true)
	})

	t.Run("site inspect/cookie bounded", func(t *testing.T) {
		mustBind(t, v, inspectReq{URL: "http://x"}, false)
		mustBind(t, v, inspectReq{URL: strings.Repeat("u", 513)}, true)
		mustBind(t, v, inspectReq{URL: "http://x", Cookie: strings.Repeat("c", 65537)}, true)
		mustBind(t, v, cookieReq{Cookie: "c=1"}, false)
		mustBind(t, v, cookieReq{Cookie: strings.Repeat("c", 65537)}, true)
	})

	t.Run("medalActionBody bounded", func(t *testing.T) {
		mustBind(t, v, medalActionBody{SiteID: 1, MedalID: "m"}, false)
		mustBind(t, v, medalActionBody{SiteID: 0, MedalID: "m"}, true) // min=1
		mustBind(t, v, medalActionBody{SiteID: 1, MedalID: strings.Repeat("m", 65)}, true)
	})

	t.Run("silenceAlertRequest bounded", func(t *testing.T) {
		mustBind(t, v, silenceAlertRequest{Minutes: 10, Until: ""}, false)
		mustBind(t, v, silenceAlertRequest{Minutes: 0}, true)                     // min=1
		mustBind(t, v, silenceAlertRequest{Minutes: 10081}, true)                 // max=10080
		mustBind(t, v, silenceAlertRequest{Until: strings.Repeat("u", 65)}, true) // max=64
	})

	t.Run("findReq/reseedReq bounded", func(t *testing.T) {
		mustBind(t, v, findReq{Name: "n", Size: 1 << 30}, false)
		mustBind(t, v, findReq{Name: "n", Size: 1099511627777}, true) // >1TB
		mustBind(t, v, reseedReq{DownloaderID: 1}, false)
		mustBind(t, v, reseedReq{DownloaderID: 0}, true) // required+min=1
	})

	t.Run("entertainment game structs bounded", func(t *testing.T) {
		mustBind(t, v, playReq{Game: "lucky"}, false)
		mustBind(t, v, playReq{Game: strings.Repeat("g", 65)}, true)
		mustBind(t, v, composeReq{MedalID: "m"}, false)
		mustBind(t, v, composeReq{MedalID: strings.Repeat("m", 65)}, true)
		mustBind(t, v, farmItemReq{Section: "crop", ItemID: 1}, false)
		mustBind(t, v, farmItemReq{Section: strings.Repeat("s", 17)}, true)
		mustBind(t, v, farmItemReq{Section: "crop", ItemID: 100001}, true)
		mustBind(t, v, sellReq{Key: "k"}, false)
		mustBind(t, v, sellReq{Key: strings.Repeat("k", 65)}, true)
		mustBind(t, v, blackjackReq{Action: "hit"}, false)
		mustBind(t, v, blackjackReq{Action: strings.Repeat("a", 17)}, true)
		mustBind(t, v, bsSendChatReq{Message: "hi"}, false)
		mustBind(t, v, bsSendChatReq{Message: strings.Repeat("m", 2001)}, true)
		mustBind(t, v, bsItemReq{Tag: "t"}, false)
		mustBind(t, v, bsItemReq{Tag: strings.Repeat("t", 65)}, true)
	})

	t.Run("betReq selection+extra dive bounded", func(t *testing.T) {
		mustBind(t, v, betReq{Selection: "big", Extra: map[string]string{"a": "b"}}, false)
		mustBind(t, v, betReq{Selection: strings.Repeat("s", 65)}, true)
		mustBind(t, v, betReq{Selection: "big", Extra: map[string]string{"k": strings.Repeat("v", 513)}}, true)
	})

	t.Run("bsRedpacket action+extra dive bounded", func(t *testing.T) {
		mustBind(t, v, bsRedpacketReq{Action: "send"}, false)
		mustBind(t, v, bsRedpacketReq{Action: strings.Repeat("a", 17)}, true)
		mustBind(t, v, bsRedpacketReq{Action: "send", Extra: map[string]string{"k": strings.Repeat("v", 513)}}, true)
	})

	t.Run("entertainment numeric ranges", func(t *testing.T) {
		mustBind(t, v, buyReq{OptionType: 5}, false)
		mustBind(t, v, buyReq{OptionType: 101}, true)
		mustBind(t, v, playMultiReq{Count: 10}, false)
		mustBind(t, v, playMultiReq{Count: 0}, true)  // min=1
		mustBind(t, v, playMultiReq{Count: 101}, true)
		mustBind(t, v, drawReq{SynthesisID: 1, Count: 10}, false)
		mustBind(t, v, drawReq{SynthesisID: 0, Count: 1}, true) // SynthesisID min=1
		mustBind(t, v, drawReq{SynthesisID: 1, Count: 101}, true)
		mustBind(t, v, synthReq{SynthesisID: 1}, false)
		mustBind(t, v, synthReq{SynthesisID: 0}, true)
		mustBind(t, v, convertReq{YiYang: 1}, false)
		mustBind(t, v, convertReq{YiYang: 100001}, true)
		mustBind(t, v, convertReq{YiYang: -100001}, true)
		mustBind(t, v, bsTaskReq{Action: "claim", TaskID: 1}, false)
		mustBind(t, v, bsTaskReq{Action: "claim", TaskID: 0}, true) // TaskID min=1
	})

	t.Run("plugin permission/rollback/dive bounded", func(t *testing.T) {
		mustBind(t, v, pluginPermissionReq{AcceptedPermissions: []string{"p"}}, false)
		mustBind(t, v, pluginPermissionReq{AcceptedPermissions: []string{strings.Repeat("p", 65)}}, true)
		mustBind(t, v, pluginInstallPermissionAccept{ID: "x", AcceptedPermissions: []string{"p"}}, false)
		mustBind(t, v, pluginInstallPermissionAccept{ID: strings.Repeat("x", 129)}, true)
		mustBind(t, v, pluginInstallPermissionAccept{ID: "x", AcceptedPermissions: []string{strings.Repeat("p", 65)}}, true)
		mustBind(t, v, pluginRollbackReq{ReleaseID: 1}, false)
		mustBind(t, v, pluginRollbackReq{ReleaseID: 0}, true) // min=1
	})

	t.Run("commandRunRequest target list + timeout bounded", func(t *testing.T) {
		mustBind(t, v, commandRunRequest{Command: "ls", TargetIDs: make([]uint, 200)}, false) // DoS 上限内（业务上限 50 由 handler 友好处理）
		mustBind(t, v, commandRunRequest{Command: "ls", TargetIDs: make([]uint, 201)}, true) // >200 DoS 拒
		mustBind(t, v, commandRunRequest{Command: "ls", TimeoutSec: 3600}, false)
		mustBind(t, v, commandRunRequest{Command: "ls", TimeoutSec: 3601}, true)
		mustBind(t, v, commandRunRequest{Command: "ls", TimeoutSec: -1}, true)                    // min=0
	})

	t.Run("hxptLoginReq.ExtraForm dive bounded (公开登录端点)", func(t *testing.T) {
		mustBind(t, v, hxptLoginReq{Username: "u", Password: "p", ExtraForm: map[string]string{"a": "b"}}, false)
		mustBind(t, v, hxptLoginReq{Username: "u", Password: "p", ExtraForm: map[string]string{"k": strings.Repeat("v", 513)}}, true)
	})

	t.Run("update.Manifest bounded (SetManifest admin)", func(t *testing.T) {
		mustBind(t, v, update.Manifest{LatestVersion: "1.0", Targets: map[string]update.Target{"docker": {}}}, false)
		mustBind(t, v, update.Manifest{LatestVersion: strings.Repeat("v", 33)}, true)
		mustBind(t, v, update.Manifest{ReleaseNotes: strings.Repeat("n", 10001)}, true)
		mustBind(t, v, update.Manifest{Targets: map[string]update.Target{"x": {URL: strings.Repeat("u", 513)}}}, true) // dive 进 Target.URL
	})

	t.Run("media.MediaInfo bounded (nfoReq, 非 admin 可达)", func(t *testing.T) {
		mustBind(t, v, media.MediaInfo{Source: "tmdb", SourceID: "1", Title: "t"}, false)
		mustBind(t, v, media.MediaInfo{Title: strings.Repeat("t", 513)}, true)
		mustBind(t, v, media.MediaInfo{Overview: strings.Repeat("o", 10001)}, true)
		mustBind(t, v, media.MediaInfo{PosterURL: strings.Repeat("u", 513)}, true)
		mustBind(t, v, media.MediaInfo{Genres: []string{strings.Repeat("g", 65)}}, true)                    // dive,max=64
		mustBind(t, v, media.MediaInfo{Cast: []media.CreditPerson{{Name: strings.Repeat("n", 129)}}}, true) // dive into Cast[].Name
	})
}
