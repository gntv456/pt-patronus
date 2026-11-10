// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 批2：Bettable 下注/选号类测试。

func TestCbgSicboBet(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 256)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","new_magic_point":500,"message":"赢"}`))
	}))
	defer srv.Close()
	res, err := CbgSicboGame{}.Bet(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"}, BetRequest{Selection: "big", Amount: 100})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.Balance != 500 || !strings.Contains(res.Prize, "big") {
		t.Errorf("cbg_sicbo = %+v", res)
	}
	if !strings.Contains(gotBody, `"type":"big"`) || !strings.Contains(gotBody, `"amount":100`) {
		t.Errorf("bet body = %q", gotBody)
	}
}

func TestCrabUnionLottoBet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"m":"","data":{}}`))
	}))
	defer srv.Close()
	res, err := CrabUnionLottoGame{}.Bet(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"}, BetRequest{
		Extra: map[string]string{"reds": "1,2,3,4,5,6", "blue": "7"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || !strings.Contains(res.Prize, "7") {
		t.Errorf("crab_unionlotto = %+v", res)
	}
	// 号码不足应失败
	bad, _ := CrabUnionLottoGame{}.Bet(context.Background(), newTestClient(t), srv.URL, Creds{}, BetRequest{})
	if bad.Success {
		t.Error("unionlotto should fail without nums")
	}
}

func TestPandaFlipBet(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 512)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.Header().Set("Content-Type", "application/json")
		// flipResult.tier 是对象 {name,reward_type,reward}（真实契约）
		_, _ = w.Write([]byte(`{"ret":0,"data":{"status":1,"balance":100,"flipResult":{"tierId":"c","tier":{"name":"二等奖","reward_type":"coins","reward":5000},"reward":5000,"isSpecial":false}}}`))
	}))
	defer srv.Close()
	// round_number 经 Extra 传入 → 只发一次 recordFlip
	res, err := PandaFlipGame{}.Bet(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"}, BetRequest{
		Selection: "5", Extra: map[string]string{"round_number": "7"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.Balance != 100 || !strings.Contains(res.Prize, "二等奖") || !strings.Contains(res.Prize, "5000") {
		t.Errorf("panda_flip = %+v", res)
	}
	if !strings.Contains(gotBody, "action=recordFlip") || !strings.Contains(gotBody, "params[tile_index]=5") || !strings.Contains(gotBody, "params[round_number]=7") {
		t.Errorf("recordFlip body = %q", gotBody)
	}
}

func TestPandaFlipState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 512)
		n, _ := r.Body.Read(buf)
		body := string(buf[:n])
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(body, "action=gameState"):
			_, _ = w.Write([]byte(`{"ret":0,"data":{"roundNumber":3,"balance":9000,"costPerFlip":2000,"allFlips":[{"index":1,"tierId":"a"},{"index":18,"tierId":"c","user":"typhoonspy"}]}}`))
		case strings.Contains(body, "action=getFlipTiers"):
			_, _ = w.Write([]byte(`{"ret":0,"data":[{"id":"a","name":"特等奖","reward":50000,"reward_type":"coins","count":1},{"id":"f","name":"五等奖","reward":0,"reward_type":"none","count":36}]}`))
		}
	}))
	defer srv.Close()
	st, err := PandaFlipGame{}.FlipState(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if st.RoundNumber != 3 || st.Balance != 9000 || st.CostPerFlip != 2000 || st.TotalTiles != 100 {
		t.Errorf("flipstate basics = %+v", st)
	}
	// allFlips → 2 个已翻格（#1 / #18 by typhoonspy）
	if st.FlippedCount != 2 || len(st.Tiles) != 2 {
		t.Errorf("flipstate tiles = %+v", st.Tiles)
	}
	var t18 *FlipTile
	for i := range st.Tiles {
		if st.Tiles[i].Index == 18 {
			t18 = &st.Tiles[i]
		}
	}
	if t18 == nil || t18.TierID != "c" || t18.ByUser != "typhoonspy" {
		t.Errorf("tile #18 = %+v", t18)
	}
	if len(st.Tiers) != 2 || st.Tiers[0].Name != "特等奖" || st.Tiers[0].Reward != 50000 || st.Tiers[0].Count != 1 {
		t.Errorf("flipstate tiers = %+v", st.Tiers)
	}
	// History 由已翻格重建（#1 与 #18，倒序 #18 在前）
	if len(st.History) != 2 || !strings.Contains(st.History[0].PrizeName, "#18") || !strings.Contains(st.History[0].PrizeName, "typhoonspy") {
		t.Errorf("flipstate history = %+v", st.History)
	}
}

func TestHxptBigSmallBet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "bigsmall.php") && !strings.Contains(r.URL.RawQuery, "action") {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html data-csrf-token="tok123"></html>`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.RawQuery, "action=game_status") {
			_, _ = w.Write([]byte(`{"code":0,"data":{"game":{"id":42,"status":"betting"},"round_token":"rtok","balance":1000,"min_bet":100}}`))
			return
		}
		if strings.Contains(r.URL.RawQuery, "action=bet") {
			_, _ = w.Write([]byte(`{"code":0,"data":{"balance":900,"bet_type":"big"}}`))
		}
	}))
	defer srv.Close()
	res, err := HxptBigSmallGame{}.Bet(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"}, BetRequest{Selection: "big", Amount: 100})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.Balance != 900 || !strings.Contains(res.Prize, "big") {
		t.Errorf("hxpt_bigsmall = %+v", res)
	}
}

// 好学猜大小带 prop_id 下注（验证 body 含 prop_id/bet_type）。
func TestHxptBigSmallBetProp(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "bigsmall.php") && !strings.Contains(r.URL.RawQuery, "action") {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html data-csrf-token="tok"></html>`))
			return
		}
		buf := make([]byte, 512)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.RawQuery, "action=game_status") {
			_, _ = w.Write([]byte(`{"code":0,"data":{"game":{"id":42,"status":"betting"},"round_token":"rt","balance":1000,"min_bet":100}}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"balance":900}}`))
	}))
	defer srv.Close()
	res, err := HxptBigSmallGame{}.Bet(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"}, BetRequest{Selection: "big", Amount: 1000, Extra: map[string]string{"prop_id": "7"}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Errorf("bet failed: %+v", res)
	}
	if !strings.Contains(gotBody, "prop_id=7") || !strings.Contains(gotBody, "bet_type=big") {
		t.Errorf("bet body missing prop_id/bet_type: %q", gotBody)
	}
}

// 好学猜大小全景快照聚合（局/走势/排行/记录/任务/道具/聊天）。
func TestHxptBigSmallState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.RawQuery
		switch {
		case strings.Contains(q, "action=game_status"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"balance":1000,"min_bet":100,"max_bet":1000000,"round_token":"rt","game":{"id":42,"game_no":"G42","status":"betting","countdown":35,"seal_countdown":30,"player_count":3,"total_bet":5000},"my_bet":{"bet_type":"big","amount":500},"last_game_result":{"game_no":"G41","bet_type":"big","amount":500,"result":"big","result_amount":475},"big_bets":[{"username":"alice","amount":500}],"small_bets":[{"username":"bob","amount":200}]}}`))
		case strings.Contains(q, "action=trends"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"stats40":{"big":21,"small":19,"house":2,"big_pct":53,"small_pct":47},"stats10":{"big":6,"small":4,"big_pct":60,"small_pct":40},"trend_text":"大出现较多","recent_results":["big","small","house","big"]}}`))
		case strings.Contains(q, "action=rankings"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"monthly_king":[{"username":"alice","total_won":1000,"king_count":7}],"daily_kings":[{"username":"bob","total_won":500,"king_count":2}]}}`))
		case strings.Contains(q, "action=game_history&type=my"):
			_, _ = w.Write([]byte(`{"code":0,"data":[{"game_no":"G41","start_time":"2026-07-01 17:52:00","end_time":"2026-07-01 17:53:00","player_count":3,"total_bet":5000,"result":"big","my_bet_type":"big","my_bet":500,"my_result":475}]}`))
		case strings.Contains(q, "action=game_history&type=all"):
			_, _ = w.Write([]byte(`{"code":0,"data":[{"game_no":"G41","start_time":"t1","end_time":"t2","result":"big","bets":[{"username":"alice","bet_info":"大：500"}]}]}`))
		case strings.Contains(q, "action=my_tasks"):
			_, _ = w.Write([]byte(`{"code":0,"data":[{"id":1,"name":"参与奖","description":"累计参与","reward_description":"获魔力","progress":64,"progress_text":"64/1000","can_reward":true,"monthly_count":0,"monthly_limit":1}]}`))
		case strings.Contains(q, "action=my_items"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":7,"code":"bigsmall_loss_refund","name":"亏返卡","quantity":2,"usable_in_bet":false}]}}`))
		case strings.Contains(q, "action=chat_get"):
			_, _ = w.Write([]byte(`{"code":0,"data":[{"id":1,"username":"alice","message":"hi","time":"12:00"}]}`))
		}
	}))
	defer srv.Close()
	st, err := HxptBigSmallGame{}.BigSmallState(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if st.Balance != 1000 || st.MinBet != 100 || !st.HasHouse || st.Currency != "火花" {
		t.Errorf("basics = %+v", st)
	}
	if st.Round.ID != 42 || st.Round.Status != "betting" || st.Round.Countdown != 35 || st.Round.SealCountdown != 30 {
		t.Errorf("round = %+v", st.Round)
	}
	if st.MyBet == nil || st.MyBet.BetType != "big" || st.MyBet.Amount != 500 {
		t.Errorf("mybet = %+v", st.MyBet)
	}
	if st.LastResult == nil || st.LastResult.NetResult != 475 || st.LastResult.Result != "big" {
		t.Errorf("last = %+v", st.LastResult)
	}
	if len(st.BigBets) != 1 || st.BigBets[0].Username != "alice" {
		t.Errorf("bigbets = %+v", st.BigBets)
	}
	if st.Trend40.BigCount != 21 || st.Trend40.HouseCount != 2 || st.Trend40.BigPct != 53 {
		t.Errorf("trend40 = %+v", st.Trend40)
	}
	if len(st.RecentResults) != 4 || st.RecentResults[2] != "house" {
		t.Errorf("recent = %+v", st.RecentResults)
	}
	if len(st.Rankings) == 0 || st.Rankings[0].Title != "本月赌神" || st.Rankings[0].Entries[0].Username != "alice" {
		t.Errorf("rankings = %+v", st.Rankings)
	}
	if len(st.MyRecords) != 1 || st.MyRecords[0].MyResult != 475 {
		t.Errorf("myrec = %+v", st.MyRecords)
	}
	if len(st.AllRecords) != 1 || !strings.Contains(st.AllRecords[0].Bets, "alice") {
		t.Errorf("allrec = %+v", st.AllRecords)
	}
	if len(st.Tasks) != 1 || st.Tasks[0].Status != "rewardable" {
		t.Errorf("tasks = %+v", st.Tasks)
	}
	if len(st.Props) != 1 || st.Props[0].Tag != "bigsmall_loss_refund" {
		t.Errorf("props = %+v", st.Props)
	}
	if len(st.Chat) != 1 || st.Chat[0].Message != "hi" {
		t.Errorf("chat = %+v", st.Chat)
	}
}

// ── 熊猫猜大小 ──

func TestPandaBigSmallState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		body := string(buf[:n])
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(body, "action=reloadData"):
			_, _ = w.Write([]byte(`{"ret":0,"data":{"my_bonus":617364,"max_bonus":1000000,"isBettingOpen":true,"timeLeft":35,"currentGame":{"id":465766,"join":"大 : 1,000"},"last_result":[[1,2,1],[2,1,1]],"my_list":[],"other_list":[]}}`))
		case strings.Contains(body, "action=get_current_users"):
			_, _ = w.Write([]byte(`{"ret":0,"data":{"big":[{"username":"alice","amount":1000}],"small":[]}}`))
		case strings.Contains(body, "action=task_list"):
			_, _ = w.Write([]byte(`{"ret":0,"data":[{"task_id":3,"title":"反悔卡","task_status":1}]}`))
		case strings.Contains(body, "action=getMyItems"):
			_, _ = w.Write([]byte(`{"ret":0,"data":[{"unique_tag":"lose_is_win","title":"反悔卡","count":1}]}`))
		case strings.Contains(body, "action=getActiveRedPackets"):
			_, _ = w.Write([]byte(`{"ret":0,"data":[{"baseId":"r1","count":5,"total":1000}]}`))
		case strings.Contains(body, "action=getLastMsg"):
			_, _ = w.Write([]byte(`{"ret":0,"data":[{"id":1,"username":"alice","content":"hello","timestamp":"12:00"}]}`))
		}
	}))
	defer srv.Close()
	st, err := PandaBigSmallGame{}.BigSmallState(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if st.Balance != 617364 || st.MinBet != 100 || st.HasHouse || st.Payout != "1.95" || st.Currency != "魔力值" {
		t.Errorf("basics = %+v", st)
	}
	if st.Round.Status != "betting" || st.Round.Countdown != 35 || st.Round.SealCountdown != 32 || st.Round.ID != 465766 {
		t.Errorf("round = %+v", st.Round)
	}
	if st.MyBet == nil || st.MyBet.BetType != "big" || st.MyBet.Amount != 1000 {
		t.Errorf("mybet = %+v", st.MyBet)
	}
	// last_result [[1,2,1],[2,1,1]] → flat [1,2,1,2,1,1] → big=4/small=2/total=6
	if st.Trend40.BigCount != 4 || st.Trend40.SmallCount != 2 || st.Trend40.BigPct != 66 {
		t.Errorf("trend40 = %+v", st.Trend40)
	}
	if len(st.RecentResults) != 6 || st.RecentResults[0] != "big" || st.RecentResults[1] != "small" {
		t.Errorf("recent = %+v", st.RecentResults)
	}
	if len(st.BigBets) != 1 || st.BigBets[0].Username != "alice" {
		t.Errorf("bigbets = %+v", st.BigBets)
	}
	if len(st.Tasks) != 1 || st.Tasks[0].ID != 3 || st.Tasks[0].Status != "doing" {
		t.Errorf("tasks = %+v", st.Tasks)
	}
	if len(st.Props) != 1 || st.Props[0].Tag != "lose_is_win" {
		t.Errorf("props = %+v", st.Props)
	}
	if len(st.Redpackets) != 1 || st.Redpackets[0].ID != "r1" || st.Redpackets[0].Total != 1000 {
		t.Errorf("red = %+v", st.Redpackets)
	}
	if len(st.Chat) != 1 || st.Chat[0].Message != "hello" {
		t.Errorf("chat = %+v", st.Chat)
	}
}

// 熊猫 saveBet：selection=small→type=2，id 取自 reloadData.currentGame.id。
func TestPandaBigSmallBet(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 512)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(gotBody, "action=reloadData") {
			_, _ = w.Write([]byte(`{"ret":0,"data":{"currentGame":{"id":465766}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"ret":0,"data":{"my_bonus":616364},"msg":"下注成功"}`))
	}))
	defer srv.Close()
	res, err := PandaBigSmallGame{}.Bet(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"}, BetRequest{Selection: "small", Amount: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.Balance != 616364 {
		t.Errorf("panda bet = %+v", res)
	}
	if !strings.Contains(gotBody, "action=saveBet") || !strings.Contains(gotBody, "params[type]=2") || !strings.Contains(gotBody, "params[id]=465766") || !strings.Contains(gotBody, "params[amount]=1000") {
		t.Errorf("saveBet body = %q", gotBody)
	}
}

func TestPandaBigSmallUseItem(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 512)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ret":0,"data":{"status":1,"msg":"已使用"}}`))
	}))
	defer srv.Close()
	res, err := PandaBigSmallGame{}.BigSmallUseItem(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"}, "lose_is_win")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Errorf("useItem = %+v", res)
	}
	if !strings.Contains(gotBody, "action=useItem") || !strings.Contains(gotBody, "params[unique_tag]=lose_is_win") {
		t.Errorf("useItem body = %q", gotBody)
	}
}

func TestPandaBigSmallRedpacket(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 512)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ret":0,"data":{"new_bonus":617364,"amount":200},"msg":"抢到"}`))
	}))
	defer srv.Close()
	res, err := PandaBigSmallGame{}.BigSmallRedpacket(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"}, "claim", map[string]string{"id": "r1"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.Balance != 617364 {
		t.Errorf("redpacket = %+v", res)
	}
	if !strings.Contains(gotBody, "action=claimRedPacket") || !strings.Contains(gotBody, "params[baseId]=r1") {
		t.Errorf("redpacket body = %q", gotBody)
	}
}

// ── OurBits 赛事押注 ── 契约来自真实 ourbits.club 抓取（bet.php / bet_odds.php?id=945 / bet_odds2.php）。

// ourbitsBetListHTML 模拟 bet.php 列表真实结构：表头缺 </tr> 闭合，数据行 <tr><tr> 双 tr 单 </tr>。
const ourbitsBetListHTML = `<html><body>
魔力值 [ 使用 ]: 372,140.3
<table><tbody>
<tr><td class="colhead">竞猜主题</td><td class="colhead">结束时间</td><td class="colhead">庄家</td><td class="colhead">下注总数</td><td class="colhead">投注总额</td><td class="colhead">我的下注</td><td class="colhead">状态</td><tr><tr> <td class='nowrap'><a href='./bet_gameinfo.php?showgames=945'><b>2026年美加墨世界杯冠军</b></a></td> <td class='nowrap'><b>2026-07-12 00:00:00</b>. 剩余: <b>10天07:10:37</b></td> <td class='nowrap' align='center'><a href="userdetails.php?id=2725"><b>dongya</b></a></td> <td class='nowrap' align='center'>30</td> <td class='nowrap' align='center'>1095182</td> <td class='nowrap' align='center'>-</td> <td class='nowrap' align='center'><a href='/bet_odds.php?id=945'><b>点击押注</b></a></td></tr>
</tbody></table></body></html>`

// ourbitsBetOddsHTML 模拟 bet_odds.php?id=945 盘口（真实选项行：<td>队名</td><td>赔率</td><td>form bonus+hidden id</td><td>已押</td>）。
const ourbitsBetOddsHTML = `<html><body>
<h1>博彩频道</h1><h1>2026年美加墨世界杯冠军</h1><p>如题</p>
<table cellpadding='5'>
<tr><td>选项</td><td>倍率</td><td>押注</td><td align='center'>已押注</td></tr>
<tr> <td>佛得角</td> <td>10951.82</td><td><form method='post' action='bet_odds2.php'> <input type='text' name='bonus' size='6' maxlength='10' /> <b> 魔力值</b> <input type='hidden' name='id' value='3223' /><input type='submit' value='押注' /></form></td><td align='center'>----</td></tr>
<tr> <td>阿根廷</td> <td>2.09</td><td><form method='post' action='bet_odds2.php'> <input type='text' name='bonus' size='6' maxlength='10' /> <b> 魔力值</b> <input type='hidden' name='id' value='3194' /><input type='submit' value='押注' /></form></td><td align='center'>----</td></tr>
</table></body></html>`

func TestOurbitsBetParseEvents(t *testing.T) {
	events := ourbitsBetParseEvents(ourbitsBetListHTML)
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1: %+v", len(events), events)
	}
	ev := events[0]
	if ev.ID != 945 || ev.Title != "2026年美加墨世界杯冠军" {
		t.Errorf("event id/title = %+v", ev)
	}
	if ev.EndTime != "2026-07-12 00:00:00" || ev.Remain != "10天07:10:37" {
		t.Errorf("event time = %+v", ev)
	}
	if ev.Bookmaker != "dongya" || ev.BetCount != 30 || ev.TotalStake != 1095182 {
		t.Errorf("event stats = %+v", ev)
	}
	if ev.MyStake != "-" || ev.Status != "open" {
		t.Errorf("event mystake/status = %+v", ev)
	}
	// 余额解析（千分位逗号）
	if b := ourbitsBetBalance(ourbitsBetListHTML); b != 372140.3 {
		t.Errorf("balance = %v, want 372140.3", b)
	}
}

func TestOurbitsBetParseDetail(t *testing.T) {
	d := ourbitsBetParseDetail(ourbitsBetOddsHTML, 945)
	if d.Event.Title != "2026年美加墨世界杯冠军" {
		t.Errorf("detail title = %q", d.Event.Title)
	}
	if len(d.Options) != 2 {
		t.Fatalf("options len = %d, want 2: %+v", len(d.Options), d.Options)
	}
	// 选项按页面顺序：佛得角(冷门高赔 10951.82) / 阿根廷(热门低赔 2.09)
	if d.Options[0].Name != "佛得角" || d.Options[0].ID != "3223" || d.Options[0].Odds != 10951.82 {
		t.Errorf("option[0] = %+v", d.Options[0])
	}
	if d.Options[1].Name != "阿根廷" || d.Options[1].ID != "3194" || d.Options[1].Odds != 2.09 {
		t.Errorf("option[1] = %+v", d.Options[1])
	}
}

func TestOurbitsBetEventDetailMerge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch {
		case strings.Contains(r.URL.Path, "bet_odds.php"):
			_, _ = w.Write([]byte(ourbitsBetOddsHTML))
		case strings.HasSuffix(r.URL.Path, "bet.php"):
			_, _ = w.Write([]byte(ourbitsBetListHTML))
		}
	}))
	defer srv.Close()
	d, err := OurbitsBetGame{}.EventDetail(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"}, 945)
	if err != nil {
		t.Fatal(err)
	}
	// 主题名+选项来自 bet_odds；截止时间/剩余/统计来自合并的 bet.php 列表。
	if d.Event.Title != "2026年美加墨世界杯冠军" || len(d.Options) != 2 {
		t.Errorf("title/options = %q / %d", d.Event.Title, len(d.Options))
	}
	if d.Event.EndTime != "2026-07-12 00:00:00" || d.Event.Remain != "10天07:10:37" {
		t.Errorf("merged endtime/remain = %+v", d.Event)
	}
	if d.Event.Bookmaker != "dongya" || d.Event.TotalStake != 1095182 {
		t.Errorf("merged stats = %+v", d.Event)
	}
}

func TestOurbitsBetBet(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 256)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html>魔力值 [ 使用 ]: 371,140.3<font class='em'>押注成功，等待开奖</font></html>`))
	}))
	defer srv.Close()
	res, err := OurbitsBetGame{}.Bet(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"}, BetRequest{Selection: "3194", Amount: 1000})
	if err != nil {
		t.Fatal(err)
	}
	// POST body: bonus=1000&id=3194（id 是选项 id 非主题 id；无 csrf）
	if !strings.Contains(gotBody, "bonus=1000") || !strings.Contains(gotBody, "id=3194") {
		t.Errorf("bet body = %q", gotBody)
	}
	if !res.Success {
		t.Errorf("bet should succeed: %+v", res)
	}
	if res.Balance != 371140.3 {
		t.Errorf("balance = %v, want 371140.3", res.Balance)
	}
	// 缺选项 id / 非正金额应前置失败（不请求站点）
	g := OurbitsBetGame{}
	if bad, _ := g.Bet(context.Background(), newTestClient(t), srv.URL, Creds{}, BetRequest{Amount: 100}); bad.Success {
		t.Error("bet without selection should fail")
	}
	if bad, _ := g.Bet(context.Background(), newTestClient(t), srv.URL, Creds{}, BetRequest{Selection: "3194"}); bad.Success {
		t.Error("bet with non-positive amount should fail")
	}
}

func TestOurbitsBetHistory(t *testing.T) {
	// 无押注：页面显示「你还没有参加」
	if r := ourbitsBetParseHistory(`<html>你还没有参加</html>`); len(r) != 0 {
		t.Errorf("empty history len = %d, want 0", len(r))
	}
	// 有押注（bet_coupon 表头：押注选项/倍率/押注金额/可获金额/状态；结构待真实有数据校准）。
	const coupon = `<table><tbody>
<tr><td>押注选项</td><td>倍率</td><td>押注金额</td><td>可获金额</td><td>状态</td></tr>
<tr><td>阿根廷</td><td>2.09</td><td>1000</td><td>2090</td><td>进行中</td></tr>
</tbody></table>`
	r := ourbitsBetParseHistory(coupon)
	if len(r) != 1 || r[0].PrizeName != "阿根廷" || r[0].Cost != 1000 || r[0].Amount != 2090 || r[0].CreatedAt != "进行中" {
		t.Errorf("history = %+v", r)
	}
}

func TestUbitsRedpacketBet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><div>领取成功</div><span>U币: 8888.0</span></html>`))
	}))
	defer srv.Close()
	res, err := UbitsRedpacketGame{}.Bet(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"}, BetRequest{Selection: "claim"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.Balance != 8888 {
		t.Errorf("ubits_redpacket = %+v", res)
	}
}
