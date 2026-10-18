package errcode

import "net/http"

// 备份导入与「写失败半途」域（从 codes.go 拆出：该文件触行数门禁；这批码语义同族——
// 都在说「这次写操作没有全部生效，下一步是什么」）。

// —— 备份导入（#285）——
var (
	// 选错文件：说清要的是什么形状、从哪儿导出，而不是「参数有误」让人去改没得改的输入。
	BackupNotMoviePilotFile = New("backup_not_moviepilot_file", "这份文件不是 MoviePilot 导出的站点配置：它应是一个 JSON 数组，每项含 name 与 url（在 MoviePilot 的站点管理里导出）", http.StatusBadRequest)
	// 站点批量写在一个事务里：写失败＝一个都没落库，必须这么说，否则用户以为导了一半而反复重试。
	// #396 注册检测域写失败。这条路径零测试覆盖、读者主要是 cron 而非界面，
	// 所以文案落在「原因可查」上——失败时除了 slog 没有任何别的地方能说清发生了什么。
	RegistrationWriteFailed = New("registration_write_failed",
		"注册检测的记录没能写进数据库：本次操作没有生效（若检测已跑完，其结论未被记录，"+
			"下一轮会继续读到旧状态）。请检查数据库后重试。原因见下方。",
		http.StatusServiceUnavailable)
	// #395 AI 助手 API Key 密封失败。与「存储写不进去」是完全不同的两件事：
	// 这一支是主密钥（SECRET_KEY）侧的问题，重试永远不会好，用户该去确认密钥。
	// 文案范式与 WebhookSecretUnavailable 同一族。
	AgentSecretWriteFailed = New("agent_secret_write_failed",
		"AI 助手的 API Key 没能加密保存（可能更换过主密钥 SECRET_KEY）：本次没有保存任何配置。"+
			"请确认服务器上的 SECRET_KEY 后重新保存，或先清掉 API Key 只保存其余设置。原因见下方。",
		http.StatusServiceUnavailable)

	// #392 辅种配置是「多次独立写、无事务」（SystemConfigOper.Set 每个 key 一次读写）。
	// 这两句必须说清「已经写进去多少」——不然用户会以为什么都没存而再点一次保存，
	// 与现实正好相反（旧文案「请稍后重试」就是这么误导人的）。
	KeepSeedConfigWriteFailed = New("keepseed_config_write_failed",
		"辅种配置没能保存，本次没有写入任何设置：可以修复后重试。原因见下方。",
		http.StatusServiceUnavailable)
	KeepSeedConfigPartialSaved = New("keepseed_config_partial_saved",
		"辅种配置只保存了一部分：IYUU token 已生效，但后续设置没能写入（Torznab 端点/Key 仍是旧值）。"+
			"请修复后重新保存剩余项，不必再次填写 token。原因见下方。",
		http.StatusServiceUnavailable)
	// #391 运维批量命令的审计记录写不进去 ⇒ 命令一条都没执行。
	// 这句必须点明「未执行」：用户据此判断能否安全重试（旧文案「请稍后重试」会诱导重复执行）。
	OpsAuditWriteFailed = New("ops_audit_write_failed",
		"运维命令的审计记录没能写入，因此本次没有执行任何命令：若命令列表为空可直接重试；"+
			"若不确定上一次是否已执行，请先到目标机器上核对再重试。原因见下方。",
		http.StatusServiceUnavailable)

	BackupWriteFailed = New("backup_write_failed", "备份里的站点没能写入数据库：本次没有导入任何站点（这一批在同一个事务里，已整体回滚）。原因见服务日志，处理后再重新导入", http.StatusServiceUnavailable)
)
