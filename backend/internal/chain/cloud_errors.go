package chain

import "errors"

// 云盘操作出口（云端整理 / STRM 修复 / 115 开放平台授权）里「重试永远不会有用」的两档。
// 从前这四条出口把一切都压成 400「请求参数有误，请检查后重试」——
// 而 115 的 auth/poll 是纯上游调用、organize 缺的是媒体识别配置，用户都没有输入可检查。
var (
	// ErrCloudMediaSearchUnconfigured 是「整理功能依赖的媒体搜索没注入/没配好」：
	// 该去配 TMDB/媒体源，而不是改整理参数。
	ErrCloudMediaSearchUnconfigured = errors.New("媒体识别服务未配置")
	// ErrCloudTokenUnreadable 是「这台网盘配过令牌、但现在解不开」（主密钥换过 / 密文损坏）。
	// 与站点 Cookie 那一档同形（#277）：Decrypt("") 回 ("", nil)，所以非 nil 必是真故障；
	// 不拦住的话会拿着空令牌打云盘，整理/修复这类会写云盘的操作可能改坏一半却说「参数有误」。
	ErrCloudTokenUnreadable = errors.New("云盘令牌读不出来")
)
