# 蓝影论坛（hdblue.cc）适配笔记

> 2026-09-06 实测（登录用户 gntv / userId 1938）。改版后为 Next.js 自建论坛（rhex-theme），
> 非 NexusPHP；无 torrent 功能，纯论坛 + 游戏娱乐站。

## 认证

- `bbs_session`（HttpOnly，HMAC 签名，**内嵌签发 IP，服务端校验**）+ `_t`（长 token，单独不识别用户）
- IP 变化即失效（签名含 ip 字段，防篡改）；后端须与登录浏览器同出口 IP
- BaoziPT OAuth 登录入口：`/api/addons/baozipt-oauth/start?mode=login`

## 用户数据（RSC 协议：GET + `RSC: 1` 头，text/x-component 流内嵌 JSON）

| 路径 | 数据 |
|---|---|
| `/users/<name>` | 影币余额、等级（Lv.1 旅人）、加入日期、帖子/回复/获赞/粉丝、五维属性(财富/阅历/勤勉/技艺/魅力/活跃) |
| `/funs/gobang` | `initialSummary{pointName:"影币"(站内显示积分),points,freeTotal/Used/Remaining,paidTotal/Used/Remaining,challengeStatus}` |
| `/funs/yinyang-contract` | `initialData.summary{points,winCount,loseCount,todayProfitPoints,todayLossPoints,totalProfit/Loss,dailyCreateLimit:5,dailyAcceptLimit:10,createdToday,acceptedToday}` + `openChallenges[]` + 排行榜 |

注意：首页 RSC 的 `session.isAuthenticated` 恒 false（布局分层缓存），登录态判定用游戏页 RSC 的 `userId`/`canPlay`。

## 五子棋 `/funs/gobang`（app.gobang 托管应用）

规则：15×15 无禁手；免费 1 局/日（VIP 3），每日上限普通 3 / VIP 5；免费胜 +20 影币；
付费 10 影币/局、胜返 30；平局判玩家胜；玩家/AI 随机先手；悔棋可用。

API（JSON，cookie 会话）：
```
POST /api/gobang {"action":"create"}                      → 开局（免费优先）
POST /api/gobang {"action":"move","matchId","x","y"}      → 落子（0-based）+AI 回应
POST /api/gobang {"action":"undo",...}                    → 悔棋（按钮存在，未抓包确认字段）
响应 data.match{id,board[15][15](0空/1玩家/2AI),moves[],currentSide,status,winnerId,firstHand}
```

## 阴阳契 `/funs/yinyang-contract`

规则：出题（问题≤120字 + 答案A/B≤40字 + 正确答案）押注 10-500 影币；对方选答案，
选对赢彩头（税率 10%）；日发起限 5 / 接受限 10；赢家榜/赚分榜/每日阴阳王。

API：
```
POST /api/yinyang-contract {"action":"create","question","optionA","optionB","correctOption":"A|B","stakePoints"}
POST /api/yinyang-contract {"action":"accept","challengeId","selectedOption":"A|B"}   ← 2026-09-06 实测
→ accept 立即结算；影币不足返回 409 {"code":409,"message":"积分不足，无法应战"}
```

## 遗留确认（2026-09-06 下午补测）

- ✅ accept 已实测（challengeId+selectedOption；选错即输，选对赢 rewardPoints=stake-税）
- ❌ 五子棋 undo：服务端未实现——按钮灰态；undo/retract/takeback/rollback/regret 全返"不支持的操作"（400）。Play 自动弈局里不做悔棋。

## 遗留

- [ ] 阴阳契 accept/cancel 字段确认（发起的测试契约 id=2f7ca79c… 10 影币，页面无取消按钮，等对方接或过期）
- [ ] undo 字段确认
