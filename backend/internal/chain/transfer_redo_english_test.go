// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

// #363 Redo 在 Aria2 下必然找不到种子，而且找不到时说的是一句英文。
//
// 两处独立的问题，叠在同一条出口上：
//
//  1. **口径不一致**：resolveAndProcessResult 用 torrentInfoHash(t)（InfoHash 为空才回落
//     到 Hash，Aria2 的 Hash 是 GID、操作标识不是 infohash，见 downloader/ar/convert.go:67
//     与 downloader.go:14 的注释）；而 Redo 逐台比对的是 `ts[j].Hash == h.DownloadHash`。
//     历史里的 DownloadHash 又是 AddSkipped 用 `t.Hash` 写的（transfer.go:286）。
//     qBittorrent 下三者恰好都等于 infohash，测试永远绿；换成 Aria2，
//     历史里存的是 GID、列表比对的也是 GID —— 这一条还能对上，
//     但 organize 路径（用 infohash）与 Redo 路径（用 GID）已经互相不认识了，
//     跨下载器找种子必然落到「已在所有下载器中找不到」，而种子明明在。
//
//  2. **原因还是英文**：h.DownloadHash == "" 那条
//     `fmt.Errorf("history %d has no download hash to redo", historyID)`
//     是本文件最后一处英文原文，且它经 Redo 的 respondReasonErr 会被**原样带进响应体**
//     （#336 之后正是那条路），也经 mcpErr 进 LLM 上下文（#350）。
//     #349/#350/#351 的测试注释都点名过这句，但那些判据只覆盖「出口有没有翻译」，
//     没有任何一条钉住「这句本身不许是英文」—— 于是它活到了现在。
//
// 判据用真数据驱动：造一条 DownloadHash 为空的历史，走 Redo，看回什么。

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/media"
)

// TestRedoEmptyDownloadHashSaysChinese Redo 遇到「这条历史没有 hash」时，
// 必须给中文 + 下一步，不许把英文原文丢给界面/LLM。
func TestRedoEmptyDownloadHashSaysChinese(t *testing.T) {
	chain, db := newTransferDeepChain(t, media.MediaInfo{
		Source: "tmdb", SourceID: "movie:1", Title: "Deep Test", Year: 2026, Type: media.TypeMovie,
	})
	// 造一条没有 DownloadHash 的历史（AddSkipped 在种子 hash 为空时就会写出这种行）。
	if err := db.Create(&models.TransferHistory{
		Status: "skipped", Src: t.TempDir(), Title: "Deep Test", DownloaderID: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}

	_, err := chain.Redo(context.Background(), 1)
	if err == nil {
		t.Fatal("没有 hash 的历史不该重做成功")
	}
	got := err.Error()
	if !strings.Contains(got, "重做") && !strings.Contains(got, "hash") {
		t.Errorf("该说清「这条历史没法重做、缺的是 hash」，实际 %q", got)
	}
	// 英文原文不许出现在界面上（#349/#350/#351 的判据都漏了这条）。
	if isAllASCII(got) {
		t.Errorf("Redo 的原因还是英文原文 %q：会经 respondReasonErr 原样进响应体、经 mcpErr 进 LLM", got)
	}
	// 反向断言：旧形状那句必须消失。
	if strings.Contains(got, "has no download hash to redo") {
		t.Errorf("「has no download hash to redo」这句英文还在：%q", got)
	}
}

// isAllASCII 判定一句话里没有中文（纯英文原文）。
func isAllASCII(s string) bool {
	for _, r := range s {
		if r > 0x2E80 {
			return false
		}
	}
	return true
}

// TestRedoMatchesAria2InfoHash Redo 跨下载器找种子时，Aria2 那种「Hash=GID、InfoHash=真 hash」
// 的形态也必须能被历史里的 infohash 命中。
//
// qBittorrent 的 Hash 本身就是 infohash，两种口径等价 —— 所以历来的测试全绿。
// Aria2 不是：downloader/ar/convert.go:67 把 Hash 设成 GID（操作标识）、
// InfoHash 才放真 infohash（downloader.go:14 的注释写明「Aria2 与操作标识不同」）。
// 而历史里的 DownloadHash 是 organize 路径按 torrentInfoHash 写进去的（真 infohash）。
// Redo 若仍比对 `ts[j].Hash`（=GID），就永远不相等 ⇒ 种子明明在客户端里，
// Redo 却回「已在所有下载器中找不到，请确认它是否已被删除或做种完成后被清理」——
// 方向完全错。这不是文案问题，是匹配口径问题：口径对齐之前写什么文案都是骗人。
func TestRedoMatchesAria2InfoHash(t *testing.T) {
	// 纯函数层：Aria2 形态必须优先给 InfoHash。
	ar := downloader.Torrent{Hash: "2089b605ecca3a8297a3a5c5f", InfoHash: "abc123"}
	if got := torrentInfoHash(ar); got != "abc123" {
		t.Fatalf("torrentInfoHash 应优先给 InfoHash（Aria2 的 Hash 是 GID），got %q", got)
	}
	// qBittorrent 形态不该被弄坏。
	qb := downloader.Torrent{Hash: "abc123"}
	if got := torrentInfoHash(qb); got != "abc123" {
		t.Errorf("qBittorrent 口径不该被这次改动弄坏，got %q", got)
	}
	// 关键判据：Redo 的逐台比对必须走同一口径。这里直接读源码那一行——
	// 只钉「Redo 用 torrentInfoHash 比」，不钉实现细节（循环怎么写都行）。
	src, err := os.ReadFile("transfer.go")
	if err != nil {
		t.Fatalf("读不到 transfer.go：%v", err)
	}
	code := string(src)
	at := strings.Index(code, "func (c *TransferChain) Redo(")
	if at < 0 {
		t.Fatal("找不到 Redo（判据没有落点）")
	}
	tail := code[at:]
	if n := strings.Index(tail[1:], "\nfunc "); n > 0 {
		tail = tail[:1+n]
	}
	if strings.Contains(tail, "ts[j].Hash == h.DownloadHash") {
		t.Errorf("Redo 不许拿 ts[j].Hash（qB 是 infohash、Aria2 是 GID）单侧直比 DownloadHash：" +
			"用户侧传的是 infohash，混合下载器下必然找不到种子，却报「种子已被删除」（旧形状）")
	}
	// #364：光「改成比 infohash」也是错的 —— 写入侧存的是 t.Hash（见 hash_key_test.go），
	// 单侧换过去会把 Aria2 存量历史也弄丢。必须是两侧归一的 matchesHash。
	if !strings.Contains(tail, "matchesHash(h.DownloadHash, ts[j])") {
		t.Error("Redo 必须走 matchesHash（两侧归一），单侧换 torrentInfoHash 会弄丢 Aria2 存量历史")
	}
}

// TestOrganizeMatchesAria2InfoHash 第二处同族：resolveAndProcessResult 的匹配也是
// `ts[i].Hash == hash`，与 #363 修掉的那处一模一样。
//
// ManualOrganize(hash=…) → resolveAndProcessResult → 逐台比对 ts[i].Hash == hash。
// Aria2 下 ts[i].Hash 是 GID，而用户/MCP 传进来的是 infohash（前端拿到的就是 infohash，
// 推链磁力也是 infohash）⇒ 永远不相等 ⇒ 回「下载器（编号 N）里没有这颗种子：
// 请在「下载器」页确认它是否已被删除或换一台」—— 种子明明在，方向却指向删除/换机器。
//
// 划清边界（别改错）：Aria2 的 aria2.getFiles 要的是 **GID**（downloader/ar/
// advanced.go:147 直接把参数传给 RPC），所以 processTorrent 里的 ListFiles(ctx, t.Hash)
// 是**对的**，不许动。要统一的只有「拿用户给的 infohash 去匹配列表」这一处。
func TestOrganizeMatchesAria2InfoHash(t *testing.T) {
	src, err := os.ReadFile("transfer.go")
	if err != nil {
		t.Fatalf("读不到 transfer.go：%v", err)
	}
	code := string(src)
	at := strings.Index(code, "func (c *TransferChain) resolveAndProcessResult")
	if at < 0 {
		t.Fatal("找不到 resolveAndProcessResult（判据没有落点）")
	}
	tail := code[at:]
	if n := strings.Index(tail[1:], "\nfunc "); n > 0 {
		tail = tail[:1+n]
	}
	if strings.Contains(tail, "ts[i].Hash == hash") {
		t.Error("手动整理不许拿 ts[i].Hash（Aria2 是 GID）与用户给的 infohash 直接比：" +
			"会报「里没有这颗种子，请确认是否已被删除或换一台」，而种子其实就在那儿")
	}
	// #364 同 Redo：两侧归一，别只换一侧。
	if !strings.Contains(tail, "matchesHash(hash, ts[i])") {
		t.Error("手动整理必须走 matchesHash（两侧归一），与 Redo 同一口径")
	}
	// 反向护栏：ListFiles 必须仍传 t.Hash —— Aria2 的 getFiles 要 GID，改了会查不到文件。
	if !strings.Contains(tail, "ListFiles(ctx, t.Hash)") && !strings.Contains(code, "ListFiles(ctx, t.Hash)") {
		t.Error("ListFiles 仍应传 t.Hash：Aria2 的 aria2.getFiles 要的是 GID，改成 infohash 会查不到文件")
	}
}
