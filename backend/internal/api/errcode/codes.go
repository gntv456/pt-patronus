package errcode

import "net/http"

// 预定义错误码唯一注册表：新增码必须先进这里（codes_test.go 校验唯一性）。
// 命名 <domain>_<reason>；HTTP 状态与语义对齐（4xx 客户端可恢复 / 5xx 服务端故障）。
//
// 第一批收录 api/v1 既有 19 处裸 code 字符串（PR-1 收口），后续域随 PR-2+ 扩充。
var (
	// —— 通用 ——
	Internal     = New("internal", "服务内部错误，请稍后重试", http.StatusInternalServerError)
	InvalidInput = New("invalid_input", "请求参数有误，请检查后重试", http.StatusBadRequest)
	NotFound     = New("not_found", "请求的资源不存在", http.StatusNotFound)
	BadGateway   = New("bad_gateway", "上游服务不可用，请稍后重试", http.StatusBadGateway)
	Conflict     = New("conflict", "资源冲突，请刷新后重试", http.StatusConflict)
	Unavailable  = New("unavailable", "服务暂不可用，请稍后重试", http.StatusServiceUnavailable)

	// —— 鉴权 / VIP ——
	AdminRequired   = New("admin_required", "需要管理员权限", http.StatusForbidden)
	TokenMismatch   = New("token_version_mismatch", "登录态已更新，请重新登录", http.StatusUnauthorized)
	NotActivated    = New("not_activated", "APP 未激活，请先使用 好学 APP 专属 VIP 激活码", http.StatusForbidden)
	NotVIP          = New("not_vip", "此功能需要好学 VIP", http.StatusForbidden)
	VIPNotVerified  = New("vip_not_verified", "好学账号未完成验证", http.StatusForbidden)
	VIPUnknown      = New("vip_unknown", "好学账号验证状态未知，请稍后重试", http.StatusForbidden)
	VIPExpired      = New("vip_expired", "好学 VIP 已过期", http.StatusForbidden)
	ChallengeMode   = New("challenge_mode", "好学已开启挑战响应认证，请改用 Cookie 登录", http.StatusConflict)
	OIDCDeprecated  = New("deprecated", "该登录方式已停用", http.StatusGone)
	ConfirmRequired = New("confirm_required", "需要先完成确认", http.StatusPreconditionRequired)

	// —— 登录 / 会话（PR-2 认证域） ——
	AuthServiceUnavailable = New("auth_service_unavailable", "登录服务未启用", http.StatusServiceUnavailable)
	AuthServerBusy         = New("auth_server_busy", "服务暂不可用，请稍后重试", http.StatusInternalServerError)
	AuthTicketFailed       = New("auth_ticket_failed", "签发 ticket 失败", http.StatusInternalServerError)
	AuthCaptchaFailed      = New("auth_captcha_failed", "取验证码失败", http.StatusBadGateway)
	AuthBadCredentials     = New("auth_bad_credentials", "用户名/密码/验证码错误", http.StatusUnauthorized)
	AuthCookieRequired     = New("auth_cookie_required", "cookie 必填", http.StatusBadRequest)
	DemoDisabled           = New("demo_disabled", "演示登录未启用", http.StatusNotFound)
	APIKeyNoRefresh        = New("apikey_no_refresh", "API-Key 旁路无需续期", http.StatusBadRequest)
	APIKeyNoLogout         = New("apikey_no_logout", "API-Key 旁路无需登出", http.StatusBadRequest)
	InvalidKey             = New("invalid_key", "key 非法", http.StatusBadRequest)
	ValueTooLarge          = New("value_too_large", "value 超过 64KB 上限", http.StatusBadRequest)
	ValueInvalidJSON       = New("value_invalid_json", "value 必须为合法 JSON", http.StatusBadRequest)
	VIPCheckDisabled       = New("vip_check_disabled", "VIP 校验未启用", http.StatusBadRequest)
	OIDCNotConfigured      = New("oidc_not_configured", "OIDC 未装配", http.StatusServiceUnavailable)
	OIDCStateMismatch      = New("oidc_state_mismatch", "OIDC state 校验失败", http.StatusForbidden)
	SaveFailed             = New("save_failed", "保存失败", http.StatusInternalServerError)

	// —— Webhook（PR-2） ——
	WebhookNotFound     = New("webhook_not_found", "webhook 不存在", http.StatusNotFound)
	WebhookInvalid      = New("webhook_invalid", "无效的 webhook", http.StatusNotFound)
	WebhookBadPayload   = New("webhook_bad_payload", "读取请求体失败", http.StatusBadRequest)
	WebhookBadSignature = New("webhook_bad_signature", "签名校验失败", http.StatusUnauthorized)
	WebhookNameRequired = New("webhook_name_required", "名称必填", http.StatusBadRequest)

	// —— 站点 / 下载器（PR-3） ——
	SiteNotFound           = New("site_not_found", "站点不存在", http.StatusNotFound)
	SiteInvalidID          = New("site_invalid_id", "无效的站点 ID", http.StatusBadRequest)
	DownloaderNotFound     = New("downloader_not_found", "下载器不存在", http.StatusNotFound)
	DownloaderInvalidInput = New("downloader_invalid_input", "下载器参数有误", http.StatusBadRequest)
	DownloaderConnFailed   = New("downloader_conn_failed", "下载器连接失败", http.StatusBadGateway)
	DownloaderPushFailed   = New("download_push_failed", "下发种子失败", http.StatusBadGateway)

	// —— 运维风险操作 ——
	RiskConfirmationRequired = New("risk_confirmation_required", "高危操作，需要先确认风险", http.StatusBadRequest)
	BackupPasswordRequired   = New("backup_password_required", "该备份文件已加密，请输入导出时设置的口令", http.StatusBadRequest)
	BackupPasswordIncorrect  = New("backup_password_incorrect", "备份口令错误", http.StatusBadRequest)

	// —— 娱乐（保号积分）——
	ForbiddenSQL   = New("forbidden_sql", "包含禁止的 SQL 语句", http.StatusBadRequest)
	WriteSQLForbidden = New("write_sql_forbidden", "数据库控制台仅允许只读 SELECT 查询（写操作请走对应功能或 SQL 客户端）", http.StatusForbidden)
	MultiStatement = New("multi_statement_blocked", "仅允许单条语句", http.StatusBadRequest)
	DuplicateSpend = New("duplicate_spend", "重复支出，请刷新后重试", http.StatusConflict)

	// —— 被控 Agent（PR-5 长尾：非透传参数校验字面量） ——
	AgentNameRequired = New("agent_name_required", "被控端名称必填", http.StatusBadRequest)
	AgentTypeInvalid  = New("agent_type_invalid", "被控端类型必须为 host 或 nas", http.StatusBadRequest)
	AgentNotFound     = New("agent_not_found", "被控端不存在", http.StatusNotFound)
	AgentMsgRequired  = New("agent_message_required", "消息内容不能为空", http.StatusBadRequest)
	AgentConfirmGone  = New("agent_confirm_gone", "无在途确认（已过期、已取消或已处理）", http.StatusNotFound)

	// —— PR-6 首批：高频跨域字面量 ——
	DockerLocalNotReady = New("docker_local_not_ready", "本地 Docker 客户端未初始化", http.StatusServiceUnavailable)

	// —— PR-6 二批：容器/订阅/令牌/参数校验 ——
	DockerContainerNameRequired = New("docker_container_name_required", "容器名必填", http.StatusBadRequest)
	ResourceNotFound            = New("resource_not_found", "资源不存在", http.StatusNotFound)
	AuthTokenFailed             = New("auth_token_failed", "签发登录令牌失败", http.StatusInternalServerError)
	TicketInvalidOrExpired      = New("ticket_invalid_or_expired", "ticket 无效或已过期", http.StatusUnauthorized)
	SubscriptionNotFound        = New("subscription_not_found", "订阅不存在", http.StatusNotFound)

	// —— PR-6 三批：挂载/种子/媒体库/参数校验 ——
	DiskInvalidMountpoint = New("disk_invalid_mountpoint", "挂载点非法", http.StatusBadRequest)
	TorrentHashRequired   = New("torrent_hash_required", "种子 hash 必填", http.StatusBadRequest)
	MediaLibraryRequired  = New("media_library_required", "library_id 必填", http.StatusBadRequest)
	UrlRequired           = New("url_required", "url 必填", http.StatusBadRequest)
	ServarrCreateFailed   = New("servarr_create_failed", "创建失败", http.StatusInternalServerError)

	// —— PR-6 四批：备份/刷流/整理/订阅/工作流/媒体/Compose ——
	BackupReadFailed       = New("backup_read_failed", "读取文件失败", http.StatusBadRequest)
	BrushflowTaskNotFound  = New("brushflow_task_not_found", "刷流任务不存在", http.StatusNotFound)
	TransferTaskNotFound   = New("transfer_task_not_found", "整理任务不存在", http.StatusNotFound)
	RssSubscriptionGone    = New("rss_subscription_not_found", "RSS 订阅不存在", http.StatusNotFound)
	WorkflowNotFound       = New("workflow_not_found", "工作流不存在", http.StatusNotFound)
	MediaKeywordRequired   = New("media_keyword_required", "搜索关键词必填", http.StatusBadRequest)
	MediaSourceIDRequired  = New("media_source_id_required", "source 与 id 必填", http.StatusBadRequest)
	DockerImageRefRequired = New("docker_image_ref_required", "镜像引用必填", http.StatusBadRequest)
	ComposeDirInvalid      = New("compose_dir_invalid", "目录必须位于扫描根内", http.StatusBadRequest)
	ComposeFileNotFound    = New("compose_file_not_found", "目录内未找到 compose 文件", http.StatusNotFound)
	ComposeWriteFailed     = New("compose_write_failed", "写入 compose 文件失败", http.StatusInternalServerError)

	// —— PR-6 五批（收尾）：散字面量按域归并 ——
	BackupFileInvalid    = New("backup_file_invalid", "请上传 .ptd 备份文件", http.StatusBadRequest)
	BackupFileTooLarge   = New("backup_file_too_large", "文件过大（上限 64MB）", http.StatusBadRequest)
	NameRequired         = New("name_required", "名称必填", http.StatusBadRequest)
	InvalidID            = New("invalid_id", "无效的 ID", http.StatusBadRequest)
	InvalidRequest       = New("invalid_request", "请求参数有误", http.StatusBadRequest)
	FieldRequired        = New("field_required", "必填字段缺失", http.StatusBadRequest)
	ResourceStateInvalid = New("resource_state_invalid", "资源当前状态不允许该操作", http.StatusBadRequest)
	DependencyMissing    = New("dependency_missing", "依赖服务未配置或未初始化", http.StatusServiceUnavailable)
	UnauthorizedOp       = New("unauthorized_op", "需要认证或管理员权限", http.StatusUnauthorized)
	UpstreamFailed       = New("upstream_failed", "上游服务调用失败", http.StatusBadGateway)
	OperationForbidden   = New("operation_forbidden", "不允许的操作", http.StatusForbidden)
)
