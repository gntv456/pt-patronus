package ut

import (
	"testing"
)

// TestTorrentInfoHash 二轮 P2-2 回归：最小 bencode 解析器能从真实结构的
// .torrent 字节算出 info btih（ut FileContent 路径补设 label 的前提）。
func TestTorrentInfoHash(t *testing.T) {
	// 构造最小合法 torrent：dict{announce:str, info:dict{length:i, name:str}}
	// 严格 bencode：announce 值 16 字节、name 值 4 字节、内外两层 dict 各闭合一个 e。
	torrent := "d8:announce16:http://tracker/x4:infod6:lengthi42e4:name4:testee"
	got := torrentInfoHash([]byte(torrent))
	if got == "" {
		t.Fatal("应能解析出 infohash")
	}
	// 同内容重算稳定
	if got2 := torrentInfoHash([]byte(torrent)); got2 != got {
		t.Fatalf("infohash 不稳定: %s vs %s", got, got2)
	}
	// info 内容不同 → hash 不同
	other := "d8:announce16:http://tracker/x4:infod6:lengthi43e4:name4:testee"
	if h3 := torrentInfoHash([]byte(other)); h3 == got {
		t.Fatal("info 不同 hash 应不同")
	}
	// 非 bencode 输入安全返回空
	if h := torrentInfoHash([]byte("<html>not torrent</html>")); h != "" {
		t.Fatalf("非 bencode 应返回空, got %s", h)
	}
}
