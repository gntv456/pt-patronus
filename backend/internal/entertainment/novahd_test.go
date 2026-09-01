package entertainment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 农场页面 HTML 片段（对齐 magic_fram.php 渲染结构）。
// 小麦 ready / 玉米 growing / 花生 idle / 土豆 idle；仓库 1 项；菜市场 小麦530 + 鸡1193。
const magicFarmHTML = `
<div class="farm-header"><div class="points-display">当前魔力值: 18,899.0</div></div>
<div class="one-key-harvest"><a href="?action=harvest_all" class="btn">一键收获</a></div>
<h2>农作物种植区 <small>农作物有效期5天</small></h2>
<div class="farm-item"><h3>小麦</h3><div class="item-actions"><a href="?action=harvest&type=crop&id=1" class="btn harvest-btn">收获</a></div></div>
<div class="farm-item"><h3>玉米</h3><div class="item-actions">剩余时间 3小时20分钟</div></div>
<div class="farm-item"><h3>花生</h3><div class="item-actions"><a href="?action=plant&type=crop&id=3" class="btn plant-btn">种植</a></div></div>
<div class="farm-item"><h3>土豆</h3><div class="item-actions"><a href="?action=plant&type=crop&id=4" class="btn plant-btn">种植</a></div></div>
<div class="farm-section"><h2>仓库</h2><table class="warehouse-table">
<tr><th>图</th><th>名</th><th>数量</th><th>到期</th><th>操作</th></tr>
<tr><td><img></td><td>小麦</td><td>3</td><td>1天2小时</td><td><a href="?action=sell&key=crop_1">出售</a></td></tr>
</table></div>
<div class="farm-section"><h2>菜市场</h2><div class="market-grid">
<div class="market-category"><h3>农作物</h3><table class="market-table">
<tr><th>图</th><th>名</th><th>价</th></tr>
<tr><td><img></td><td>小麦</td><td>530</td></tr>
</table></div>
<div class="market-category"><h3>动物</h3><table class="market-table">
<tr><th>图</th><th>名</th><th>价</th></tr>
<tr><td><img></td><td>鸡</td><td>1193</td></tr>
</table></div>
</div></div>
`

// TestMagicFarmParse 农场 HTML 解析：余额/状态/仓库/菜市场。
func TestMagicFarmParse(t *testing.T) {
	st := magicParseFarm([]byte(magicFarmHTML))
	if st.Balance != 18899 {
		t.Errorf("balance = %v, want 18899", st.Balance)
	}
	if len(st.Crops) != 4 {
		t.Fatalf("crops len = %d, want 4", len(st.Crops))
	}
	if st.Crops[0].Status != "ready" { // 小麦
		t.Errorf("wheat status = %q, want ready", st.Crops[0].Status)
	}
	if st.Crops[0].ExpireDays != 5 { // 好学农作物有效期 5 天（从 section 标题抠）
		t.Errorf("crop expire = %d, want 5", st.Crops[0].ExpireDays)
	}
	if st.Crops[1].Status != "growing" { // 玉米（无链接）
		t.Errorf("corn status = %q, want growing", st.Crops[1].Status)
	}
	if st.Crops[2].Status != "idle" { // 花生（plant 链接）
		t.Errorf("peanut status = %q, want idle", st.Crops[2].Status)
	}
	if st.Crops[0].Name != "小麦" || st.Crops[0].Price != 500 || st.Crops[0].GrowHours != 4 {
		t.Errorf("wheat def = %+v", st.Crops[0])
	}
	if len(st.Warehouse) != 1 || st.Warehouse[0].Name != "小麦" || st.Warehouse[0].Count != 3 || st.Warehouse[0].Key != "crop_1" {
		t.Errorf("warehouse = %+v", st.Warehouse)
	}
	if len(st.Market) != 2 {
		t.Fatalf("market len = %d, want 2", len(st.Market))
	}
	if st.Market[0].Name != "小麦" || st.Market[0].Section != "crop" || st.Market[0].MarketPrice != 530 {
		t.Errorf("market[0] = %+v", st.Market[0])
	}
	if st.Market[1].Name != "鸡" || st.Market[1].Section != "animal" || st.Market[1].MarketPrice != 1193 {
		t.Errorf("market[1] = %+v", st.Market[1])
	}
}

// TestMagicFarmPlant 种植操作：GET ?action=plant&type=crop&id=1 → 抠提示与余额。
func TestMagicFarmPlant(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><div class="message">种植成功: 小麦</div><div class="points-display">当前魔力值: 17,399.0</div></html>`))
	}))
	defer srv.Close()

	res, err := MagicFarmGame{}.Plant(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"}, "crop", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || !strings.Contains(res.Message, "种植成功") || res.Balance != 17399 {
		t.Errorf("plant = %+v", res)
	}
	if !strings.Contains(gotQuery, "action=plant") || !strings.Contains(gotQuery, "type=crop") || !strings.Contains(gotQuery, "id=1") {
		t.Errorf("query = %q, want action=plant&type=crop&id=1", gotQuery)
	}
}

// TestMagicFarmHarvestAll 一键收获：section=animal 的 Plant 走 breed。
func TestMagicFarmBreed(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`<html><div class="message">养殖成功: 牛</div><div class="points-display">当前魔力值: 8,899.0</div></html>`))
	}))
	defer srv.Close()

	res, err := MagicFarmGame{}.Plant(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"}, "animal", 4)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.Balance != 8899 {
		t.Errorf("breed = %+v", res)
	}
	if !strings.Contains(gotQuery, "action=breed") || !strings.Contains(gotQuery, "type=animal") || !strings.Contains(gotQuery, "id=4") {
		t.Errorf("query = %q, want action=breed&type=animal&id=4", gotQuery)
	}
}

// TestNovaHDScratchPlay 刮刮乐：buy(HTML 抠 scratch_id) → scratch(JSON) → settle。
func TestNovaHDScratchPlay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "magic_scratch.php") {
			w.WriteHeader(404)
			return
		}
		switch r.URL.Query().Get("action") {
		case "buy":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><script>var currentScratchId = 12345;</script></html>`))
		case "scratch":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"prize":500,"prize_level":"五等奖","scratch_id":12345}`))
		case "settle":
			_, _ = w.Write([]byte(`ok`))
		}
	}))
	defer srv.Close()

	res, err := NovaHDScratchGame{}.Play(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.Amount != 500 || res.Prize != "五等奖" {
		t.Errorf("scratch play = %+v", res)
	}
}

// TestNovaHDScratchBuyFail 购买失败（无 scratch_id）→ 抠提示。
func TestNovaHDScratchBuyFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><div class="message">魔力值不足</div></html>`))
	}))
	defer srv.Close()

	res, err := NovaHDScratchGame{}.Play(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Success || !strings.Contains(res.Message, "魔力值不足") {
		t.Errorf("scratch buy fail = %+v", res)
	}
}

// TestNovaHDScratchPrizes 6 档规则常量。
func TestNovaHDScratchPrizes(t *testing.T) {
	ps, err := NovaHDScratchGame{}.Prizes(context.Background(), nil, "", Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 6 {
		t.Fatalf("prizes len = %d, want 6", len(ps))
	}
	if ps[5].Name != "一等奖 +100000 魔力值" {
		t.Errorf("first prize = %q", ps[5].Name)
	}
}

// TestNovaHDLuckyPlay 幸运抽奖单抽：解析 results，中奖计入 Amount。
func TestNovaHDLuckyPlay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"results":[
			{"status":"awarded","type":"bonus","value":100,"unit":"魔力","name":"魔力值"},
			{"status":"nothing","type":"nothing"}]}`))
	}))
	defer srv.Close()

	res, err := NovaHDLuckyGame{}.Play(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || len(res.Prizes) != 2 || res.Amount != 100 {
		t.Errorf("lucky play = %+v", res)
	}
	if !res.Prizes[0].IsWin || res.Prizes[1].IsWin {
		t.Errorf("prizes win flags = %+v", res.Prizes)
	}
}

// TestNovaHDLuckyPlayMulti 连抽：count 透传 + 多结果聚合。
func TestNovaHDLuckyPlayMulti(t *testing.T) {
	var gotCount string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCount = r.FormValue("count")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"results":[
			{"status":"awarded","type":"bonus","value":50},
			{"status":"awarded","type":"attendance_card","value":1,"unit":"张"},
			{"status":"nothing","type":"nothing"}]}`))
	}))
	defer srv.Close()

	res, err := NovaHDLuckyGame{}.PlayMulti(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if gotCount != "10" {
		t.Errorf("count = %q, want 10", gotCount)
	}
	if len(res.Prizes) != 3 || res.Amount != 50 { // attendance_card value=1 不计入 Amount（IsWin 但非 bonus）
		t.Errorf("multi = %+v amount %v", res.Prizes, res.Amount)
	}
}

// TestNovaHDLuckyFail 抽奖失败（余额不足等）→ message 透传。
func TestNovaHDLuckyFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":false,"message":"魔力不足，需要 100000"}`))
	}))
	defer srv.Close()

	res, err := NovaHDLuckyGame{}.Play(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Success || !strings.Contains(res.Message, "魔力不足") {
		t.Errorf("lucky fail = %+v", res)
	}
}

// TestMagicFarmBaozi 包子（t-baozi.cc）形态：操作链接是相对 ?action=…（好学/NovaHD 为
// magic_fram.php?action=…）。magicFarmStatus 按 query 子串匹配，两种前缀都应解析出 idle。
func TestMagicFarmBaozi(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/magic_fram.php" {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><div class="points-display"><span>当前魔力值</span><strong>100.0</strong></div>
<a href="?action=plant&type=crop&id=1#farm-crops">种植</a>
<a href="?action=breed&type=animal&id=4">饲养</a>
<a href="?action=harvest_all">全部收获</a>
<div>农作物有效期2天</div><div>动物有效期5天</div></html>`))
	}))
	defer srv.Close()

	st, err := MagicFarmGame{}.Farm(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if st.Balance != 100 {
		t.Errorf("balance = %v", st.Balance)
	}
	if len(st.Crops) != 4 || st.Crops[0].Status != "idle" || st.Crops[0].Price != 500 || st.Crops[0].GrowHours != 4 {
		t.Errorf("crops[0] = %+v", st.Crops[0])
	}
	if len(st.Animals) != 4 || st.Animals[3].Status != "idle" || st.Animals[3].Name != "牛" {
		t.Errorf("animals[3] = %+v", st.Animals[3])
	}
}
