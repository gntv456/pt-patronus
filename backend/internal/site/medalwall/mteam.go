// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package medalwall

import "context"

// MTeamAdapter 馒头（MTeam）勋章墙占位：MTeam 走 JSON API，无可用勋章商店页。
// Supported=false 仅用于阻止 Generic/NexusPHP 适配器对 MTeam API 主机请求 /medal.php 触发 403 噪声。
type MTeamAdapter struct{}

func (MTeamAdapter) Platform() string { return "mteam" }
func (MTeamAdapter) Supported() bool  { return false }

func (MTeamAdapter) FetchShop(context.Context, HTTPDoer, Def, string, string) ([]Item, error) {
	return nil, nil
}
func (MTeamAdapter) FetchOwned(context.Context, HTTPDoer, Def, string, string, string) ([]Owned, error) {
	return nil, nil
}
func (MTeamAdapter) Buy(context.Context, HTTPDoer, Def, string, string, string) (ActionResult, error) {
	return ActionResult{Success: false, Message: "馒头暂不支持勋章购买"}, nil
}
func (MTeamAdapter) Wear(context.Context, HTTPDoer, Def, string, string, string) (ActionResult, error) {
	return ActionResult{Success: false, Message: "馒头暂不支持勋章佩戴"}, nil
}
func (MTeamAdapter) TakeOff(context.Context, HTTPDoer, Def, string, string, string) (ActionResult, error) {
	return ActionResult{Success: false, Message: "馒头暂不支持勋章摘下"}, nil
}
