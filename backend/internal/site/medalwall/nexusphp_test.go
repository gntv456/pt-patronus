package medalwall

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func loadDoc(html string) *goquery.Document {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		panic(err)
	}
	return doc
}

// TestParseShopCards 卡片式布局（data-* 属性，好学等现代 NexusPHP 皮肤）。
func TestParseShopCards(t *testing.T) {
	html := `<html><body>
<div class="medal-card" data-id="101" data-name="上传达人" data-price="500" data-bonus="+20%" data-inventory="100">
  <img src="/medals/101.png"/><span class="medal-validity">30天</span>
</div>
<div class="medal-card" data-id="102" data-name="永久纪念" data-price="5000" data-bonus="+50%">
  <img src="/medals/102.png"/><span class="medal-validity">永久</span>
</div>
<p>魔力 加成 佩戴 购买 库存</p>
</body></html>`
	items := parseShopHTML(html, "https://hxpt.org")
	if len(items) != 2 {
		t.Fatalf("items: got %d want 2", len(items))
	}
	if items[0].MedalID != "101" || items[0].Name != "上传达人" {
		t.Errorf("item0: id=%q name=%q want 101/上传达人", items[0].MedalID, items[0].Name)
	}
	if items[0].Price != 500 || items[0].BonusRate != "+20%" || items[0].Stock != 100 {
		t.Errorf("item0: price=%v bonus=%q stock=%d (want 500 / +20%% / 100)", items[0].Price, items[0].BonusRate, items[0].Stock)
	}
	if items[0].Validity != "30天" {
		t.Errorf("item0 validity=%q want 30天", items[0].Validity)
	}
	if items[1].Validity != "永久" || items[1].Stock != -1 {
		t.Errorf("item1: validity=%q stock=%d want 永久/-1", items[1].Validity, items[1].Stock)
	}
	// 图片须绝对化
	if items[0].ImageSmall != "https://hxpt.org/medals/101.png" {
		t.Errorf("item0 image: got %q want https://hxpt.org/medals/101.png", items[0].ImageSmall)
	}
	if items[0].Currency != "魔力" || items[0].PurchaseStatus != StatusAvailable {
		t.Errorf("item0 currency/status: got %q/%s want 魔力/AVAILABLE", items[0].Currency, items[0].PurchaseStatus)
	}
}

// TestParseShopNotMedalPage 无勋章关键词 → 不解析（防误识普通页）。
func TestParseShopNotMedalPage(t *testing.T) {
	items := parseShopHTML(`<html><body><div class="medal-card" data-id="1" data-name="x"></div></body></html>`, "https://x.com")
	if items != nil {
		t.Errorf("non-medal page should not parse, got %d items", len(items))
	}
}

// 经典表格解析见 TestParseShopTableReal（真实蟹黄堡 colhead 10 列结构）。

// TestResolveMedalID id 抠取（onclick / href）。
func TestResolveMedalID(t *testing.T) {
	if id := resolveMedalID("", loadDoc(`<div><a onclick="buyMedal(123)">买</a></div>`).Find("div")); id != "123" {
		t.Errorf("onclick id: got %q want 123", id)
	}
	if id := resolveMedalID("", loadDoc(`<div><a href="medal.php?action=buy&id=456">买</a></div>`).Find("div")); id != "456" {
		t.Errorf("href id: got %q want 456", id)
	}
	if id := resolveMedalID("", loadDoc(`<div><input name="medal_id" value="789"/></div>`).Find("div")); id != "789" {
		t.Errorf("input id: got %q want 789", id)
	}
}

// TestApplySaleWindow 状态修正（缺货 / 已拥有不改）。
func TestApplySaleWindow(t *testing.T) {
	it := Item{Stock: 0, PurchaseStatus: StatusAvailable}
	applySaleWindow(&it)
	if it.PurchaseStatus != StatusOutOfStock {
		t.Errorf("stock 0 → OUT_OF_STOCK, got %s", it.PurchaseStatus)
	}
	// 已拥有不应被改
	owned := Item{Stock: 0, PurchaseStatus: StatusOwned}
	applySaleWindow(&owned)
	if owned.PurchaseStatus != StatusOwned {
		t.Errorf("OWNED must not be overridden, got %s", owned.PurchaseStatus)
	}
}

// TestParseActionOK 购买响应解析（JSON / HTML）。
func TestParseActionOK(t *testing.T) {
	cases := []struct {
		body []byte
		want bool
	}{
		{[]byte(`{"ret":0,"msg":"购买成功"}`), true},
		{[]byte(`{"code":0}`), true},
		{[]byte(`{"success":true}`), true},
		{[]byte(`{"ret":1,"msg":"魔力不足"}`), false},
		{[]byte(`<html>购买成功</html>`), true},
		{[]byte(`<html>魔力不足</html>`), false},
	}
	for _, c := range cases {
		ok, _ := parseActionOK(c.body)
		if ok != c.want {
			t.Errorf("parseActionOK(%s): got %v want %v", c.body, ok, c.want)
		}
	}
}

// TestMTeamUnsupported 馒头占位（Supported=false，防 403 噪声）。
func TestMTeamUnsupported(t *testing.T) {
	var a Adapter = MTeamAdapter{}
	if a.Supported() {
		t.Error("MTeam should be unsupported (placeholder)")
	}
	if a.Platform() != "mteam" {
		t.Errorf("platform: got %q want mteam", a.Platform())
	}
}

// TestNormalizeName 合并去重用归一（trim+lower+去空白）。
func TestNormalizeName(t *testing.T) {
	if normalizeName("  上传 达人 ") != "上传达人" {
		t.Errorf("normalizeName failed: %q", normalizeName("  上传 达人 "))
	}
	if normalizeName("ABC") != "abc" {
		t.Errorf("normalizeName should lowercase")
	}
}

// TestParseOwnedHTMLReal 真实好学 userdetails.php 勋章结构（实测 D:\html 快照）：
// class="nexus-username-medal preview"（顶部）+ "nexus-username-medal-big preview"（勋章墙区，重复），
// title=勋章名，src=图片（文件名含 hash 数字，非 medalId）。须按 name 去重、id 留空。
func TestParseOwnedHTMLReal(t *testing.T) {
	html := `<html><body>
<img src="./files/好学开站勋章6879e35ebe92955.png" title="开站勋章" class="nexus-username-medal preview" style="x">
<img src="./files/放暑假啦.gif" title="放暑假啦" class="nexus-username-medal preview" style="x">
<img src="./files/美人鱼69b7d6fbadc882.webp" title="美人鱼" class="nexus-username-medal preview" style="x">
<img src="./files/好学开站勋章6879e35ebe92955.png" title="开站勋章" class="nexus-username-medal-big preview">
<img src="./files/美人鱼69b7d6fbadc882.webp" title="美人鱼" class="nexus-username-medal-big preview">
</body></html>`
	owned := parseOwnedHTML(html, "https://hxpt.org")
	if len(owned) != 3 {
		t.Fatalf("owned: got %d want 3 (dedup -medal + -big)", len(owned))
	}
	names := map[string]bool{}
	for _, o := range owned {
		names[o.Name] = true
		if o.MedalID != "" {
			t.Errorf("owned %q id must be empty (src is filename hash, not medalId), got %q", o.Name, o.MedalID)
		}
	}
	for _, want := range []string{"开站勋章", "放暑假啦", "美人鱼"} {
		if !names[want] {
			t.Errorf("missing owned medal %q", want)
		}
	}
}

// TestParseShopTableReal 真实蟹黄堡 medal.php 经典表（实测 D:\html 快照）：
// 10 列 colhead（ID/图片/描述/可购买时间/有效期/加成/价格/库存/购买/赠送）+ input.buy[data-id]。
func TestParseShopTableReal(t *testing.T) {
	html := `<html><body><table class="main"><tbody><tr><td class="embedded"><h1>勋章</h1>
<table border="1" cellspacing="0" cellpadding="5" width="100%">
<thead><tr>
<td class="colhead">ID</td><td class="colhead">图片</td><td class="colhead">描述</td>
<td class="colhead" style="width:115px">可购买时间</td><td class="colhead">购买后有效期(天)</td>
<td class="colhead">蟹币加成</td><td class="colhead">价格</td><td class="colhead">库存</td>
<td class="colhead">购买</td><td class="colhead">赠送</td>
</tr></thead><tbody>
<tr><td>45</td><td><img src="./files/20260401log400.png" class="preview"></td><td><h1>愚人</h1></td><td>不限 ~<br>不限</td><td>永久有效</td><td>1%</td><td>41,000</td><td>无限</td><td><input type="button" class="buy" data-id="45" value="购买"></td><td><input type="button" class="gift" data-id="45" value="赠送"></td></tr>
<tr><td>42</td><td><img src="./files/65c0a7e8a1ac6.webp" class="preview"></td><td><h1>枫</h1>继续上架一波</td><td>2026-02-28 ~<br>不限</td><td>永久有效</td><td>5%</td><td>2,000,000</td><td>0</td><td><input type="button" class="" data-id="42" value="库存不足" disabled=""></td><td></td></tr>
<tr><td>27</td><td><img src="./files/1year.png" class="preview"></td><td><h1>蟹黄堡1周年</h1>永久持有</td><td>不限 ~<br>不限</td><td>永久有效</td><td>2%</td><td>20,252</td><td>无限</td><td><input type="button" class="" data-id="27" value="已经购买" disabled=""></td><td></td></tr>
</tbody></table></td></tr></tbody></table>
<p>勋章 加成 购买</p></body></html>`
	items := parseShopHTML(html, "https://crabpt.vip")
	if len(items) != 3 {
		t.Fatalf("items: got %d want 3", len(items))
	}
	byID := map[string]Item{}
	for _, it := range items {
		byID[it.MedalID] = it
	}
	if it := byID["45"]; it.Name != "愚人" || it.Price != 41000 || it.BonusRate != "1%" || it.Stock != -1 || it.PurchaseStatus != StatusAvailable {
		t.Errorf("id45: name=%q price=%v bonus=%q stock=%d status=%s want 愚人/41000/1%%/-1/AVAILABLE", it.Name, it.Price, it.BonusRate, it.Stock, it.PurchaseStatus)
	}
	if it := byID["45"]; it.Description != "" || it.Validity != "永久有效" {
		t.Errorf("id45 desc/validity: desc=%q validity=%q", it.Description, it.Validity)
	}
	if it := byID["42"]; it.PurchaseStatus != StatusOutOfStock {
		t.Errorf("id42 (库存不足) status: got %s want OUT_OF_STOCK", it.PurchaseStatus)
	}
	if it := byID["27"]; it.PurchaseStatus != StatusOwned {
		t.Errorf("id27 (已经购买) status: got %s want OWNED", it.PurchaseStatus)
	}
}

// TestParseShopCardsReal 真实 Audiences 勋章中心现代卡片（实测 D:\html 快照 medal_center.php）：
// article.medal-item[data-medal-id/data-price/data-owned] + ul.medal-item__meta>li（meta-k/meta-v）
// + a[data-original]（真实图）+ p.medal-item__desc + is-soldout class（售罄）/ form hidden 购买。
// 校验：name/id/price/bonus(爆米花加成)/validity(加成天数)/stock(库存)/desc/image(绝对URL)/currency(爆米花)/status。
func TestParseShopCardsReal(t *testing.T) {
	html := `<html><body>
<article class="medal-item is-soldout" data-medal-id="12" data-price="580000" data-limit="1" data-owned="0">
  <div class="medal-item__art"><span class="medalcontainer"><a href="javascript:;" data-original="https://audiences.me/pic/birthday4.gif"><img src="./files/birthday4.gif" title="4周年纪念勋章动图版" alt="4周年纪念勋章动图版"></a></span><span class="medal-item__ribbon medal-item__ribbon--soldout">已售罄</span></div>
  <div class="medal-item__body">
    <h3 class="medal-item__name">4周年纪念勋章动图版</h3>
    <p class="medal-item__desc">观众4周年纪念勋章，仅能二选一！</p>
    <ul class="medal-item__meta">
      <li><span class="medal-item__meta-k">价格</span><span class="medal-item__meta-v medal-item__price"><i class="fas fa-coins"></i> 580,000</span></li>
      <li><span class="medal-item__meta-k">库存</span><span class="medal-item__meta-v" data-medal-balance="">0</span></li>
      <li><span class="medal-item__meta-k">限购</span><span class="medal-item__meta-v">每人 1</span></li>
      <li><span class="medal-item__meta-k">爆米花加成</span><span class="medal-item__meta-v medal-item__buff">18%</span></li>
      <li><span class="medal-item__meta-k">加成天数</span><span class="medal-item__meta-v">365 天</span></li>
    </ul>
  </div>
  <div class="medal-item__foot"><form action="https://audiences.me/medal_center.php?" method="post" class="medal-item__buy"><input type="hidden" name="action" value="buy"><input type="hidden" name="id" value="12"><input type="text" name="uid" value="" class="medal-item__uid"><button type="submit" class="medal-item__btn" disabled><span class="medal-item__btn-label">已售罄</span></button></form></div>
</article>
<article class="medal-item" data-medal-id="11" data-price="280000" data-owned="0">
  <div class="medal-item__art"><a href="javascript:;" data-original="https://audiences.me/pic/birthday4.png"><img src="./files/birthday4.png" title="4周年纪念勋章" alt="4周年纪念勋章"></a></div>
  <div class="medal-item__body">
    <h3 class="medal-item__name">4周年纪念勋章</h3>
    <p class="medal-item__desc">观众4周年纪念勋章。</p>
    <ul class="medal-item__meta">
      <li><span class="medal-item__meta-k">价格</span><span class="medal-item__meta-v medal-item__price">280,000</span></li>
      <li><span class="medal-item__meta-k">库存</span><span class="medal-item__meta-v">100</span></li>
      <li><span class="medal-item__meta-k">爆米花加成</span><span class="medal-item__meta-v medal-item__buff">8%</span></li>
      <li><span class="medal-item__meta-k">加成天数</span><span class="medal-item__meta-v">365 天</span></li>
    </ul>
  </div>
  <div class="medal-item__foot"><form class="medal-item__buy"><input type="hidden" name="action" value="buy"><input type="hidden" name="id" value="11"><button type="submit" class="medal-item__btn"><span class="medal-item__btn-label">购买</span></button></form></div>
</article>
<p>勋章 加成 购买 库存 爆米花</p>
</body></html>`
	items := parseShopHTML(html, "https://audiences.me")
	if len(items) != 2 {
		t.Fatalf("items: got %d want 2", len(items))
	}
	byID := map[string]Item{}
	for _, it := range items {
		byID[it.MedalID] = it
	}
	if it := byID["12"]; it.Name != "4周年纪念勋章动图版" || it.Price != 580000 || it.BonusRate != "18%" ||
		it.Validity != "365 天" || it.Stock != 0 || it.PurchaseStatus != StatusOutOfStock ||
		it.Currency != "爆米花" || it.ImageSmall != "https://audiences.me/pic/birthday4.gif" ||
		it.Description != "观众4周年纪念勋章，仅能二选一！" {
		t.Errorf("id12: name=%q price=%v bonus=%q validity=%q stock=%d status=%s currency=%q img=%q desc=%q\nwant 4周年纪念勋章动图版/580000/18%%/365 天/0/OUT_OF_STOCK/爆米花/绝对URL/描述",
			it.Name, it.Price, it.BonusRate, it.Validity, it.Stock, it.PurchaseStatus, it.Currency, it.ImageSmall, it.Description)
	}
	if it := byID["11"]; it.Name != "4周年纪念勋章" || it.Price != 280000 || it.BonusRate != "8%" ||
		it.Validity != "365 天" || it.Stock != 100 || it.PurchaseStatus != StatusAvailable || it.Currency != "爆米花" {
		t.Errorf("id11: name=%q price=%v bonus=%q validity=%q stock=%d status=%s currency=%q want 4周年纪念勋章/280000/8%%/365 天/100/AVAILABLE/爆米花",
			it.Name, it.Price, it.BonusRate, it.Validity, it.Stock, it.PurchaseStatus, it.Currency)
	}
}

// TestParseShopCardsUBits 真实 UBits 勋章商城卡片（实测 D:\html 快照 medal.php）：
// div.medal-card.unpurchased + .medal-name + .medal-info>div（<strong>LABEL：</strong>value，strong 是 label 非 value）
// + input.buy[data-id][value=购买/仅授予/已过可购买时间]。三态全有（快照 18购买/1仅授予/1已过）。
// 关键守护：unpurchased class 不得被误判 OWNED（"purchased" 子串陷阱，parseCardStatus 用 token 精确匹配）。
func TestParseShopCardsUBits(t *testing.T) {
	html := `<html><body><div class="medal-cards">
<div class="medal-card unpurchased visible">
  <div class="medal-image-container"><img class="medal-image" src="./files/Audited_team.gif" alt="种审组"></div>
  <div class="medal-name">种审组</div>
  <div class="medal-info">
    <div><strong>可购买时间：</strong>不限 至 不限</div>
    <div><strong>购买后有效期(天)：</strong>永久有效</div>
    <div><strong>U币加成：</strong>30%</div>
    <div><strong>价格：</strong>0</div>
    <div><strong>手续费：</strong>0%</div>
  </div>
  <div class="medal-action"><input type="button" class="btn buy" data-id="64" value="仅授予" disabled><div class="gift-container"><input type="button" class="btn gift" data-id="64" value="仅授予" disabled></div></div>
</div>
<div class="medal-card unpurchased visible">
  <div class="medal-image-container"><img class="medal-image" src="./files/The_Third_years.gif" alt="三周年勋章"></div>
  <div class="medal-name">三周年勋章</div>
  <div class="medal-info">
    <div><strong>可购买时间：</strong>2026-03-29 00:00 至 2026-04-30 23:59</div>
    <div><strong>购买后有效期(天)：</strong>永久有效</div>
    <div><strong>U币加成：</strong>1%</div>
    <div><strong>价格：</strong>500,000</div>
    <div><strong>手续费：</strong>100%</div>
  </div>
  <div class="medal-action"><input type="button" class="btn buy" data-id="62" value="已过可购买时间" disabled><div class="gift-container"><input type="button" class="btn gift" data-id="62" value="已过可购买时间" disabled></div></div>
</div>
<div class="medal-card unpurchased visible">
  <div class="medal-image-container"><img class="medal-image" src="./files/36.China.Film.Group.png" alt="中影"></div>
  <div class="medal-name">中影</div>
  <div class="medal-info">
    <div><strong>购买后有效期(天)：</strong>永久有效</div>
    <div><strong>U币加成：</strong>1%</div>
    <div><strong>价格：</strong>200,000</div>
  </div>
  <div class="medal-action"><input type="button" class="btn buy" data-id="60" value="购买"><div class="gift-container"><input type="button" class="btn gift" data-id="60" value="赠送"></div></div>
</div>
</div><p>勋章 加成 购买 价格</p></body></html>`
	items := parseShopHTML(html, "https://ubits.club")
	if len(items) != 3 {
		t.Fatalf("items: got %d want 3", len(items))
	}
	byID := map[string]Item{}
	for _, it := range items {
		byID[it.MedalID] = it
	}
	if it := byID["64"]; it.Name != "种审组" || it.Price != 0 || it.BonusRate != "30%" ||
		it.Validity != "永久有效" || it.PurchaseStatus != StatusGrantOnly || it.Currency != "U币" {
		t.Errorf("id64: name=%q price=%v bonus=%q validity=%q status=%s currency=%q want 种审组/0/30%%/永久有效/GRANT_ONLY/U币",
			it.Name, it.Price, it.BonusRate, it.Validity, it.PurchaseStatus, it.Currency)
	}
	if it := byID["62"]; it.Name != "三周年勋章" || it.Price != 500000 || it.BonusRate != "1%" ||
		it.PurchaseStatus != StatusDiscontinued || it.Currency != "U币" {
		t.Errorf("id62: name=%q price=%v bonus=%q status=%s currency=%q want 三周年勋章/500000/1%%/DISCONTINUED/U币",
			it.Name, it.Price, it.BonusRate, it.PurchaseStatus, it.Currency)
	}
	if it := byID["60"]; it.Name != "中影" || it.Price != 200000 || it.BonusRate != "1%" ||
		it.PurchaseStatus != StatusAvailable || it.Currency != "U币" {
		t.Errorf("id60 (购买): name=%q price=%v bonus=%q status=%s currency=%q want 中影/200000/1%%/AVAILABLE/U币 (unpurchased 不得误判 OWNED)",
			it.Name, it.Price, it.BonusRate, it.PurchaseStatus, it.Currency)
	}
}

// TestParseShopCardsHxpt 真实好学 medal.php 卡片布局（实测 D:\html 快照）：
// div.medal-card + button.view-detail[data-name/data-price/data-bonus/data-duration/data-inventory/data-image]
// + medal-title(含 owned-badge) + medal-detail-item(定价/时长/加成)。
func TestParseShopCardsHxpt(t *testing.T) {
	html := `<html><body><div class="medal-grid">
<div class="medal-card">
  <div class="medal-image-container"><img src="./files/bailu.gif" alt="二十四节气 - 白露" class="medal-image"></div>
  <div class="medal-content">
    <div class="medal-title">二十四节气 - 白露<div class="owned-badge">已拥有</div></div>
    <div class="medal-description">白露节气</div>
    <div class="medal-details">
      <div class="medal-detail-item"><span>定价:</span><strong>188,888</strong></div>
      <div class="medal-detail-item"><span>时长:</span><strong>360 天</strong></div>
      <div class="medal-detail-item"><span>加成:</span><strong>1%</strong></div>
    </div>
    <div class="medal-actions">
      <button class="action-btn view-detail" data-id="16" data-name="二十四节气 - 白露" data-price="188888" data-bonus="1%" data-duration="360 天" data-inventory="无限" data-sale-begin="2025-09-06 00:00" data-sale-end="不限" data-image="https://img.ptang.top/bailu.gif">详情</button>
      <button class="action-btn buy" data-id="16" disabled>已拥有</button>
    </div>
  </div>
</div>
<div class="medal-card">
  <div class="medal-image-container"><img src="./files/qiufen.gif" alt="秋分" class="medal-image"></div>
  <div class="medal-content">
    <div class="medal-title">秋分</div>
    <div class="medal-details">
      <div class="medal-detail-item"><span>定价:</span><strong>50000</strong></div>
      <div class="medal-detail-item"><span>加成:</span><strong>2%</strong></div>
    </div>
    <div class="medal-actions">
      <button class="action-btn view-detail" data-id="17" data-name="秋分" data-price="50000" data-bonus="2%" data-inventory="100">详情</button>
      <button class="action-btn buy" data-id="17">购买</button>
    </div>
  </div>
</div>
</div><p>魔力 加成 购买</p></body></html>`
	items := parseShopHTML(html, "https://www.hxpt.org")
	if len(items) != 2 {
		t.Fatalf("items: got %d want 2", len(items))
	}
	byID := map[string]Item{}
	for _, it := range items {
		byID[it.MedalID] = it
	}
	if it := byID["16"]; it.Name != "二十四节气 - 白露" || it.Price != 188888 || it.BonusRate != "1%" || it.Validity != "360 天" || it.Stock != -1 || it.PurchaseStatus != StatusOwned {
		t.Errorf("id16: name=%q price=%v bonus=%q validity=%q stock=%d status=%s want 二十四节气-白露/188888/1%%/360天/-1/OWNED", it.Name, it.Price, it.BonusRate, it.Validity, it.Stock, it.PurchaseStatus)
	}
	if it := byID["16"]; it.ImageSmall != "https://img.ptang.top/bailu.gif" {
		t.Errorf("id16 image: got %q want data-image 绝对URL", it.ImageSmall)
	}
	if it := byID["17"]; it.Name != "秋分" || it.Price != 50000 || it.BonusRate != "2%" || it.Stock != 100 || it.PurchaseStatus != StatusAvailable {
		t.Errorf("id17: name=%q price=%v bonus=%q stock=%d status=%s want 秋分/50000/2%%/100/AVAILABLE", it.Name, it.Price, it.BonusRate, it.Stock, it.PurchaseStatus)
	}
}

// TestParseShopCardsCspt 真实财神 medal.php 卡片布局（实测 D:\html 快照）：
// div.medal-card[data-id] + class(purchased/locked) + h4.medal-name + .stat-item(label>span.stat-value)
// + gift-btn[data-name/data-price/data-image]。
func TestParseShopCardsCspt(t *testing.T) {
	html := `<html><body>
<div class="medal-card purchased" data-id="53"><span class="status-icon unlocked">✓</span>
  <div class="medal-image"><img src="./files/qixi.png" alt="七夕"></div>
  <div class="medal-info">
    <h4 class="medal-name">七夕 [星河誓爱章]</h4>
    <p class="medal-desc">星河见证爱情</p>
    <div class="stats-grid">
      <div class="stat-item">购买后有效期(天): <span class="stat-value">365</span></div>
      <div class="stat-item">魔力加成: <span class="stat-value primary">+2%</span></div>
      <div class="stat-item">价格: <span class="stat-value primary">150,000</span></div>
      <div class="stat-item">库存: <span class="stat-value">无限</span></div>
    </div>
    <div class="actions"><button class="btn wear-btn" data-id="10342" data-medal-id="53">佩戴</button><button class="btn gift-btn" data-id="53" data-name="七夕 [星河誓爱章]" data-image="https://img1.pixhost.to/qixi.png" data-price="150,000" data-fee="10%" data-total="165,000">赠送</button></div>
  </div>
</div>
<div class="medal-card locked" data-id="63"><span class="status-icon locked">🔒</span>
  <div class="medal-image"><img src="./files/suishou.gif" alt="岁首华章"></div>
  <div class="medal-info">
    <h4 class="medal-name">岁首华章</h4>
    <div class="stats-grid">
      <div class="stat-item">价格: <span class="stat-value primary">200,000</span></div>
      <div class="stat-item">魔力加成: <span class="stat-value primary">+5%</span></div>
      <div class="stat-item">库存: <span class="stat-value">无限</span></div>
    </div>
    <div class="actions"><button class="btn gift-btn" data-id="63" data-name="岁首华章" data-price="200,000">赠送</button></div>
  </div>
</div>
<p>魔力 加成 购买</p></body></html>`
	items := parseShopHTML(html, "https://cspt.top")
	if len(items) != 2 {
		t.Fatalf("items: got %d want 2", len(items))
	}
	byID := map[string]Item{}
	for _, it := range items {
		byID[it.MedalID] = it
	}
	if it := byID["53"]; it.Name != "七夕 [星河誓爱章]" || it.Price != 150000 || it.BonusRate != "+2%" || it.Validity != "365" || it.Stock != -1 || it.PurchaseStatus != StatusOwned {
		t.Errorf("id53: name=%q price=%v bonus=%q validity=%q stock=%d status=%s want 七夕/150000/+2%%/365/-1/OWNED", it.Name, it.Price, it.BonusRate, it.Validity, it.Stock, it.PurchaseStatus)
	}
	if it := byID["53"]; it.ImageSmall != "https://img1.pixhost.to/qixi.png" {
		t.Errorf("id53 image: got %q", it.ImageSmall)
	}
	if it := byID["63"]; it.Name != "岁首华章" || it.Price != 200000 || it.BonusRate != "+5%" || it.PurchaseStatus != StatusNotOnSale {
		t.Errorf("id63 (locked): name=%q price=%v bonus=%q status=%s want 岁首华章/200000/+5%%/NOT_ON_SALE", it.Name, it.Price, it.BonusRate, it.PurchaseStatus)
	}
}

// TestParseShopCardsWornExcluded 财神 worn-medals-bar（顶部「当前佩戴」栏）的
// worn-medal-item[div data-medal-id] 不应被误当商店 card 抓（否则 name 空 → "未命名"）。
func TestParseShopCardsWornExcluded(t *testing.T) {
	html := `<html><body>
<div class="worn-medals-bar"><div class="worn-medals-list">
  <div class="worn-medal-item" data-medal-id="12" title="等级勋章"><img src="x" alt="等级勋章"><div class="worn-medal-name">等级勋章</div></div>
  <div class="worn-medal-item" data-medal-id="15" title="荣誉勋章"><img src="y" alt="荣誉勋章"><div class="worn-medal-name">荣誉勋章</div></div>
</div></div>
<div class="medal-card" data-id="100">
  <h4 class="medal-name">真勋章</h4>
  <button class="gift-btn" data-id="100" data-name="真勋章" data-price="100">赠送</button>
</div>
<p>魔力 加成 购买</p></body></html>`
	items := parseShopHTML(html, "https://cspt.top")
	if len(items) != 1 {
		t.Fatalf("items: got %d want 1 (worn-medal-item 必须排除，不当作商店 card)", len(items))
	}
	if items[0].Name != "真勋章" {
		t.Errorf("name: got %q want 真勋章", items[0].Name)
	}
	if items[0].MedalID != "100" {
		t.Errorf("id: got %q want 100", items[0].MedalID)
	}
}

// TestParseShopCardsHxptPremium 好学过期勋章：sale-end 已过但可溢价 +50% 购买
//（data-buy-price=溢价 283332 > data-price=原价 188888），价格取溢价、状态可购买（非下架）。
func TestParseShopCardsHxptPremium(t *testing.T) {
	html := `<html><body><div class="medal-grid">
<div class="medal-card">
  <div class="medal-content">
    <div class="medal-title">扫墓祭祖</div>
    <div class="medal-details">
      <div class="medal-detail-item"><span>定价:</span><strong>188,888</strong></div>
      <div class="medal-detail-item"><span>购买价(过期+50%):</span><strong>283,332</strong></div>
    </div>
    <div class="medal-actions">
      <button class="view-detail" data-id="58" data-name="扫墓祭祖" data-price="188888" data-buy-price="283332" data-markup="1" data-duration="360 天" data-bonus="1%" data-inventory="无限" data-sale-begin="2019-04-03 18:41" data-sale-end="2020-04-30 18:41">详情</button>
      <button class="buy" data-id="58">购买</button>
    </div>
  </div>
</div>
</div><p>魔力 加成 购买</p></body></html>`
	items := parseShopHTML(html, "https://www.hxpt.org")
	if len(items) != 1 {
		t.Fatalf("items: got %d want 1", len(items))
	}
	it := items[0]
	if it.Name != "扫墓祭祖" {
		t.Errorf("name: got %q want 扫墓祭祖", it.Name)
	}
	if it.Price != 283332 {
		t.Errorf("price: got %v want 283332 (data-buy-price 溢价优先于 data-price 原价)", it.Price)
	}
	if it.PurchaseStatus != StatusAvailable {
		t.Errorf("status: got %s want AVAILABLE (过期可溢价购买，非 DISCONTINUED)", it.PurchaseStatus)
	}
}

// TestParseShopCardsLongPT 真实 LongPT medal.php 卡片布局（实测 D:\html 快照）：
// h2.medal-name + detail-item(detail-label/detail-value span) + input[value=购买/仅授予]。
func TestParseShopCardsLongPT(t *testing.T) {
	html := `<html><body><div class="medal-grid">
<div class="medal-card">
  <div class="medal-header">
    <div class="medal-image-container"><img class="medal-image" alt="五万种纪念"></div>
    <h2 class="medal-name">五万种纪念</h2>
  </div>
  <div class="medal-description">五万种纪念勋章</div>
  <div class="medal-details">
    <div class="detail-item"><span class="detail-label">购买后有效期(天)</span><span class="detail-value">永久有效</span></div>
    <div class="detail-item"><span class="detail-label">魔力加成</span><span class="detail-value">1.5%</span></div>
    <div class="detail-item"><span class="detail-label">价格</span><span class="detail-value">150,000</span></div>
    <div class="detail-item"><span class="detail-label">库存</span><span class="detail-value">无限</span></div>
  </div>
  <div class="medal-actions"><div class="action-row"><input type="button" class="" data-id="201" value="仅授予" disabled=""></div></div>
</div>
<div class="medal-card">
  <div class="medal-header">
    <div class="medal-image-container"><img class="medal-image" alt="端午节"></div>
    <h2 class="medal-name">端午节</h2>
  </div>
  <div class="medal-details">
    <div class="detail-item"><span class="detail-label">价格</span><span class="detail-value">88,888</span></div>
    <div class="detail-item"><span class="detail-label">魔力加成</span><span class="detail-value">1%</span></div>
  </div>
  <div class="medal-actions"><div class="action-row"><input type="button" class="medal-action-btn-unified buy" data-id="200" value="购买"></div></div>
</div>
</div><p>魔力 加成 购买</p></body></html>`
	items := parseShopHTML(html, "https://longpt.org")
	if len(items) != 2 {
		t.Fatalf("items: got %d want 2", len(items))
	}
	byID := map[string]Item{}
	for _, it := range items {
		byID[it.MedalID] = it
	}
	if it := byID["201"]; it.Name != "五万种纪念" || it.Price != 150000 || it.BonusRate != "1.5%" || it.Validity != "永久有效" || it.Stock != -1 || it.PurchaseStatus != StatusGrantOnly {
		t.Errorf("id201: name=%q price=%v bonus=%q validity=%q stock=%d status=%s want 五万种纪念/150000/1.5%%/永久有效/-1/GRANT_ONLY", it.Name, it.Price, it.BonusRate, it.Validity, it.Stock, it.PurchaseStatus)
	}
	if it := byID["200"]; it.Name != "端午节" || it.Price != 88888 || it.BonusRate != "1%" || it.PurchaseStatus != StatusAvailable {
		t.Errorf("id200: name=%q price=%v bonus=%q status=%s want 端午节/88888/1%%/AVAILABLE", it.Name, it.Price, it.BonusRate, it.PurchaseStatus)
	}
}

// TestAppendParam 查询参数拼接：站点商店路径自带 query 时用 & 而非 ?（青蛙 medal.php?type=-1）。
func TestAppendParam(t *testing.T) {
	cases := []struct{ url, kv, want string }{
		{"https://x/medal.php", "page=1", "https://x/medal.php?page=1"},
		{"https://x/medal.php?type=-1", "page=2", "https://x/medal.php?type=-1&page=2"},
		{"https://x/medal.php?type=-1", "action=buy&id=5", "https://x/medal.php?type=-1&action=buy&id=5"},
	}
	for _, c := range cases {
		if got := appendParam(c.url, c.kv); got != c.want {
			t.Errorf("appendParam(%q,%q)=%q want %q", c.url, c.kv, got, c.want)
		}
	}
}

// TestVerifyQingwa... 临时测试已删（见 commit）。

// div.medal-item + .medal-info>h2 名称（含 <span title>未拥有</span> 状态徽章，须剥离）+ table.medal-details
// 的 <tr><td>LABEL</td><td>VALUE</td></tr>（蝌蚪加成/加成有效期/价格/库存/赠送手续费）+ input.gift-btn[data-id]。
// 三处青蛙独有点：① 字段在 medal-details 表格 tr（detailItem 走 it.Is("tr") 取第二列）；② 名称裸 h2 须剥 span[title]；
// ③ 货币「蝌蚪」（青蛙对魔力的别称）。快照 18 勋章全「已过可购买时间」→ DISCONTINUED。
func TestParseShopCardsQingwa(t *testing.T) {
	html := `<html><body><div class="medal-list">
<div class="medal-item">
  <div class=""><img src="./files/1368a29146947de6.webp" alt="海贼王（MONKEY·D·LUFFY-NIKA）" class="preview grayscale" width="200px"></div>
  <div class="medal-info">
    <h2 style="display:flex">海贼王（MONKEY·D·LUFFY-NIKA） <span title="拥有时间：未拥有" style="background:#9ebecc">未拥有</span></h2>
    <p style="min-height:60px">海贼王系列勋章</p>
    <p>2025/08/10 00:00~2025/12/31 23:59</p>
    <table class="medal-details"><tbody>
      <tr><td>蝌蚪加成</td><td>6%</td></tr>
      <tr><td>加成有效期</td><td>365</td></tr>
      <tr><td>价格</td><td>600,000</td></tr>
      <tr><td>库存</td><td>无限</td></tr>
      <tr><td>赠送手续费</td><td>10%</td></tr>
    </tbody></table>
    <div class="medal-buttons"><div class="action-container gift-action"><div class="input-group">
      <input type="number" class="uid" disabled placeholder="UID(不填写为自己购买)">
      <input type="button" class=" gift-btn" data-id="210" title="购买率: 10%" value="已过可购买时间" disabled>
    </div></div></div>
  </div>
</div>
<div class="medal-item">
  <div><img src="./files/x.webp" alt="海贼王（CHOPPER-CHRISTMAS）" class="preview grayscale"></div>
  <div class="medal-info">
    <h2>海贼王（CHOPPER-CHRISTMAS） <span title="拥有时间：未拥有">未拥有</span></h2>
    <table class="medal-details"><tbody>
      <tr><td>蝌蚪加成</td><td>0%</td></tr>
      <tr><td>加成有效期</td><td>永久有效</td></tr>
      <tr><td>价格</td><td>1,000</td></tr>
      <tr><td>库存</td><td>无限</td></tr>
    </tbody></table>
    <div class="medal-buttons"><div class="input-group">
      <input type="button" class="gift-btn" data-id="218" value="已过可购买时间" disabled>
    </div></div>
  </div>
</div>
<div class="medal-item">
  <div><img src="./files/a.webp" alt="种审组专属徽章" class="preview grayscale"></div>
  <div class="medal-info">
    <h2>种审组专属徽章 <span title="拥有时间：未拥有">未拥有</span></h2>
    <table class="medal-details"><tbody>
      <tr><td>蝌蚪加成</td><td>5%</td></tr>
      <tr><td>加成有效期</td><td>永久有效</td></tr>
      <tr><td>价格</td><td>0</td></tr>
      <tr><td>库存</td><td>0</td></tr>
    </tbody></table>
    <div class="medal-buttons"><div class="input-group">
      <input type="button" class="gift-btn" data-id="300" value="仅授予" disabled>
    </div></div>
  </div>
</div>
<div class="medal-item">
  <div><img src="./files/b.webp" alt="2026国庆节徽章" class="preview grayscale"></div>
  <div class="medal-info">
    <h2>2026国庆节徽章 <span title="拥有时间：未拥有">未拥有</span></h2>
    <table class="medal-details"><tbody>
      <tr><td>蝌蚪加成</td><td>3%</td></tr>
      <tr><td>加成有效期</td><td>365</td></tr>
      <tr><td>价格</td><td>100,000</td></tr>
      <tr><td>库存</td><td>无限</td></tr>
    </tbody></table>
    <div class="medal-buttons"><div class="input-group">
      <input type="button" class="gift-btn" data-id="301" value="未到可购买时间" disabled>
    </div></div>
  </div>
</div>
</div><p>勋章 加成 购买 库存 价格 有效期</p></body></html>`
	items := parseShopHTML(html, "https://www.qingwapt.com")
	if len(items) != 4 {
		t.Fatalf("items: got %d want 4", len(items))
	}
	byID := map[string]Item{}
	for _, it := range items {
		byID[it.MedalID] = it
	}
	if it := byID["210"]; it.Name != "海贼王（MONKEY·D·LUFFY-NIKA）" || it.Price != 600000 || it.BonusRate != "6%" ||
		it.Validity != "365" || it.Stock != -1 || it.PurchaseStatus != StatusDiscontinued || it.Currency != "蝌蚪" {
		t.Errorf("id210: name=%q price=%v bonus=%q validity=%q stock=%d status=%s currency=%q\nwant 海贼王(MONKEY·D·LUFFY-NIKA)/600000/6%%/365/-1/DISCONTINUED/蝌蚪",
			it.Name, it.Price, it.BonusRate, it.Validity, it.Stock, it.PurchaseStatus, it.Currency)
	}
	// 名称不得混入状态徽章「未拥有」
	if strings.Contains(byID["210"].Name, "未拥有") {
		t.Errorf("id210 name leaked status span: %q (须剥离 span[title])", byID["210"].Name)
	}
	if it := byID["218"]; it.Name != "海贼王（CHOPPER-CHRISTMAS）" || it.Price != 1000 || it.BonusRate != "0%" ||
		it.Validity != "永久有效" || it.Stock != -1 || it.PurchaseStatus != StatusDiscontinued || it.Currency != "蝌蚪" {
		t.Errorf("id218: name=%q price=%v bonus=%q validity=%q stock=%d status=%s currency=%q want 海贼王(CHOPPER-CHRISTMAS)/1000/0%%/永久有效/-1/DISCONTINUED/蝌蚪",
			it.Name, it.Price, it.BonusRate, it.Validity, it.Stock, it.PurchaseStatus, it.Currency)
	}
	// 仅授予 + 库存 0：状态须为 GRANT_ONLY，不得被 applySaleWindow 误覆盖为 OUT_OF_STOCK
	if it := byID["300"]; it.Name != "种审组专属徽章" || it.Price != 0 || it.Stock != 0 ||
		it.PurchaseStatus != StatusGrantOnly || it.Currency != "蝌蚪" {
		t.Errorf("id300 (仅授予/库存0): name=%q price=%v stock=%d status=%s currency=%q want 种审组专属徽章/0/0/GRANT_ONLY/蝌蚪 (不得误判 OUT_OF_STOCK)",
			it.Name, it.Price, it.Stock, it.PurchaseStatus, it.Currency)
	}
	// 未到可购买时间 → NOT_ON_SALE（青蛙真实 16 个此态，原 parseCardStatus 无「未到」映射会误判 AVAILABLE）
	if it := byID["301"]; it.Name != "2026国庆节徽章" || it.Price != 100000 || it.PurchaseStatus != StatusNotOnSale || it.Currency != "蝌蚪" {
		t.Errorf("id301 (未到可购买时间): name=%q price=%v status=%s currency=%q want 2026国庆节徽章/100000/NOT_ON_SALE/蝌蚪",
			it.Name, it.Price, it.PurchaseStatus, it.Currency)
	}
}
