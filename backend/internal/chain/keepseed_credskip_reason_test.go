// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

// #384 辅种候选筛选把「没配凭据」与「凭据解不开」压成同一个静默 continue。
//
// keepseed_reseed.go:88-91 的形状：
//
//	candURL, ok := c.buildCandidateURL(local, iyuuSite, t.TorrentID)
//	if !ok {
//	    continue // 无可用凭据（MTeam 缺 api_key / 其它站缺 passkey）
//	}
//
// 而 buildCandidateURL 内部（keepseed_resolve.go）：
//
//	if local.Platform == "mteam" {
//	    apiKey, _ := c.siteOp.APIKey(local)      // ← 错误丢掉
//	    if apiKey == "" { return "", false }
//	    ...
//	}
//	passkey := c.siteOp.DecryptedPasskey(local)   // ← 同形状
//	if passkey == "" { return "", false }
//
// 两个「== ""」含义完全不同：
//   - 本来就没填     → 用户该去站点设置里补凭据；
//   - 填了但解不开   → 主密钥换过/密文损坏，用户该去**重存**凭据（或联系运维换回密钥）。
// 而 DecryptedPasskey 与 APIKey 都是 (string, error) 形态的 oper 层函数在吞错
// （#383 记的就是同一族：db/oper 的单值包装把 error 丢掉）。
//
// 后果：辅种匹配一轮跑完，用户看到的是「没有匹配到可辅种的种子」「0 个候选」——
// 一个听起来像「这批种子没有同种」的中性结论，没有任何地方说「本地读不出你的凭据」。
// 这与 #377/#383 治的病同形，只是出口在**筛选路径**上（不是请求路径），更不容易被发现。
//
// 与 #369 的关系：#369 已经为「MTeam 未配 api_key」立了 ErrMTeamNoAPIKey 哨兵，接口层
// 用 WithDetail 放行原话；但 buildCandidateURL 这条筛选路径压根不产生错误，只产生 bool，
// 哨兵到不了这里。修法就是把它改成能区分两种「空」。
import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/iyuu"
)

// rotatedSeedSite 造一个「用 A 加密、用 B 解密」且填了凭据的站点。
func rotatedSeedSite(t *testing.T, platform string) (*KeepSeedChain, *models.Site) {
	t.Helper()
	db := newReseedDB(t)
	w := oper.NewSiteOper(db, security.NewCrypto("old-key"))
	in := oper.SiteInput{Name: "候选站", Platform: platform, BaseURL: "https://cand.test", Passkey: "REAL-PASSKEY", APIKey: "REAL-APIKEY"}
	if _, err := w.Create(in); err != nil {
		t.Fatal(err)
	}
	rot := oper.NewSiteOper(db, security.NewCrypto("rotated-key"))
	s, err := rot.Get(1)
	if err != nil {
		t.Fatal(err)
	}
	return &KeepSeedChain{siteOp: rot}, s
}

// TestBuildCandidateURLDistinguishesUnreadable 「填了但解不开」必须与「没填」分开。
func TestBuildCandidateURLDistinguishesUnreadable(t *testing.T) {
	for _, platform := range []string{"nexusphp", "mteam"} {
		t.Run(platform, func(t *testing.T) {
			c, s := rotatedSeedSite(t, platform)
			url, ok, err := c.buildCandidateURL(s, iyuu.SiteInfo{BaseURL: "https://cand.test"}, 42)
			if err == nil {
				t.Fatalf("%s：凭据填了却解不开，buildCandidateURL 却返回 nil 错误 + (ok=%v, %q) —— "+
					"这会让上层把它当「没配凭据」静默 continue，用户看到的是「没匹配到同种」", platform, ok, url)
			}
			if !errors.Is(err, ErrSiteCredentialUnreadable) {
				t.Errorf("%s：该是可判别的 ErrSiteCredentialUnreadable（与 #377/#383 同一哨兵），got %v", platform, err)
			}
			if !strings.Contains(err.Error(), s.Name) {
				t.Errorf("%s：原因里必须带出站点名，got %q", platform, err.Error())
			}
		})
	}
}

// TestBuildCandidateURLAbsentIsNotFailure 反向对照：压根没填凭据是正常跳过，不许报错。
// 这条最要紧 —— 误判会让「用户还没配置」的普通情况整片报故障。
func TestBuildCandidateURLAbsentIsNotFailure(t *testing.T) {
	db := newReseedDB(t)
	o := oper.NewSiteOper(db, security.NewCrypto("same-key"))
	s, err := o.Create(oper.SiteInput{Name: "没配凭据的站", Platform: "nexusphp", BaseURL: "https://cand.test"})
	if err != nil {
		t.Fatal(err)
	}
	c := &KeepSeedChain{siteOp: o}
	url, ok, err := c.buildCandidateURL(s, iyuu.SiteInfo{BaseURL: "https://cand.test"}, 42)
	if err != nil {
		t.Fatalf("没配凭据是正常状态（跳过该候选），不该报错: %v", err)
	}
	if ok || url != "" {
		t.Errorf("没配凭据时应当是「跳过」而不是产出 URL，got ok=%v url=%q", ok, url)
	}
}

// TestReseedSurfacesUnreadableCredentials 接线护栏：buildCandidateURL 交出来的 error
// 必须一路到达 Reseed 的返回值，不许只 collect 不报（那是本仓反复出现的
// 「新加的失败出口没找它的读者」——判据盯着 helper 绿，界面照样什么都没说）。
func TestReseedSurfacesUnreadableCredentials(t *testing.T) {
	src, err := os.ReadFile("keepseed_reseed.go")
	if err != nil {
		t.Fatalf("读不到被测源文件: %v", err)
	}
	code := stripKeepseedComments(string(src))

	// 调用点必须接住第三个返回值。
	if !strings.Contains(code, "candURL, ok, credErr := c.buildCandidateURL(") {
		t.Error("reseed 调用点没有接 buildCandidateURL 的 error 返回值（旧形状：只有 ok）")
	}
	// 且必须把凭据读不出的站名汇总出去。
	if !strings.Contains(code, "unreadableSites") {
		t.Error("reseed 没有收集读不出凭据的站点（旧形状：静默 continue）")
	}
	// 汇总必须在返回前报出来，否则收集等于没用。
	if !strings.Contains(code, "return created, fmt.Errorf(") {
		t.Error("读不出凭据时 Reseed 仍然返回 nil error —— 用户只看到「没匹配到同种」")
	}
	// 反向断言：旧形状那句裸 continue 必须消失。
	if strings.Contains(code, "continue // 无可用凭据") {
		t.Error("还在用 `continue // 无可用凭据` —— 那是把「没配」与「解不开」吞成同形的旧写法")
	}
}

// stripKeepseedComments 去掉整行注释，免得判据匹配到自己写的旧形状说明。
func stripKeepseedComments(s string) string {
	out := make([]string, 0, 256)
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

// TestReseedReportIsReachable 接线护栏（顺序版）：#384 的汇报块曾经被放在
// `return created, nil` **之后** —— 语法合法、编译通过、连 go vet 都只在后来才报 unreachable，
// 而当时的接线判据只查「源码里有没有 return created, fmt.Errorf(」照样全绿。
// 也就是说：那次修复实际上一次都没生效，而判据是绿的。
// 这条判据直接比位置：汇报块必须排在正常 return 之前。
func TestReseedReportIsReachable(t *testing.T) {
	src, err := os.ReadFile("keepseed_reseed.go")
	if err != nil {
		t.Fatalf("读不到 keepseed_reseed.go: %v", err)
	}
	code := stripKeepseedComments(string(src))

	plain := strings.Index(code, "\treturn created, nil")
	report := strings.Index(code, "return created, fmt.Errorf(")
	if plain < 0 || report < 0 {
		t.Fatalf("判据没有落点（plain=%d report=%d）——两个返回点都得在", plain, report)
	}
	if report > plain {
		t.Errorf("汇报块排在 return created, nil 之后 ⇒ 不可达，#384 的修复从未生效（plain@%d, report@%d）", plain, report)
	}
}
