// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package api

import (
	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/api/v1"
)

// registerEntertainmentRoutes 注册娱乐游戏路由（翻面夺宝 WS 桥接注册在 api 上绕过 header 鉴权）。
// ENTERTAINMENT_ENABLED=false（默认）时整组不注册——游戏自动化可能违反站规，
// 用户显式开启才暴露（风险自担，见 config.example.yaml 告知）。
func registerEntertainmentRoutes(api, guarded *gin.RouterGroup, d *Deps) {
	if !d.Cfg.EntertainmentEnabled {
		// 整组不注册时，前端菜单/页面照样会往 /entertainment 打请求，只会拿到通用
		// 404「请求的资源不存在」——那是「路径不存在」的意思，不是「功能被关掉」。
		// 这里挂一个说明性入口，把默认关闭与下一步（开环境变量）说清楚。
		disabled := func(c *gin.Context) { errcode.Respond(c, errcode.EntertainmentDisabled) }
		guarded.Any("/entertainment", disabled)
		guarded.Any("/entertainment/*path", disabled)
		return
	}
	entH := v1.NewEntertainmentHandler(d.Entertainment, d.Tickets, splitOrigins(d.Cfg.CORSAllowedOrigins))
	ent := guarded.Group("/entertainment")
	{
		ent.GET("", entH.Sites)
		ent.GET("/overview", entH.Overview)
		ent.GET("/:id/games", entH.Games)
		ent.POST("/:id/play", entH.Play)
		ent.GET("/:id/games/:game/stats", entH.Stats)
		ent.GET("/:id/games/:game/buy-options", entH.BuyOptions)
		ent.POST("/:id/games/:game/buy", entH.Buy)
		ent.POST("/:id/games/:game/play-multi", entH.PlayMulti)
		ent.GET("/:id/games/:game/prizes", entH.Prizes)
		ent.GET("/:id/games/:game/history", entH.History)
		ent.GET("/:id/games/:game/syntheses", entH.Syntheses)
		ent.POST("/:id/games/:game/draw", entH.Draw)
		ent.POST("/:id/games/:game/synthesize", entH.Synthesize)
		ent.GET("/:id/games/:game/medal-tree", entH.MedalTree)
		ent.GET("/:id/games/:game/medal-shards", entH.MedalShards)
		ent.POST("/:id/games/:game/compose", entH.ComposeMedal)
		ent.POST("/:id/games/:game/compose-big", entH.ComposeBigMedal)
		ent.POST("/:id/games/:game/convert-shard", entH.ConvertShard)
		ent.GET("/:id/games/:game/farm", entH.Farm)
		ent.POST("/:id/games/:game/plant", entH.Plant)
		ent.POST("/:id/games/:game/harvest", entH.Harvest)
		ent.POST("/:id/games/:game/harvest-all", entH.HarvestAll)
		ent.POST("/:id/games/:game/sell", entH.Sell)
		ent.POST("/:id/games/:game/bet", entH.Bet)
		ent.GET("/:id/games/:game/events", entH.Events)
		ent.GET("/:id/games/:game/events/:eventID", entH.EventDetail)
		ent.POST("/:id/games/:game/blackjack", entH.Blackjack)
		ent.GET("/:id/games/:game/flip-state", entH.FlipState)
		ent.GET("/:id/games/:game/bigsmall-state", entH.BigSmallState)
		ent.GET("/:id/games/:game/bigsmall-chat", entH.BigSmallChat)
		ent.POST("/:id/games/:game/bigsmall-task", entH.BigSmallTaskAction)
		ent.POST("/:id/games/:game/bigsmall-item", entH.BigSmallUseItem)
		ent.POST("/:id/games/:game/bigsmall-chat", entH.BigSmallSendChat)
		ent.POST("/:id/games/:game/bigsmall-redpacket", entH.BigSmallRedpacket)
		// 五子棋（蜂巢）：全景快照 + 动作（create/choose-color/move/undo/forfeit/retry-ai）
		ent.GET("/:id/games/:game/gobang-state", entH.GobangState)
		ent.POST("/:id/games/:game/gobang-hint", entH.GobangHint)
		ent.POST("/:id/games/:game/gobang-action", entH.GobangAction)
		// 好友买卖（PANDA）全景快照；钓鱼（思齐）全景快照（批8 富 UI）
		ent.GET("/:id/games/:game/friendtrade-state", entH.FriendTradeState)
		ent.GET("/:id/games/:game/fishing-state", entH.FishingState)
		// 躺平抽奖全景（批14 富 UI）
		ent.GET("/:id/games/:game/tangpt-lottery-state", entH.TangptLotteryState)
		// 藏宝阁大转盘/老虎机全景（批13 富 UI）
		ent.GET("/:id/games/:game/cbg-wheel-state", entH.CbgWheelState)
		ent.GET("/:id/games/:game/cbg-slot-state", entH.CbgSlotState)
		// 13City 大转盘/幸运骰子全景（批12 富 UI）
		ent.GET("/:id/games/:game/city13-wheel-state", entH.City13WheelState)
		ent.GET("/:id/games/:game/city13-dice-state", entH.City13DiceState)
		// 翻面夺宝 websocket 桥接：?ticket= 鉴权（浏览器 WS 不能发 header JWT），
		// 注册在 api（非 guarded）绕过 header 鉴权，handler 内自验 ticket。
		api.GET("/entertainment/:id/games/:game/flip-ws", entH.FlipWS)
	}
}
