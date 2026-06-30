package entertainment

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// LongPT 六十四卦勋章（/plugins/medals）：爻碎片合成 + 金字塔升级系统。
//
// 玩法（契约来自站点 SPA bundle，非源码）：
//   - 5 层金字塔：太极(1) → 两仪(2) → 四象(4) → 八卦(8) → 六十四卦(64)，靠 parentMedalId 串成树。
//   - 太极/两仪/四象：消耗阳爻/阴爻碎片合成（mergeUserMedal）。medalId 是 0/1 二进制串，
//     "1"位=阳爻、"0"位=阴爻，所需数 = medalId 各位计数（位数=该层爻数）。
//   - 八卦/六十四卦：消耗魔力 + 全部子勋章合成更高一阶（mergeUserBigMedal），魔力 = 200000 * 2^(4-level)。
//   - 阴阳转换：花 10000 魔力把 1 个阳爻转阴爻（yiYang=-1）或阴爻转阳爻（yiYang=1）。
//
// 端点分两组前缀：getList 在 /plugins/medals/，写操作在 /nexus/。纯 Cookie 鉴权，{code,msg,data} 信封。
// medalId 是大整数（卦象），全程用 json.Number 保留精度，合成时原样数字回传。

// LongPTMedalGame 六十四卦勋章（非抽奖，Play 仅引导；实际走 MedalComposable 各端点）。
type LongPTMedalGame struct{}

func (LongPTMedalGame) ID() string   { return "longpt_medal" }
func (LongPTMedalGame) Name() string { return "六十四卦勋章" }

// Play 兜底（六十四卦无单次抽奖概念，引导用专属合成操作）。
func (LongPTMedalGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	return &PlayResult{Success: false, Message: "请选择勋章进行合成"}, nil
}

// MedalTree 勋章金字塔（pluginsMedalInfo/getList，data 是数组）。消耗由后端按 medalId/level 算好供前端展示。
func (LongPTMedalGame) MedalTree(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]MedalNode, error) {
	data, code, msg, err := longptPost(ctx, hc, baseURL, "plugins/medals/pluginsMedalInfo/getList",
		map[string]any{"medalUnit": "3"}, cr.Cookie)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("%s", msg)
	}
	rawItems, err := longptAsList(data)
	if err != nil {
		return nil, fmt.Errorf("parse medal tree: %w", err)
	}
	out := make([]MedalNode, 0, len(rawItems))
	for _, raw := range rawItems {
		var it struct {
			MedalID       json.Number `json:"medalId"`
			ParentMedalID json.Number `json:"parentMedalId"`
			MedalType     string      `json:"medalType"`
			MedalName     string      `json:"medalName"`
			MedalLevel    int         `json:"medalLevel"`
			MedalImageURL string      `json:"medalImageUrl"`
			Owned         bool        `json:"owned"`
		}
		if json.Unmarshal(raw, &it) != nil {
			continue
		}
		id := it.MedalID.String()
		yang, yin := countYao(id)
		out = append(out, MedalNode{
			MedalID:  id,
			ParentID: it.ParentMedalID.String(),
			Type:     it.MedalType,
			Name:     it.MedalName,
			Level:    it.MedalLevel,
			ImageURL: it.MedalImageURL,
			Owned:    it.Owned,
			NeedYang: yang,
			NeedYin:  yin,
			BigCost:  longptBigCost(it.MedalLevel),
		})
	}
	return out, nil
}

// MedalShards 阳爻/阴爻碎片余额（getUserMedalShardInfo）。
func (LongPTMedalGame) MedalShards(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*MedalShardInfo, error) {
	data, code, msg, err := longptPost(ctx, hc, baseURL, "nexus/pluginsUserMedalShard/getUserMedalShardInfo",
		map[string]any{}, cr.Cookie)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("%s", msg)
	}
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	return &MedalShardInfo{
		YangYao: int(numField(m, "yangYao")),
		YinYao:  int(numField(m, "yinYao")),
	}, nil
}

// ComposeMedal 碎片合成（太极/两仪/四象，mergeUserMedal）。
func (LongPTMedalGame) ComposeMedal(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, medalID string) (*PlayResult, error) {
	return longptMedalAction(ctx, hc, baseURL, "nexus/pluginsUserMedalInfo/mergeUserMedal", medalID, cr.Cookie, "合成成功")
}

// ComposeBigMedal 大勋章合成（八卦/六十四卦，mergeUserBigMedal，耗魔力+子勋章）。
func (LongPTMedalGame) ComposeBigMedal(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, medalID string) (*PlayResult, error) {
	return longptMedalAction(ctx, hc, baseURL, "nexus/pluginsUserMedalInfo/mergeUserBigMedal", medalID, cr.Cookie, "大勋章合成成功")
}

// longptMedalAction 通用合成 POST {medalId}（json.Number 保持数字形式回传）。
func longptMedalAction(ctx context.Context, hc *httpclient.Client, baseURL, path, medalID, cookie, okMsg string) (*PlayResult, error) {
	_, code, msg, err := longptPost(ctx, hc, baseURL, path,
		map[string]json.Number{"medalId": json.Number(medalID)}, cookie)
	if err != nil {
		return &PlayResult{Success: false, Message: "站点响应异常，可能未登录或 cookie 已失效"}, nil
	}
	if code != 0 {
		return &PlayResult{Success: false, Message: msg}, nil
	}
	res := &PlayResult{Success: true, Message: msg}
	if res.Message == "" {
		res.Message = okMsg
	}
	return res, nil
}

// ConvertShard 阴阳转换（convertYinYang?yiYang=1|-1）。yiYang>0 阴→阳，<0 阳→阴（10000 魔力 1:1）。
func (LongPTMedalGame) ConvertShard(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, yiYang int) (*PlayResult, error) {
	q := "1"
	if yiYang < 0 {
		q = "-1"
	}
	_, code, msg, err := longptGet(ctx, hc, baseURL, "nexus/pluginsUserMedalShard/convertYinYang", "yiYang="+q, cr.Cookie)
	if err != nil {
		return &PlayResult{Success: false, Message: "站点响应异常，可能未登录或 cookie 已失效"}, nil
	}
	if code != 0 {
		return &PlayResult{Success: false, Message: msg}, nil
	}
	res := &PlayResult{Success: true, Message: msg}
	if res.Message == "" {
		if yiYang < 0 {
			res.Message = "已将 1 个阳爻转换为阴爻（消耗 10000 魔力）"
		} else {
			res.Message = "已将 1 个阴爻转换为阳爻（消耗 10000 魔力）"
		}
	}
	return res, nil
}

// countYao 数 medalId 二进制串的阳爻("1")/阴爻("0")个数。
func countYao(id string) (yang, yin int) {
	for _, c := range id {
		switch c {
		case '1':
			yang++
		case '0':
			yin++
		}
	}
	return
}

// longptBigCost 大勋章合成魔力消耗：200000 * 2^(4-level)，仅 level>=4（八卦/六十四卦）；碎片合成层为 0。
func longptBigCost(level int) int {
	if level < 4 {
		return 0
	}
	return 200000 >> (level - 4)
}
