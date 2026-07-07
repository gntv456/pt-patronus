package site

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"gopkg.in/yaml.v3"
)

// five_sites_test.go 锁定 Audiences / 藏宝阁(cbg) / 躺平(tangpt) / NovaHD / UBits 五站
// 用户明细解析的回归测试。用真实 YAML 定义（生产配置）+ 内联 HTML（忠实还原各站 DOM），
// 不依赖本机 D:\html，CI 可跑。
//
// 覆盖三类修复：
//  1. 逐键合并（mergeDefaultFields）：站点只配 bonus 时仍继承 uploaded/downloaded/ratio。
//  2. 正则失配返空：ratio 显示「无限」时返 0 而非误抓魔力值。
//  3. 默认 bonus/seeding + Audiences 自定义皮肤（ud-stat）选择器。

// defOf 加载 definitions/<id>.yaml 并 EnsureDefaults，返回生产配置（与运行时一致）。
func defOf(t *testing.T, id string) *SiteMetadata {
	t.Helper()
	p := filepath.Join("..", "..", "definitions", id+".yaml")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	var def SiteMetadata
	if err := yaml.Unmarshal(data, &def); err != nil {
		t.Fatalf("unmarshal %s: %v", p, err)
	}
	return EnsureDefaults(&def)
}

func mustDoc(t *testing.T, html string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader([]byte(html)))
	if err != nil {
		t.Fatalf("parse html: %v", err)
	}
	return doc
}

// nexusphpUserBarHTML 还原 NexusPHP 用户条（<span class="medium"> 内的 color_* 锚点 +
// [使用] 魔力、当前活动 ↑↓）+ rowhead 资料表（加入日期/邮箱/等级/魔力值行）。忠实于
// cbg/novahd/ubits/tangpt 真页结构（见 five_sites_calib_probe_test.go 校准）。
func nexusphpUserBarHTML() string {
	return `<html><body>
<span class="medium">
欢迎回来, <a href="userdetails.php?id=10422">gntv</a> [退出]
[<a href="usercp.php"> 控制面板 </a>] [<a href="torrents.php?inclbookmarked=1">收藏</a>]
<font class="color_bonus">魔力值 </font>[<a href="mybonus.php">使用</a>]: 940,870.1 [签到已得86]
<a href="task.php">[任务]</a> <font class="color_invite">邀请 </font>[<a href="invite.php?id=10422">发送</a>]: 0(0) <br>
<font class="color_ratio">分享率:</font> 7.539
<font class="color_uploaded">上传量:</font> 27.10 GB
<font class="color_downloaded"> 下载量:</font> 3.59 GB
<font class="color_active">当前活动:</font>
<img class="arrowup" alt="Torrents seeding" title="当前做种" src="trans.gif">1
<img class="arrowdown" alt="Torrents leeching" title="当前下载" src="trans.gif">1
</span>
<table><tbody>
<tr><td class="rowhead nowrap" align="right">加入日期</td><td class="rowfollow">2025-08-01 11:32:41</td></tr>
<tr><td class="rowhead nowrap" align="right">邮箱地址</td><td class="rowfollow">95836184@qq.com</td></tr>
<tr><td class="rowhead nowrap" align="right">等级</td><td class="rowfollow"><img alt="User" title="User" src="user.gif"> </td></tr>
<tr><td class="rowhead nowrap" align="right">魔力值</td><td class="rowfollow">940,870.1</td></tr>
</tbody></table>
</body></html>`
}

// audiencesDetailHTML 还原 Audiences 自定义皮肤用户详情页（ud-stat 卡片 + hero meta + 活跃徽标）。
func audiencesDetailHTML() string {
	return `<html><body>
<div class="ud-hero">
  <div class="ud-hero__badges"><span class="ud-class"><b class="CrazyUser_Name">(街头霸王)Crazy User</b></span></div>
  <div class="ud-hero__meta"><span><i class="far fa-calendar-plus" aria-hidden="true"></i>加入日期：2024-12-05 11:50:05</span></div>
</div>
<div class="ud-stats">
  <div class="ud-stat"><div class="ud-stat__body"><div class="ud-stat__value">2.778 TB</div><div class="ud-stat__label">上传量</div></div></div>
  <div class="ud-stat"><div class="ud-stat__body"><div class="ud-stat__value">508.31 GB</div><div class="ud-stat__label">下载量</div></div></div>
  <div class="ud-stat"><div class="ud-stat__body"><div class="ud-stat__value">5.595</div><div class="ud-stat__label">分享率</div></div></div>
  <div class="ud-stat"><div class="ud-stat__body"><div class="ud-stat__value">564831.6</div><div class="ud-stat__label">爆米花</div></div></div>
  <div class="ud-stat"><div class="ud-stat__body"><div class="ud-stat__value">983626.6</div><div class="ud-stat__label">做种积分</div></div></div>
</div>
<span class="site-userbar__compact-metric site-userbar__compact-metric--active" title="活跃 ↑ 75 / ↓ 0" aria-label="活跃 ↑ 75 / ↓ 0"></span>
</body></html>`
}

// TestFiveSitesDefaultsPerKeyMerge 锁定逐键合并修复：四站（cbg 只配 bonus、tangpt 只配 bonus、
// novahd 不配、ubits 不配）经 EnsureDefaults 后，UserInfo.Fields 必须同时含
// uploaded/downloaded/ratio/bonus/seeding——否则站点明细恒空（原「整段非空就不填默认」bug）。
func TestFiveSitesDefaultsPerKeyMerge(t *testing.T) {
	for _, id := range []string{"cbg", "tangpt", "novahd", "ubits", "audiences"} {
		def := defOf(t, id)
		for _, key := range []string{"uploaded", "downloaded", "ratio", "bonus", "seeding"} {
			if _, ok := def.UserInfo.Fields[key]; !ok {
				t.Errorf("[%s] UserInfo.Fields 缺默认键 %q（逐键合并未生效 → 明细会恒空）", id, key)
			}
		}
		for _, key := range []string{"name", "email", "level", "join_date"} {
			if _, ok := def.UserInfo.DetailFields[key]; !ok {
				t.Errorf("[%s] UserInfo.DetailFields 缺默认键 %q", id, key)
			}
		}
	}
}

// TestFourStandardSitesUserBar cbg/novahd/ubits/tangpt 共享 NexusPHP 用户条 DOM，
// 用各自生产配置解析同一份忠实 HTML，断言统计值正确（cbg/tangpt bonus 走自定义 rowhead，
// novahd/ubits bonus 走默认 [使用]）。
func TestFourStandardSitesUserBar(t *testing.T) {
	doc := mustDoc(t, nexusphpUserBarHTML())
	for _, id := range []string{"cbg", "tangpt", "novahd", "ubits"} {
		t.Run(id, func(t *testing.T) {
			def := defOf(t, id)
			u := ParseUserInfo(doc, def)
			if u.Uploaded != 29098403430 {
				t.Errorf("uploaded = %d, want 29098403430 (27.10 GB)", u.Uploaded)
			}
			if u.Downloaded != 3854733148 {
				t.Errorf("downloaded = %d, want 3854733148 (3.59 GB)", u.Downloaded)
			}
			if u.Ratio != 7.539 {
				t.Errorf("ratio = %.3f, want 7.539", u.Ratio)
			}
			if u.Bonus != 940870.1 {
				t.Errorf("bonus = %.1f, want 940870.1", u.Bonus)
			}
			if u.Seeding != 1 {
				t.Errorf("seeding = %d, want 1", u.Seeding)
			}
			// 身份字段在 detail 页（rowhead 表）
			ud := &UserInfo{}
			ParseUserDetail(doc, def, ud)
			if ud.Level != "User" {
				t.Errorf("level = %q, want User", ud.Level)
			}
			if ud.JoinDate != "2025-08-01" {
				t.Errorf("join_date = %q, want 2025-08-01", ud.JoinDate)
			}
			if ud.Email != "95836184@qq.com" {
				t.Errorf("email = %q, want 95836184@qq.com", ud.Email)
			}
		})
	}
}

// TestAudiencesDetail Audiences 自定义皮肤（无 color_*），统计全在 userdetails 的 ud-stat 卡片。
// 断言 detail_fields 选择器抽取上传/下载/分享率/爆米花/等级/加入时间。
func TestAudiencesDetail(t *testing.T) {
	def := defOf(t, "audiences")
	// Audiences 用户条页无 color_*，base 页 Fields 应全部抽空（不报错、不抓垃圾）
	baseDoc := mustDoc(t, `<html><body><span class="medium">欢迎回来, gntv</span></body></html>`)
	base := ParseUserInfo(baseDoc, def)
	if base.Uploaded != 0 || base.Ratio != 0 {
		t.Errorf("audiences base 页应无统计（无 color_*），got uploaded=%d ratio=%.3f", base.Uploaded, base.Ratio)
	}
	// detail 页 ud-stat 抽取
	u := &UserInfo{}
	ParseUserDetail(mustDoc(t, audiencesDetailHTML()), def, u)
	if u.Uploaded != 3054443301961 {
		t.Errorf("uploaded = %d, want 3054443301961 (2.778 TB)", u.Uploaded)
	}
	if u.Downloaded != 545793706557 {
		t.Errorf("downloaded = %d, want 545793706557 (508.31 GB)", u.Downloaded)
	}
	if u.Ratio != 5.595 {
		t.Errorf("ratio = %.3f, want 5.595", u.Ratio)
	}
	if u.Bonus != 564831.6 {
		t.Errorf("bonus = %.1f, want 564831.6 (爆米花)", u.Bonus)
	}
	if u.Level != "(街头霸王)Crazy User" {
		t.Errorf("level = %q, want (街头霸王)Crazy User", u.Level)
	}
	if u.JoinDate != "2024-12-05" {
		t.Errorf("join_date = %q, want 2024-12-05", u.JoinDate)
	}
	if u.Seeding != 75 {
		t.Errorf("seeding = %d, want 75（活跃 ↑75 徽标）", u.Seeding)
	}
}

// TestAudiencesUserCPUIDDiscovery Audiences usercp 无 color_* 也无 ud-stat（统计全在 userdetails），
// 其唯一职责是发现 uid+用户名供 FetchUserInfo 抓详情页。锁定用户名链接
// <a href="userdetails.php?id=27174" class="CrazyUser_Name"><b>gntv</b></a> 能被 ExtractUIDAndName 命中。
func TestAudiencesUserCPUIDDiscovery(t *testing.T) {
	html := `<html><body>
<span class="userbar-placeholder" data-uploader-label="gntv" data-uploader-url="userdetails.php?id=27174"></span>
<span class="nowrap"><a href="https://audiences.me/userdetails.php?id=27174" class="CrazyUser_Name"><b>gntv</b></a></span>
</body></html>`
	uid, name := ExtractUIDAndName(mustDoc(t, html))
	if uid != "27174" {
		t.Errorf("uid = %q, want 27174", uid)
	}
	if name != "gntv" {
		t.Errorf("name = %q, want gntv", name)
	}
}

func TestNexusPHPUUIDUserDetailsDiscovery(t *testing.T) {
	html := `<html><body>
<span class="nowrap"><a href="https://mua.xloli.cc/userdetails.php?uuid=abc-123" class="PowerUser_Name"><b>loli-user</b></a></span>
</body></html>`
	uid, name := ExtractUIDAndName(mustDoc(t, html))
	if uid != "abc-123" {
		t.Errorf("uid = %q, want abc-123", uid)
	}
	if name != "loli-user" {
		t.Errorf("name = %q, want loli-user", name)
	}
}

// TestRatioNoGarbageOnInfinite 锁定正则失配返空修复：分享率显示「无限」（下载量为 0）时，
// ratio 正则不匹配 → 返空 → 0，而非旧逻辑保留整段父文本被 ToFloat64 误抓成魔力值。
func TestRatioNoGarbageOnInfinite(t *testing.T) {
	html := `<html><body><span class="medium">
<font class="color_bonus">魔力值 </font>[<a href="mybonus.php">使用</a>]: 104,863,714.0
<font class="color_ratio">分享率:</font> 无限
<font class="color_uploaded">上传量:</font> 100.00 GB
<font class="color_downloaded"> 下载量:</font> 0.00 KB
<font class="color_active">当前活动:</font> <img class="arrowup">0 <img class="arrowdown">0
</span></body></html>`
	def := defOf(t, "tangpt")
	u := ParseUserInfo(mustDoc(t, html), def)
	if u.Ratio != 0 {
		t.Errorf("ratio = %.3f, want 0（无限应返空 → 0，不得误抓魔力值 104863714）", u.Ratio)
	}
	if u.Uploaded != 107374182400 {
		t.Errorf("uploaded = %d, want 107374182400 (100 GB)", u.Uploaded)
	}
	// tangpt 自定义 bonus（rowhead 魔力值）在本 HTML 无 rowhead 行 → 空，不误抓 bar 的 104863714
	if u.Bonus != 0 {
		t.Errorf("bonus = %.1f, want 0（无 rowhead 魔力值行应空）", u.Bonus)
	}
}

// TestRegexpFilterEmptyOnNoMatch 直接锁定 ApplyFilters regexp 失配返空（防 ratio 垃圾回归）。
func TestRegexpFilterEmptyOnNoMatch(t *testing.T) {
	got := ApplyFilters("分享率: --- 后面没数字", []Filter{{Name: "regexp", Args: `分享率[：:]?\s*(\d+\.?\d*)`}})
	if got != "" {
		t.Errorf("regexp 失配应返空，got %q", got)
	}
	got = ApplyFilters("分享率: 7.539", []Filter{{Name: "regexp", Args: `分享率[：:]?\s*(\d+\.?\d*)`}})
	if !strings.HasPrefix(got, "7.539") {
		t.Errorf("regexp 命中应返捕获组，got %q", got)
	}
}

// TestSpanColorAnchors 锁定 font/span 双标签命中：部分 NexusPHP 皮肤（如 OurBits）的
// ratio/bonus 用 <span class=color_*> 而非 <font>，默认选择器须两者都命中，否则 ratio/bonus 恒 0。
func TestSpanColorAnchors(t *testing.T) {
	html := `<html><body><span class="medium">
<span class="color_bonus">魔力值 </span>[<a href="mybonus.php">使用</a>]: 369,985.1
<span class="color_ratio">分享率:</span> 3.37
<font class="color_uploaded">上传量:</font> 1.21 TB
<font class="color_downloaded">下载量:</font> 385.89 GB
<font class="color_active">当前活动:</font> <img class="arrowup">8 <img class="arrowdown">0
</span></body></html>`
	def := defOf(t, "ourbits")
	u := ParseUserInfo(mustDoc(t, html), def)
	wantUp := ParseBytes("1.21 TB") // 1.21 TB 按 TiB 归一
	if u.Ratio != 3.37 {
		t.Errorf("ratio(span) = %.3f, want 3.37（span.color_ratio 须命中）", u.Ratio)
	}
	if u.Bonus != 369985.1 {
		t.Errorf("bonus(span) = %.1f, want 369985.1（span.color_bonus 须命中）", u.Bonus)
	}
	if u.Uploaded != wantUp {
		t.Errorf("uploaded(font) = %d, want %d", u.Uploaded, wantUp)
	}
	if u.Seeding != 8 {
		t.Errorf("seeding = %d, want 8", u.Seeding)
	}
}
