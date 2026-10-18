package plugins

import "errors"

// 市场安装失败的语义分档。
//
// 这些哨兵只承担「哪一档失败」的判定：文案自带中文头一句，细节由抛出点用
// fmt.Errorf("%w：细节") 追加，接口层据此挑 HTTP 状态码。从前安装失败不分青红皂白
// 被报成 400「请求参数有误」——市场源挂了、包被改过、宿主不兼容都不是用户填错表，
// 指错方向会让用户反复重试同一坏包，也把篡改提示降级成了手误。
var (
	// ErrMarketUnreachable 市场索引或插件归档取不到（网络、上游非 2xx、超限）。
	ErrMarketUnreachable = errors.New("市场源暂时取不到")
	// ErrPluginNotInMarket 索引里没有这个插件 id（多半是列表已刷新、该插件被下架）。
	ErrPluginNotInMarket = errors.New("市场里没有这个插件")
	// ErrPluginNotInstallable 包是好的，但当前宿主装不了（不兼容、缺能力、安装计划自相矛盾）。
	ErrPluginNotInstallable = errors.New("这个插件在当前主机上装不了")
	// ErrUntrustedPackage 包内容与市场声明/签名不一致：可能被篡改，也可能源与包版本错位。
	ErrUntrustedPackage = errors.New("插件包校验不通过")
	// ErrPackageInvalid 包解压或 manifest 解析/校验不过（损坏或不符合插件规范）。
	ErrPackageInvalid = errors.New("插件包读不出有效内容")
	// ErrPermissionsUnconfirmed 调用方没带上与该插件权限集一致的确认——这确实是请求侧问题。
	ErrPermissionsUnconfirmed = errors.New("安装前需要确认该插件申请的权限")
	// ErrInstallWriteFailed 落盘、载入、发布快照等本机动作失败。
	ErrInstallWriteFailed = errors.New("插件写入本机失败")
)
