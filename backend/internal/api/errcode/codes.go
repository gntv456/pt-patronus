// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package errcode

import (
	"net/http"

	"github.com/hxpt/ptpatronus/internal/errtext"
)

// 预定义错误码唯一注册表：新增码必须先进这里（codes_test.go 校验唯一性）。
// 命名 <domain>_<reason>；HTTP 状态与语义对齐（4xx 客户端可恢复 / 5xx 服务端故障）。
//
// 第一批收录 api/v1 既有 19 处裸 code 字符串（PR-1 收口），后续域随 PR-2+ 扩充。
var (
	// —— 通用 ——
	Internal     = New("internal", "服务内部错误，请稍后重试", http.StatusInternalServerError)
	InvalidInput = New("invalid_input", "请求有参数后端无法接受，但没能指出具体是哪一项：对照接口文档核对必填项，或查看后端日志里记录的具体字段", http.StatusBadRequest)
	NotFound     = New("not_found", "请求的资源不存在，可能已被删除，请刷新页面后重试", http.StatusNotFound)
	BadGateway   = New("bad_gateway", "上游服务不可用，请稍后重试", http.StatusBadGateway)
	Conflict     = New("conflict", "资源冲突，请刷新后重试", http.StatusConflict)
	Unavailable  = New("unavailable", "服务暂不可用，请稍后重试", http.StatusServiceUnavailable)

	// —— 鉴权 / 会话 ——
	// 下列 VIP* 码为好学时代遗留（闸门已退役，生产不再返回），保留变量名兼容旧客户端与测试。
	AdminRequired   = New("admin_required", "需要管理员权限", http.StatusForbidden)
	TokenMismatch   = New("token_version_mismatch", "登录态已更新，请重新登录", http.StatusUnauthorized)
	InvalidToken    = New("invalid_token", "登录态无效，请重新登录", http.StatusUnauthorized)
	NotActivated    = New("not_activated", "实例未激活", http.StatusForbidden)
	NotVIP          = New("not_vip", "无权访问该功能", http.StatusForbidden)
	VIPNotVerified  = New("vip_not_verified", "登录态未完成验证，请重新登录", http.StatusForbidden)
	VIPUnknown      = New("vip_unknown", "登录态验证状态未知，请稍后重试", http.StatusForbidden)
	VIPExpired      = New("vip_expired", "登录态已过期，请重新登录", http.StatusForbidden)
	ChallengeMode   = New("challenge_mode", "该登录方式需要额外验证", http.StatusConflict)
	OIDCDeprecated  = New("deprecated", "该登录方式已停用", http.StatusGone)
	ConfirmRequired = New("confirm_required", "需要先完成确认", http.StatusPreconditionRequired)

	// —— 登录 / 会话（PR-2 认证域） ——
	AuthServiceUnavailable = New("auth_service_unavailable", "登录服务未启用", http.StatusServiceUnavailable)
	AuthServerBusy         = New("auth_server_busy", "服务暂不可用，请稍后重试", http.StatusInternalServerError)
	AuthTicketFailed       = New("auth_ticket_failed", "签发 ticket 失败", http.StatusInternalServerError)
	AuthCaptchaFailed      = New("auth_captcha_failed", "取验证码失败", http.StatusBadGateway)
	AuthBadCredentials     = New("auth_bad_credentials", "用户名/密码/验证码错误", http.StatusUnauthorized)
	// 本地密码登录专属错误。
	AuthPasswordNotSet = New("auth_password_not_set",
		"该账号未设置本地密码：升级实例请在 compose 设 ADMIN_PASSWORD 后重启，或执行 reset-admin --password 初始化", http.StatusConflict)
	AuthWeakPassword = New("auth_weak_password",
		"密码至少 8 位且需包含字母和数字", http.StatusBadRequest)
	AuthCookieRequired = New("auth_cookie_required", "cookie 必填", http.StatusBadRequest)
	DemoDisabled       = New("demo_disabled", "演示登录未启用", http.StatusNotFound)
	AuthUserRejected   = New("auth_user_rejected", "本实例已绑定首个登录账号，不再接受新账号登录", http.StatusForbidden)
	APIKeyNoRefresh    = New("apikey_no_refresh", "API-Key 旁路无需续期", http.StatusBadRequest)
	APIKeyNoLogout     = New("apikey_no_logout", "API-Key 旁路无需登出", http.StatusBadRequest)
	InvalidKey         = New("invalid_key", "key 非法", http.StatusBadRequest)
	ValueTooLarge      = New("value_too_large", "value 超过 64KB 上限", http.StatusBadRequest)
	ValueInvalidJSON   = New("value_invalid_json", "value 必须为合法 JSON", http.StatusBadRequest)
	VIPCheckDisabled   = New("vip_check_disabled", "VIP 校验未启用", http.StatusBadRequest)
	OIDCNotConfigured  = New("oidc_not_configured", "OIDC 未装配", http.StatusServiceUnavailable)
	OIDCStateMismatch  = New("oidc_state_mismatch", "OIDC state 校验失败", http.StatusForbidden)
	// OIDC 发起/回调的两个真实失败处境，此前全被折进 invalid_input「请求参数有误」：
	// 这两个端点一个只有浏览器带来的 cookie，一个只有 Provider 带来的 code/state，
	// 用户手上没有任何「参数」可核 —— 说「参数有误」等于把「SSO 配坏了」说成「你传错了」。
	OIDCDisabled            = New("oidc_disabled", "SSO 未启用或 issuer、client_id 没填全，请到「设置 - OIDC/SSO」补齐", http.StatusServiceUnavailable)
	OIDCProviderUnreachable = New("oidc_provider_unreachable", "连不上 SSO 身份提供方：issuer 地址可能填错、被 SSRF 策略拦下或对方服务不可用", http.StatusBadGateway)
	OIDCLoginFailed         = New("oidc_login_failed", "SSO 登录未完成：与身份提供方交换令牌或读取用户信息失败，请核对 client_id/client_secret 与 issuer", http.StatusBadGateway)
	// MediaCoverNoImage 封面拼图「有条目但一张都没贴上」。不能回一张全黑的 200：
	// 黑图会被读成「这批新片没有海报」，而真实原因是媒体服务器的图片通道不可达/未授权。
	// MediaImageTooLarge 海报代理的体积闸门：超限直接拒，不发「200 + 半张图」。
	MediaImageTooLarge = New("media_image_too_large", "这张图片超过海报代理的 5MB 上限，已拒绝而不是返回半张图：请让浏览器直连源站或改用低画质地址", http.StatusBadGateway)
	MediaCoverNoImage  = New("media_cover_no_image", "封面一张都没取到：条目有封面地址但图片全部取失败，请检查媒体服务器图片接口可达性与凭据", http.StatusBadGateway)

	SaveFailed = New("save_failed", "保存失败：写入服务端配置时出错，可查看后端日志里的具体原因；若提示主密钥相关，请检查 SECRET_KEY 是否被更换", http.StatusInternalServerError)
	// OCR 服务的两种处境分开（Go/os 原文只进日志，界面留人话）：
	// 连不上＝核对端点；答了但读不懂＝多半是识别模式与服务端返回形态不匹配。
	OCRServiceUnreachable = New("ocr_service_unreachable", "连不上 OCR 服务：端点地址或端口可能写错、服务未启动，或本机网络策略拦下了请求；请在「系统设置 → OCR」核对端点后重试", http.StatusBadGateway)
	OCRServiceBadAnswer   = New("ocr_service_bad_answer", "OCR 服务有响应，但状态异常或结果读不出来：多为识别模式（paddle/simple/ddddoc）与服务端返回形态不匹配，请在「系统设置 → OCR」切换模式后重试", http.StatusBadGateway)

	// —— Webhook（PR-2） ——
	WebhookNotFound     = New("webhook_not_found", "Webhook 不存在，可能已被删除，请刷新 Webhook 列表后重新配置", http.StatusNotFound)
	WebhookInvalid      = New("webhook_invalid", "无效的 webhook", http.StatusNotFound)
	WebhookBadPayload   = New("webhook_bad_payload", "读取请求体失败", http.StatusBadRequest)
	WebhookBadSignature = New("webhook_bad_signature", "签名校验失败", http.StatusUnauthorized)
	WebhookNameRequired = New("webhook_name_required", "名称必填", http.StatusBadRequest)
	// 密钥「配了但解不开」（换过 SECRET_KEY / 迁移恢复）与「没配密钥」必须区分：
	// 前者若当后者处理，这个 webhook 就静默变成任何人可触发。
	WebhookSecretUnavailable = New("webhook_secret_unavailable", "该 Webhook 的签名密钥无法解密（可能更换过主密钥），请在 Webhook 设置里重新保存密钥", http.StatusServiceUnavailable)

	// —— 站点 / 下载器（PR-3） ——
	SiteNotFound = New("site_not_found", "站点不存在，可能已被删除，请刷新站点列表后重新选择", http.StatusNotFound)
	// 娱乐域三类失败：游戏没这个能力（换游戏）/ 站点没完成消耗游戏币的操作（先核对余额）/ 站点数据读不出来。
	EntertainmentGameNotFound    = New("entertainment_game_not_found", "这个站点没有注册该游戏（或站点已被删除）：请刷新娱乐页，从可用的游戏里重新选择", http.StatusNotFound)
	EntertainmentGameUnsupported = New("entertainment_game_unsupported", "这个游戏不支持该操作：请换用支持它的游戏或站点", http.StatusConflict)
	EntertainmentSpendFailed     = New("entertainment_spend_failed", "站点没有完成这次消耗游戏币的操作：是否已经扣费以站点页面为准，请先核对余额再决定是否重试", http.StatusBadGateway)
	EntertainmentDataUnreadable  = New("entertainment_data_unreadable", "站点娱乐数据读不出来：这不是参数有误（这一项没有可检查的输入），请稍后重试或到站点页面核对", http.StatusBadGateway)
	// 凭据解不开时链层已拦住、没向站点发请求，所以这里能确定地说「没扣费」（对齐 WebhookSecretUnavailable 的 503）。
	EntertainmentCredentialUnreadable = New("entertainment_credential_unreadable", "该站点的凭据无法解密（可能更换过主密钥），请到「站点」里重新保存凭据；本次未向站点发请求，游戏币未消耗", http.StatusServiceUnavailable)
	SiteInvalidID                     = New("site_invalid_id", "无效的站点 ID", http.StatusBadRequest)
	SiteDuplicate                     = New("site_duplicate", "该站点已存在（同域名站点只能添加一个）", http.StatusConflict)
	DownloaderNotFound                = New("downloader_not_found", "下载器不存在，可能已被删除，请刷新下载器列表后重新选择", http.StatusNotFound)
	DownloaderInvalidInput            = New("downloader_invalid_input", "下载器参数有误", http.StatusBadRequest)
	DownloaderConnFailed              = New("downloader_conn_failed", errtext.DownloaderOffline, http.StatusBadGateway)
	DownloaderPushFailed              = New("download_push_failed", "下发种子失败", http.StatusBadGateway)
	// 站点建在内网/环回时 SSRF 守卫会拦死（默认关闭守卫，有 ALLOW_PRIVATE_SITES 开关）。
	// 这类失败重试永远不会有用，必须给出可操作的下一步，不能只说「服务内部错误」。
	SSRFBlocked = New("ssrf_blocked", errtext.SSRFHint, http.StatusBadRequest)

	// —— 媒体服务器 ——
	MediaServerNotFound = New("mediaserver_not_found", "媒体服务器不存在，可能已被删除，请刷新媒体服务器列表后重新选择", http.StatusNotFound)

	// —— 插件 ——
	// 内置插件的代码在二进制里，磁盘上没有对应目录；点「重载」会撞到 os 的 not-exist，
	// 按 409 说清「内置插件不需要重载」而不是甩一句「参数有误」。
	PluginNotReloadable = New("plugin_not_reloadable", "该插件没有可重载的磁盘代码（内置插件随程序版本更新，只有磁盘/市场安装的插件支持重载）", http.StatusConflict)
	// 市场安装的失败分档：从前安装失败不分原因一律回 400「请求参数有误」，把「市场源挂了」
	// 「包被改过」「宿主不兼容」都说成用户填错表——篡改提示被降级成手误，用户会拿同一个坏包反复重试。
	PluginMarketUnreachable = New("plugin_market_unreachable", "市场源暂时取不到：请检查市场地址是否可访问（设置 → 插件市场）后重试", http.StatusBadGateway)
	PluginNotInMarket       = New("plugin_not_in_market", "市场里没有这个插件，可能已下架：请刷新市场列表后重新选择", http.StatusNotFound)
	PluginNotInstalled      = New("plugin_not_installed", "本机没有安装这个插件，可能已卸载：请刷新插件列表", http.StatusNotFound)
	PluginInstanceNotFound  = New("plugin_instance_not_found", "这个分身实例不存在，可能已被删除：请刷新插件详情", http.StatusNotFound)
	PluginDisabled          = New("plugin_disabled", "该插件当前未启用：请先在插件页把它启用后再执行这个动作", http.StatusConflict)
	PluginConfigUnreadable  = New("plugin_config_unreadable", "这个插件保存的设置读不出来：请到插件设置页重新保存一次再试", http.StatusConflict)
	PluginNotInstallable    = New("plugin_not_installable", "这个插件在当前主机上装不了：版本或所需能力与本机不兼容", http.StatusConflict)
	PluginPackageUntrusted  = New("plugin_package_untrusted", "插件包校验不通过（sha256/签名/权限声明与包内容不一致）：请确认市场源可信后重新拉取，不要反复重试同一个包", http.StatusBadRequest)
	PluginPackageInvalid    = New("plugin_package_invalid", "插件包读不出有效内容：包可能已损坏或不符合插件规范，请联系发布者重新打包", http.StatusBadRequest)
	PluginInstallFailed     = New("plugin_install_failed", "插件写入本机失败：请检查插件目录磁盘空间与写权限", http.StatusInternalServerError)
	// 市场源与信任配置的保存都是「整份替换」：读不出来时界面拿到空列表，用户点一次保存
	// 就把现有的源/受信公钥抹掉。两码都要既说清处境，又劝住「先别保存」。
	PluginMarketSourcesUnreadable = New("plugin_market_sources_unreadable", "插件市场源读不出来（系统配置读取失败，或这条记录被写坏了）：列表显示为空不代表您没配过源。请先修复该问题——在读出来之前不要保存市场源，否则现有的源会被整份替换掉", http.StatusServiceUnavailable)
	PluginTrustConfigUnreadable   = New("plugin_trust_config_unreadable", "插件签名信任配置读不出来（系统配置读取失败，或这条记录被写坏了）：这不代表您没有受信公钥。请先修复该问题——在读出来之前不要保存信任配置，否则现有公钥会被清空，未签名的包会一律装不上", http.StatusServiceUnavailable)
	// PluginStateNotCleared：卸载时目录已删、安装状态却没写回数据库。500——这是服务端自己
	// 没做完，不是用户参数错（面板会显示「仍已安装」，调用又报「未安装」）。
	PluginStateNotCleared = New("plugin_state_not_cleared", "插件文件已删除，但安装状态没能写入数据库：请重试卸载，或确认插件列表里它是否仍显示已安装", http.StatusInternalServerError)
	// 回滚（Rollback）此前所有失败一律 400「请求参数有误」：用户对着输入框反复试，
	// 而每一次重试都会再停一次插件、再动一次目录。以下五码把五种真实处境分开说。
	PluginReleaseNotFound         = New("plugin_release_not_found", "这个历史版本记录已经不在了（可能被清理或换过数据库）：请刷新版本列表后重新选择", http.StatusNotFound)
	PluginReleaseMismatch         = New("plugin_release_mismatch", "这个版本记录不属于该插件（列表串位或参数有误）：请回到插件详情页重新选择版本", http.StatusBadRequest)
	PluginRollbackNoSnapshot      = New("plugin_rollback_no_snapshot", "这个版本的快照文件不在磁盘上（插件目录可能被清理过）：无法回滚到它，请重新安装该插件", http.StatusNotFound)
	PluginRootUnconfigured        = New("plugin_root_unconfigured", "本机还没有配置文件目录，因此无法回滚或安装插件：请先到「系统设置」指定插件目录", http.StatusConflict)
	PluginRollbackRestoreFailed   = New("plugin_rollback_restore_failed", "回滚中途失败，且原来的插件目录没能放回原位：插件目录现在可能是空的。旧版本仍保留在插件目录里名为「<插件ID>.rollback-时间戳」的文件夹中，可手工放回或重装该插件；请勿反复重试回滚（每次都会再改动一次插件目录）", http.StatusInternalServerError)
	PluginRollbackLedgerUnwritten = New("plugin_rollback_ledger_unwritten", "插件代码已回到旧版本，但「当前版本」没能写入数据库：列表可能仍显示新版本，请刷新页面确认，必要时重装该插件", http.StatusInternalServerError)
	// SubtitleSourcesFailed 字幕站全部没答话：是上游问题，不是本服务内部错误。
	SubtitleSourcesFailed = New("subtitle_sources_failed", "字幕源全部查询失败：请检查本机到字幕站的网络，或在字幕设置里补 cookie/token", http.StatusBadGateway)

	// —— 按 id 定位记录（找不到要说 404，不能让 500 冒充服务端故障） ——
	RegistrationNotFound = New("registration_not_found", "注册检测目标不存在，可能已被删除，请刷新列表", http.StatusNotFound)
	CloudDriveNotFound   = New("cloud_drive_not_found", "云盘账号不存在，可能已被删除，请重新选择云盘", http.StatusNotFound)
	StrmRuleNotFound     = New("strm_rule_not_found", "STRM 规则或其云盘账号不存在，可能已被删除，请刷新后重试", http.StatusNotFound)
	StrmRecordNotFound   = New("strm_record_not_found", "这条 STRM 记录不存在，可能已被删除或磁盘文件已变动", http.StatusNotFound)
	// 过滤规则/规则组存在 system_config 的 JSON 里（不是数据库行），删/改到不存在的名字
	// 从前一律压成 400「请求参数有误」——用户既不知道是没了、也不知道是自己写错名字。
	FilterRuleNotFound  = New("filter_rule_not_found", "过滤规则不存在，可能已被删除，请刷新规则列表", http.StatusNotFound)
	FilterGroupNotFound = New("filter_group_not_found", "规则组不存在，可能已被删除，请刷新规则组列表", http.StatusNotFound)
	// 编辑/删除一条已经被删掉的记录：报 500 会让用户以为服务端坏了，报 400 又让他去查表单。
	FileStorageNotFound     = New("file_storage_not_found", "存储后端不存在，可能已被删除，请刷新列表", http.StatusNotFound)
	OpsCredentialNotFound   = New("ops_credential_not_found", "凭据不存在，可能已被删除，请刷新凭据列表", http.StatusNotFound)
	TransferHistoryNotFound = New("transfer_history_not_found", "整理历史记录不存在，可能已被删除，请刷新列表", http.StatusNotFound)
	OpsConfigNotFound       = New("ops_config_not_found", "运维配置项不存在，可能已被删除，请刷新列表", http.StatusNotFound)
	// 告警/通知的「已读、已确认、已静默」标记：改不动行时不能说成功，
	// 否则交接班时以为有人处理过这条告警。
	MonitorAlertNotFound = New("monitor_alert_not_found", "告警不存在，可能已被清理，请刷新告警列表", http.StatusNotFound)
	NoticeNotFound       = New("notice_not_found", "通知不存在，可能已被删除，请刷新通知列表", http.StatusNotFound)
	ContractNotFound     = New("contract_not_found", "保种契约规则不存在，可能已被删除，请刷新契约列表", http.StatusNotFound)
	// ContractRulesUnreadable：契约页的 GET 没有任何参数，从前清单读失败也回 400「请求参数有误」，
	// 把服务端读不出来推成用户手滑，且界面接着显示「暂无规则」——两句话都在撒谎。
	ContractRulesUnreadable = New("contract_rules_unreadable", "保种契约规则清单读不出来：这不是参数问题，列表显示为空也不代表您没建过规则，请稍后重试或查看日志", http.StatusInternalServerError)

	// —— 运维风险操作 ——
	RiskConfirmationRequired = New("risk_confirmation_required", "高危操作，需要先确认风险", http.StatusBadRequest)
	BackupPasswordRequired   = New("backup_password_required", "该备份文件已加密，请输入导出时设置的口令", http.StatusBadRequest)
	BackupPasswordIncorrect  = New("backup_password_incorrect", "备份口令错误", http.StatusBadRequest)
	// 备份文件本身不是本程序导出的（或缺 version 标记）/ 来自更高版本：
	// 这两种都不能算「导入成功」，否则用户以为恢复做完了。
	BackupNotOurFile = New("backup_not_our_file", "这不是 PTPatronus 导出的备份文件：请选择「备份与恢复」页面导出的备份", http.StatusBadRequest)
	// #375 加密信封解析失败（bad salt / bad iv / bad data）：文件本身坏了，不是用户填错。
	// 损坏不可恢复，必须点明「重新导出」，否则用户会反复检查自己从没改过的表单。
	BackupFileCorrupt   = New("backup_file_corrupt", "备份文件已损坏或无效，无法解密：文件可能在传输中被截断或改动过；请到「备份与恢复」重新导出一次后再导入", http.StatusBadRequest)
	BackupVersionTooNew = New("backup_version_too_new", "备份版本高于当前程序，请先升级 PTPatronus 再导入", http.StatusBadRequest)

	// 运维各面板（Docker/磁盘/进程/SSH）都按监控目标 id 下发远程命令；目标被删后
	// 底层抛的是 gorm 的 record not found，一律报 502 会把用户支去查网络而不是查节点配置。
	MonitorTargetNotFound = New("monitor_target_not_found", "运维目标不存在，可能已被删除，请在运维中心重新选择目标主机", http.StatusNotFound)

	// #402 「这个目标不支持这个操作」从前与「目标不可达」共用一句 502「上游服务不可达，
	// 请稍后重试」。这一档是**用户自己能改的**（换个目标类型 / 换个采集方式 / 命令短一点），
	// 而「上游服务不可达」把方向指向网络与 agent —— 用户会去查网络，改不动任何东西。
	//
	// 四条同族哨兵都落这一档（monitor/command.go 与 terminal.go，原文都是英文）：
	//	ErrUnsupportedCommandTarget  目标不是 host/nas（或 NAS 没配 SSH）
	//	ErrUnsupportedCommandRunner  该采集器不支持执行命令
	//	ErrUnsupportedTerminalTarget/Runner  同上，终端侧
	//	ErrCommandTooLong          命令超长
	//
	// 200 vs 4xx：这里返回 400——它确实是「这次请求的目标选错了」，不是上游故障。
	OpsTargetUnsupported = New("ops_target_unsupported",
		"这个运维目标不支持该操作（远程命令/终端只对已配置 SSH 的主机与 NAS 可用）："+
			"请在运维中心换一台已配置 SSH 连接的主机，或先给该 NAS 配置 SSH 后再试。原因见下方。",
		http.StatusBadRequest)

	// #402「命令太长了」与上一档分开：它不是「目标不支持」，换一台主机照样超长。
	// 混在一档会让文案指向错误的方向（用户照做发现还是不行，比给通用话术更糟）。
	OpsCommandTooLong = New("ops_command_too_long",
		"这条命令太长，没能下发到目标主机（命令原文见下方）：请缩短命令后重试。",
		http.StatusBadRequest)

	// —— 娱乐（保号积分）——
	ForbiddenSQL      = New("forbidden_sql", "包含禁止的 SQL 语句", http.StatusBadRequest)
	WriteSQLForbidden = New("write_sql_forbidden", "数据库控制台仅允许只读 SELECT 查询（写操作请走对应功能或 SQL 客户端）", http.StatusForbidden)
	MultiStatement    = New("multi_statement_blocked", "仅允许单条语句", http.StatusBadRequest)
	DuplicateSpend    = New("duplicate_spend", "重复支出，请刷新后重试", http.StatusConflict)
	// 站点侧拒绝（cookie 失效/未登录）：站点原文案随 error 透传，提示用户更新站点 cookie。
	SiteAuthRejected = New("site_auth_rejected", "站点登录态已失效，请在站点管理中更新该站点的 Cookie", http.StatusBadGateway)
	// #366 站点校准诊断（Inspect）的两类失败分开：
	//   - 连不上/地址写错/域名解析不了：用户改一下就好，给 BadGateway 会让他反复重试；
	//   - 抓取超时：这一档「稍后重试」才是对的，与上一档必须区分。
	SiteInspectUnreachable = New("site_inspect_unreachable", errtext.Unreachable, http.StatusBadGateway)
	UpstreamTimeout        = New("upstream_timeout", errtext.Timeout, http.StatusBadGateway)

	// —— 被控 Agent（PR-5 长尾：非透传参数校验字面量） ——
	AgentNameRequired         = New("agent_name_required", "被控端名称必填", http.StatusBadRequest)
	AgentTypeInvalid          = New("agent_type_invalid", "被控端类型必须为 host 或 nas", http.StatusBadRequest)
	AgentNotFound             = New("agent_not_found", "被控端不存在，可能已被离线或删除，请在「运维中心」重新选择被控端", http.StatusNotFound)
	AgentConversationNotFound = New("agent_conversation_not_found", "会话不存在，可能已被删除，请刷新会话列表", http.StatusNotFound)
	AgentMsgRequired          = New("agent_message_required", "消息内容不能为空", http.StatusBadRequest)
	AgentConfirmGone          = New("agent_confirm_gone", "无在途确认（已过期、已取消或已处理）", http.StatusNotFound)

	// —— AI 助手（建会话前预检：不预检就会为每次失败点击留一条空会话） ——
	// 文案与 aiagent.ReadyDisabled / ReadyNoKey 的 SSE 提示是一对镜像。
	AgentDisabled = New("agent_disabled", "AI 助手未启用，请先在「设置 → AI 助手」配置 LLM provider 与 API key", http.StatusServiceUnavailable)
	// 娱乐游戏整组路由默认不注册（ENTERTAINMENT_ENABLED=false）：不挂说明性入口的话，
	// 前端页面只会拿到通用 404「请求的资源不存在」，用户不知道这是被刻意关掉的功能。
	EntertainmentDisabled = New("entertainment_disabled", "娱乐游戏自动化默认关闭（可能违反站规）：需设置环境变量 ENTERTAINMENT_ENABLED=true 并重启程序后才可用", http.StatusServiceUnavailable)
	AgentNoAPIKey         = New("agent_no_api_key", "未配置 API key，请在「设置 → AI 助手」填入", http.StatusServiceUnavailable)
	// 填过 API Key 却解不开（主密钥轮换 / 密文损坏）：与「没填」分开，否则提示会把人引去
	// 重新填写一个其实还在的值。
	AgentKeyUnreadable = New("agent_key_unreadable", "API Key 无法解密（主密钥可能已更换），请在「设置 → AI 助手」重新保存 API Key", http.StatusServiceUnavailable)
	// AgentConfigUnreadable：配置读不出来时被退化成默认值，默认值的 Enabled=false、APIKeyEnc 空——
	// 于是「未启用」「没配 key」两种假话都会出现，而 SetConfig 的合并基准也是这份默认值，
	// 用户不填 key 点一次保存就把真 key 抹掉。所以这条既要说清处境，也要劝住「先别保存」。
	AgentConfigUnreadable = New("agent_config_unreadable", "AI 助手的配置读不出来（系统设置读取失败，或这条记录被写坏了）：这不是「未启用」，也不是「没配 API Key」。请先修复该问题；在读出配置之前不要在「设置 → AI 助手」里点保存，否则现有的 API Key 会被覆盖掉", http.StatusServiceUnavailable)

	// —— PR-6 首批：高频跨域字面量 ——
	DockerLocalNotReady = New("docker_local_not_ready", "本地 Docker 客户端未初始化", http.StatusServiceUnavailable)
	// DockerEngineUnavailable 本机 Docker 引擎根本不在（管道/套接字找不到、连接被拒）。
	// 这类错误**重试永远不会好**，不能和「引擎答了但报错」共用 502「本次服务不可用，请稍后重试」
	//（Docker Desktop 没启动是 NAS/Windows 上的日常状态，面板会一直显示一句无意义的等一等）。
	DockerEngineUnavailable = New("docker_engine_unavailable",
		"连不上本机 Docker 引擎：默认走 /var/run/docker.sock（容器内运行需挂载该 socket）。若 Docker Desktop/服务没启动请先启动它；Windows 原生运行本程序时可设 DOCKER_HOST=tcp://127.0.0.1:2375（或 unix:// 指向 WSL 内的 socket）后重启",
		http.StatusServiceUnavailable)

	// —— PR-6 二批：容器/订阅/令牌/参数校验 ——
	DockerContainerNameRequired = New("docker_container_name_required", "容器名必填", http.StatusBadRequest)
	ResourceNotFound            = New("resource_not_found", "资源不存在，可能已被删除，请刷新列表后重试", http.StatusNotFound)
	AuthTokenFailed             = New("auth_token_failed", "签发登录令牌失败", http.StatusInternalServerError)
	TicketInvalidOrExpired      = New("ticket_invalid_or_expired", "ticket 无效或已过期", http.StatusUnauthorized)
	SubscriptionNotFound        = New("subscription_not_found", "订阅不存在，可能已被删除，请刷新订阅列表后重新添加", http.StatusNotFound)

	// —— PR-6 三批：挂载/种子/媒体库/参数校验 ——
	DiskInvalidMountpoint = New("disk_invalid_mountpoint", "挂载点非法", http.StatusBadRequest)
	TorrentHashRequired   = New("torrent_hash_required", "种子 hash 必填", http.StatusBadRequest)
	MediaLibraryRequired  = New("media_library_required", "library_id 必填", http.StatusBadRequest)
	UrlRequired           = New("url_required", "url 必填", http.StatusBadRequest)
	// #340 原来只有「创建失败」三个字：既没说是创建什么，也没给下一步（全仓最差的一条文案）。
	// 改成说清对象 + 可走的路；真实原因由出口用 WithErr/WithDetail 补进响应体。
	ServarrCreateFailed = New("servarr_create_failed", "订阅创建失败（Radarr/Sonarr 兼容接口）：请检查该条目是否已存在，或在「订阅」页手动添加", http.StatusInternalServerError)

	// —— PR-6 四批：备份/刷流/整理/订阅/工作流/媒体/Compose ——
	BackupReadFailed      = New("backup_read_failed", "读取文件失败", http.StatusBadRequest)
	BrushflowTaskNotFound = New("brushflow_task_not_found", "刷流任务不存在，可能已被删除，请刷新刷流任务列表后重新创建", http.StatusNotFound)
	// 刷流的两个清单读不出来：从前都回 400「请求参数有误」，而这两条 GET 没有任何参数，
	// 界面接着把列表画成「暂无任务」/「暂无种子历史」——一句谎话套两层。
	BrushflowTasksUnreadable  = New("brushflow_tasks_unreadable", "刷流任务列表读不出来：这不是参数问题（这条请求没有入参），列表为空也不代表您没建过任务，请稍后重试或查看日志", http.StatusInternalServerError)
	BrushflowSeedsUnreadable  = New("brushflow_seeds_unreadable", "这个刷流任务的种子历史读不出来：这不是参数问题，列表为空不代表这一轮没加过种子，请稍后重试或查看日志", http.StatusInternalServerError)
	BrushflowDownloaderNoTags = New("brushflow_downloader_no_tags",
		"该下载器类型不支持标签，刷流无法识别本任务种子；请改用 qBittorrent/Transmission 等支持标签的下载器", http.StatusBadRequest)
	TransferTaskNotFound   = New("transfer_task_not_found", "整理任务不存在，可能已被删除，请刷新整理任务列表后重新创建", http.StatusNotFound)
	RssSubscriptionGone    = New("rss_subscription_not_found", "RSS 订阅不存在，可能已被删除，请刷新 RSS 订阅列表后重新添加", http.StatusNotFound)
	WorkflowNotFound       = New("workflow_not_found", "工作流不存在，可能已被删除，请刷新工作流列表后重新选择", http.StatusNotFound)
	MediaKeywordRequired   = New("media_keyword_required", "搜索关键词必填", http.StatusBadRequest)
	MediaSourceIDRequired  = New("media_source_id_required", "source 与 id 必填", http.StatusBadRequest)
	DockerImageRefRequired = New("docker_image_ref_required", "镜像引用必填", http.StatusBadRequest)
	ComposeDirInvalid      = New("compose_dir_invalid", "目录必须位于扫描根内", http.StatusBadRequest)
	ComposeFileNotFound    = New("compose_file_not_found", "该目录内没有 compose 文件：请确认目录选对，或先在此目录新建 compose 文件", http.StatusNotFound)
	// 文件管理器按路径操作：磁盘上那个文件已经没了（被别的程序删了 / 列表是旧的），
	// 报 500 会让用户以为存储坏了，报 200 又谎称删除成功。
	FileNotFound       = New("file_not_found", errtext.FileMissing, http.StatusNotFound)
	ComposeWriteFailed = New("compose_write_failed", "写入 compose 文件失败", http.StatusInternalServerError)

	// —— PR-6 五批（收尾）：散字面量按域归并 ——
	BackupFileInvalid    = New("backup_file_invalid", "请上传 .ptd 备份文件", http.StatusBadRequest)
	BackupFileTooLarge   = New("backup_file_too_large", "文件过大（上限 64MB）", http.StatusBadRequest)
	NameRequired         = New("name_required", "名称必填", http.StatusBadRequest)
	InvalidID            = New("invalid_id", "ID 无法解析为有效编号（可能是空值或非数字）：请检查链接或请求里的 ID，或查看后端日志", http.StatusBadRequest)
	InvalidRequest       = New("invalid_request", "请求格式或内容不合法：后端没能指出具体是哪一处，请对照接口文档核对请求体结构，或查看后端日志", http.StatusBadRequest)
	FieldRequired        = New("field_required", "缺少某个必填项，但后端未能指出是哪一项：对照接口文档补齐必填字段，或查看后端日志", http.StatusBadRequest)
	ResourceStateInvalid = New("resource_state_invalid", "资源当前状态不允许该操作", http.StatusBadRequest)
	DependencyMissing    = New("dependency_missing", "依赖服务未配置或未初始化", http.StatusServiceUnavailable)
	UnauthorizedOp       = New("unauthorized_op", "需要认证或管理员权限", http.StatusUnauthorized)
	UpstreamFailed       = New("upstream_failed", "上游服务调用失败", http.StatusBadGateway)
	OperationForbidden   = New("operation_forbidden", "不允许的操作", http.StatusForbidden)
	// #354 根目录守卫专用：文件管理里 9 处 isRootPath 拒绝（写/删/改名/移动/复制），
	// 从前一律回 OperationForbidden「不允许的操作」——用户既不知道被拒的是哪个动作，
	// 也不知道原因。实际原因很明确：这是文件浏览器的根，操作它没有意义也不安全。
	RootPathProtected = New("root_path_protected", "不允许直接操作存储根目录：请选择根目录下的具体文件或子目录再操作", http.StatusBadRequest)
)

// —— 云盘操作出口（#282）——
// 云端整理 / STRM 修复 / 115 开放平台授权四条出口从前把一切压成 400「请求参数有误」，
// 而这两档重试永远不会有用（要去配东西、不是要改参数），其余才是真的上游没完成。
var (
	CloudTokenUnreadable    = New("cloud_token_unreadable", "该云盘的令牌无法解密（可能更换过主密钥），请到「云盘」里重新保存凭据；本次没有向云盘发请求，文件未被改动", http.StatusServiceUnavailable)
	MediaSearchUnconfigured = New("media_search_unconfigured", "云端整理依赖的媒体识别还没配置：请先在「设置 → 媒体源」配好检索（本次未向云盘发请求，文件未被移动）", http.StatusServiceUnavailable)
	CloudOpUpstreamFailed   = New("cloud_op_upstream_failed", "云盘没有完成这次批量操作：是否已改动部分文件请以云盘页面为准，建议先核对目录再重试", http.StatusBadGateway)
)

// —— 插件动作 / 分身（#287）——
// 这三档从前在 SaveConfig / Bridge / CreatePluginInstance 三个出口全被压成
// 400「请求参数有误，请检查后重试」：插件自己报错、插件没启用、单实例插件不让建分身，
// 在界面上是同一句话，而用户没有任何输入可检查。
var (
	PluginActionFailed = New("plugin_action_failed", "插件没有执行成功这个动作：原因在插件日志里，反复点同一个动作不会变好；先看插件输出，必要时联系插件作者", http.StatusBadGateway)
	// 启用/停用被插件关系或权限确认挡下：既不是「参数有误」（改输入无用），
	// 也不该和存储故障混为一谈。两句都把下一步写在文案里。
	PluginStateChangeRejected = New("plugin_state_change_rejected", "插件状态没能改变：依赖关系不允许，先按提示调整其他插件", http.StatusConflict)
	PluginPermissionsPending  = New("plugin_permissions_pending", "启用前需要先确认这个插件声明的权限：请重新打开插件页再启用", http.StatusConflict)
	PluginSingleInst          = New("plugin_single_instance", "这个插件是单实例插件，不能建分身（内置插件由宿主单点管理）：请直接使用它的主实例", http.StatusConflict)
)

// —— 运维 SQL 控制台（#290）——
// 这条端点已经给「缺 SQL / 写语句 / 禁用语句 / 多语句」各配了专门码，
// 唯独真正执行时数据库给的那句报错被压成通用 400「请求参数有误」——
// 而 no such column / syntax error 正是操作者要改的地方。
var (
	DatabaseQueryFailed = New("database_query_failed", "这条 SQL 没跑起来：把下面数据库给出的原因对着改一下再重跑", http.StatusBadRequest)
	// 表结构查询：真实表至少有一列，查不到任何列 = 表不存在（可能刚被删）。
	// 从前回 200 {"columns":[]}，与「空表」同形，界面画一张没有列的表。
	DatabaseTableNotFound = New("database_table_not_found",
		"这张表不存在：库结构可能刚变过，请刷新表清单后重选", http.StatusNotFound)

	// #398 站点凭据解不开（「配过、但现在读不出来」）从站点抓取的其它失败里分出来。
	//
	// chain.ErrSiteCredentialUnreadable 是 #377 立的可判别哨兵，链层那句话已经把
	// 下一步写全了（点名站点、点名哪一项凭据、说「本次未向站点发请求」）——
	// 而 #377 铺开之后，entertainment.go 有一处出口在读它，其余站点抓取出口
	// （dashboard.SyncOne 与 message 的四处）都不读，全被 Internal/BadGateway 接走，
	// 原因只进日志，界面只见一句「服务内部错误 / 上游不可用」。
	//
	// 为什么单独一档而不并进 BadGateway：这一支的重试永远不会好（换的是主密钥，
	// 不是网络），而且「本次没向站点发请求」是用户判断能否放心重试的唯一依据。
	// 套上「上游服务不可用，请稍后重试」会让人一直重试，并误以为站点那边出了问题。
	SiteCredentialUnreadable = New("site_credential_unreadable",
		"这个站点的凭据读不出来（可能更换过主密钥 SECRET_KEY）：本次没有向站点发请求，"+
			"站点那边没有发生任何事。请到「站点」里重新保存该站点的凭据后再试。原因见下方。",
		http.StatusServiceUnavailable)

	// #397 表清单里每张表的行数读不出来时，从前被 `count, _ := h.countRows(name)` 整个
	// 吞掉、照样 200 返回、那一张显示成 **0**。这一页是管理员的「库还在不在」体检页：
	// users 0 / sites 0 在这一页只有一个读法——数据没了。用户于是去查备份、想去重建，
	// 而真相只是那一张表的 COUNT(*) 没跑出来（连接池打满 / 库被锁 / 表刚被并发删掉）。
	//
	// 关键不在于「少了个数字」，而在于 **0 在这一页不是中性值**。所以这里不说「部分表读不出来」
	// 就完事：必须点名「读不出来的表按未知显示、不当成空表」，否则用户还是会读成 0。
	DatabaseTableCountFailed = New("database_table_count_failed",
		"有表读不出行数（下面的表名后显示为「?」，不是 0 行）：行数没数出来不等于表是空的，"+
			"数据多半还在。请不要据此重建或清理数据库，先到下方查询区执行 SELECT COUNT(*) 确认。原因见下方。",
		http.StatusServiceUnavailable)
)

// —— Trakt 设备授权 / 同步（#280）——
// 这四档从前全被压成 400「请求参数有误，请检查后重试」，而 device-code 与 sync 两个 POST
// 没有请求体：用户既没得检查，也听不到「去哪儿填 / 先点发起授权 / 重新拿码」这句现成的话。
var (
	TraktNotConfigured  = New("trakt_not_configured", "Trakt 应用凭据还没填：请先在 trakt.tv 建应用，再到「设置 → Trakt」填入 client_id/client_secret（本次没有向 Trakt 发请求）", http.StatusServiceUnavailable)
	TraktNotAuthorized  = New("trakt_not_authorized", "Trakt 还没走完设备授权：请先点「发起授权」在浏览器里输码，成功后再同步", http.StatusConflict)
	TraktCodeExpired    = New("trakt_code_expired", "这张设备授权码已经过期：请重新点「发起授权」拿新码（继续等不会生效）", http.StatusConflict)
	TraktUpstreamFailed = New("trakt_upstream_failed", "Trakt 没有完成这次请求：请稍后重试，或到 trakt.tv 核对应用凭据与授权状态", http.StatusBadGateway)
)
