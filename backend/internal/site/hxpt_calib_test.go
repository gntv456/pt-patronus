package site

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// calibratedHxpt 与 definitions/hxpt.yaml 保持一致的选择器（按站点源码校准）。
func calibratedHxpt() *SiteMetadata {
	return &SiteMetadata{
		ID: "hxpt", Schema: "nexusphp",
		Selectors: SiteSelectors{
			Rows: ElementQuery{Selector: "table.torrents tr"},
			Fields: map[string]ElementQuery{
				"title":     {Selector: `a[href^="details.php"]:not([href*="#"])`},
				"subtitle":  {Selector: ".torrent-subtitle"},
				"detail":    {Selector: `a[href^="details.php"]:not([href*="#"])`, Attr: "href"},
				"download":  {Selector: `a[href^="download.php"]`, Attr: "href"},
				"completed": {Selector: `a[href^="viewsnatches.php"]`},
				"seeders":   {Selector: `a[href*="#seeders"]`},
				"leechers":  {Selector: `a[href*="#leechers"]`},
				"size":      {Selector: "td:nth-last-child(5)"},
			},
		},
		UserInfo: UserInfoDef{
			Fields: map[string]ElementQuery{
				"level":      {Selector: `td.rowhead:contains("等级") + td`},
				"uploaded":   {Selector: `td.rowhead:contains("上传量") + td`},
				"downloaded": {Selector: `td.rowhead:contains("下载量") + td`},
				"bonus":      {Selector: `td.rowhead:contains("火花") + td`},
				"ratio":      {Selector: `td.embedded:contains("分享率") font`},
			},
			VIPLevels: []string{"APP专属VIP"},
		},
	}
}

// TestHxptVIPGate 验证 user_info.level 选择器解析 + site.IsVIP 通用工具（legacy）。
// 注意：好学生产 VIP 准入已不走这里——改由 /api/verifyAccess.php 判定（见 auth.HxptVIP）。
// 本测仅守护「等级行头衔文本可被解析 + IsVIP 文本匹配」这一通用机制不被回归。
func TestHxptVIPGate(t *testing.T) {
	// 等级行：img(类名=贵宾) + 头衔文本「APP专属VIP」
	html := `<html><body><table>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">等级</td><td width="99%" class="rowfollow" valign="top" align="left"><img alt="贵宾" title="贵宾" src="pic/vip.gif" />&nbsp;APP专属VIP</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">上传量</td><td width="99%" class="rowfollow" valign="top" align="left">12.4 TB</td></tr>
<tr><td width="1%" class="rowhead nowrap" valign="top" align="right">火花</td><td width="99%" class="rowfollow" valign="top" align="left">1234.5</td></tr>
<tr><td class="embedded"><strong>分享率</strong>: <font color="#0000ff">6.890</font></td></tr>
</table></body></html>`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	ui := ParseUserInfo(doc, calibratedHxpt())
	if ui.Level != "APP专属VIP" {
		t.Errorf("level = %q, want APP专属VIP", ui.Level)
	}
	if !IsVIP(ui, calibratedHxpt()) {
		t.Error("IsVIP should be true for APP专属VIP")
	}

	// 普通用户（无 APP专属VIP 头衔，仅 class img）→ 非门槛
	plain := `<html><body><table>
<tr><td class="rowhead nowrap">等级</td><td class="rowfollow"><img alt="贵宾" title="贵宾" src="pic/vip.gif" /></td></tr>
</table></body></html>`
	doc2, _ := goquery.NewDocumentFromReader(strings.NewReader(plain))
	ui2 := ParseUserInfo(doc2, calibratedHxpt())
	if ui2.Level != "" {
		t.Errorf("plain user level = %q, want empty", ui2.Level)
	}
	if IsVIP(ui2, calibratedHxpt()) {
		t.Error("plain user should NOT be VIP (no APP专属VIP title)")
	}
}

// TestHxptTorrentsParse 验证种子列表选择器对 torrenttable 真实行结构的解析。
func TestHxptTorrentsParse(t *testing.T) {
	// 来源：include/functions.php torrenttable() 数据行（普通用户、含评论列、共 9 列）
	html := `<html><body><table class="torrents">
<tr>
<td class="colhead">类型</td><td class="colhead">名称</td><td class="colhead">评论</td>
<td class="colhead">时间</td><td class="colhead">大小</td><td class="colhead">种子</td>
<td class="colhead">下载者</td><td class="colhead">完成</td><td class="colhead">操作</td>
</tr>
<tr>
<td class="rowfollow"><img class="cat_movie" src="pic/trans.gif" alt="movie" /></td>
<td class="rowfollow" colspan="2"><a href="details.php?id=123&amp;hit=1" mouseovertorrent><b>The Movie 2024 1080p</b></a><div class="torrent-subtitle">蓝光原盘</div></td>
<td class="rowfollow"><a href="details.php?id=123&amp;hit=1&amp;cmtpage=1#startcomments">8</a></td>
<td class="rowfollow nowrap">2024-01-01</td>
<td class="rowfollow">1.46 GB</td>
<td class="rowfollow"><a href="details.php?id=123&amp;hit=1&amp;dllist=1#seeders">12</a></td>
<td class="rowfollow"><a href="details.php?id=123&amp;hit=1&amp;dllist=1#leechers">3</a></td>
<td class="rowfollow"><a href="viewsnatches.php?id=123"><b>500</b></a></td>
<td class="rowfollow" align="center"><a href="download.php?id=123"><img src="pic/xiazai.png" alt="download"/></a></td>
</tr>
</table></body></html>`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	ts := ParseSearch(doc, calibratedHxpt(), "https://hxpt.org")
	if len(ts) != 1 {
		t.Fatalf("expected 1 torrent (header skipped), got %d", len(ts))
	}
	x := ts[0]
	if x.Title != "The Movie 2024 1080p" {
		t.Errorf("title = %q", x.Title)
	}
	if x.SubTitle != "蓝光原盘" {
		t.Errorf("subtitle = %q", x.SubTitle)
	}
	if x.DetailURL != "https://hxpt.org/details.php?id=123&hit=1" {
		t.Errorf("detail = %q", x.DetailURL)
	}
	if x.DownloadURL != "https://hxpt.org/download.php?id=123" {
		t.Errorf("download = %q", x.DownloadURL)
	}
	if x.Seeders != 12 || x.Leechers != 3 || x.Completed != 500 {
		t.Errorf("seeders/leechers/completed = %d/%d/%d", x.Seeders, x.Leechers, x.Completed)
	}
	if x.Size != "1.46 GB" {
		t.Errorf("size = %q, want 1.46 GB", x.Size)
	}
}
