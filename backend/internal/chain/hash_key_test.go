package chain

import (
	"testing"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// #364 #363 的修复在 Aria2 下引入了反向回归：这个测试钉住正确的口径，
//
//	写入侧（DownloadHash）= t.Hash
//	匹配侧（Redo/organize）= torrentInfoHash(t)
//
// 两者在 qBittorrent / Transmission / RT / qB / Aria2 除外的所有下载器上恒等
// （Hash 本身就是 infohash），所以历来的测试全绿；只有 Aria2 分叉：
//
//	ar/convert.go:66-67   Hash: t.GID（操作标识）  InfoHash: strings.ToLower(t.InfoHash)
//
// 于是 Aria2 下「写入=匹配」不成立：
//
//	历史里 DownloadHash 存的是 GID（record/AddSkipped 传的都是 t.Hash），
//	而 #363 让 Redo/organize 拿 torrentInfoHash(ts[i])（infohash）去比它 ⇒ 必然不相等。
//
// #363 之前 Redo 比的是 ts[j].Hash == h.DownloadHash，两边都是 GID，**Aria2 下本来是对的**；
// 真正的洞在另一侧：用户手动整理传的 infohash 与 Aria2 列表里的 GID 对不上。
// 也就是说：一个「统一成 torrentInfoHash」的改法同时修好手动整理、弄坏 Redo。
//
// 正确口径：匹配时**两侧都归一**到 torrentInfoHash，而不是只改一侧。
// 代价是存量 Aria2 历史里的 GID 匹配不上 infohash —— 这属于历史数据口径迁移，
// 用「两种 key 都试」兜住（GID 与 infohash 各比一次）既修好新数据也不丢旧数据。
func TestRedoMatchesBothStoredKeyShapes(t *testing.T) {
	ar := downloader.Torrent{Hash: "GID123", InfoHash: "INFOABC"}

	// 写入侧的值（历史里真的存这个）。
	stored := ar.Hash
	// 匹配侧应能同时接受两种 key：库里存的是 GID（旧）或 infohash（新）。
	if !matchesHash(stored, ar) {
		t.Errorf("Redo 匹配必须同时认 GID 与 infohash 两种 key：stored=%q", stored)
	}
	if !matchesHash(ar.InfoHash, ar) {
		t.Errorf("Redo 匹配必须认 infohash（用户/前端传的就是它）：%q", ar.InfoHash)
	}
	// 反向：不该把不相关的 hash 也判成命中。
	if matchesHash("deadbeef", ar) {
		t.Error("不相关的 hash 不许被匹配成命中（会把种子整理错）")
	}
	// qBittorrent 形态：Hash 就是 infohash，行为不许被弄坏。
	qb := downloader.Torrent{Hash: "abc123"}
	if !matchesHash("abc123", qb) {
		t.Error("qBittorrent 的 Hash 形态必须照旧能匹配")
	}
}
