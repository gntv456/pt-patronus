// 后端错误码 → 本地化文案（#355）。
//
// 起因：http.ts 的响应拦截器对后端 envelope 是
//
//	const backend = body?.error || body?.message
//	if (typeof backend === 'string' && backend) { err.message = backend }
//
// 直接把后端文案赋给 err.message，不查 i18n。后端 errcode 信封**总是**带 message，
// 所以英文界面下用户看到的一直是后端中文——i18n 里那套 apiErr.* 的英文兜底
// 几乎永远走不到（只有后端没给 message 的少数场景）。
//
// 本表把「机器可读 code」翻成当前界面语言：查得到就用本地文案，查不到回落后端原文
// （后端那条链层原因往往是唯一线索，不该被加工或吞掉）。
//
// 为什么单独一个文件而不进 locales/*.ts：locale 表是懒加载的大包，错误码映射
// 必须**在请求返回的那一刻**可用——若随 locale 异步加载，首个失败就会拿到空表而
// 回落到中文，等于没修。与 i18nErr.ts 同理：数据层要能静态 import 的轻量入口。

export const errcodeI18n = {
  en: {
    // —— 记录不存在一族（语义：可能被删除，下一步刷新对应列表）——
    not_found: 'The requested item no longer exists (it may have been deleted). Please refresh the list and try again.',
    site_not_found: 'This site no longer exists (it may have been deleted). Please refresh the site list and pick another one.',
    downloader_not_found: 'This downloader no longer exists. Please refresh the downloader list and pick another one.',
    subscription_not_found: 'This subscription no longer exists. Please refresh the subscription list and add it again.',
    workflow_not_found: 'This workflow no longer exists. Please refresh the workflow list and pick another one.',
    rss_subscription_not_found: 'This RSS subscription no longer exists. Please refresh the list and add it again.',
    brushflow_task_not_found: 'This brush-flow task no longer exists. Please refresh the task list and create it again.',
    transfer_task_not_found: 'This organize task no longer exists. Please refresh the task list and create it again.',
    mediaserver_not_found: 'This media server no longer exists. Please refresh the media server list and pick another one.',
    webhook_not_found: 'This webhook no longer exists. Please refresh the webhook list and configure it again.',
    agent_not_found: 'This agent no longer exists (it may be offline or removed). Please pick another agent in the Ops Center.',
    resource_not_found: 'The requested item no longer exists. Please refresh the list and try again.',
    // —— 写入面 ——
    save_failed: 'Saving to the server config failed. Check the backend log for the specific cause; if it mentions the master key, verify SECRET_KEY has not changed.',
    root_path_protected: 'Cannot operate on the storage root directly. Select a specific file or subfolder under the root and try again.',
    operation_forbidden: 'This operation is not allowed.',
    // —— 参数面 ——
    invalid_input: 'The request has an input the backend could not accept, but it could not say which one. Check the API docs for required fields, or read the backend log.',
    invalid_request: 'The request body is malformed. Check the structure against the API docs, or read the backend log.',
    field_required: 'A required field is missing, but the backend could not say which one. Fill in the required fields per the API docs, or read the backend log.',
    invalid_id: 'The ID could not be parsed as a valid number (it may be empty or non-numeric). Check the ID in the link or request, or read the backend log.',
    // —— 其它 ——
    internal: 'Internal server error. Please try again later.',
    bad_gateway: 'The upstream service is unavailable. Please try again later.',
    downloader_conn_failed:
      'Cannot reach the downloader: confirm the client is running and the address/port are correct; use "Test connection" on the Downloaders page.',
    timeout: 'The request timed out. Please try again later.',

    // ===== 以下为本轮补齐：后端「使用 ≥2 次」的码（用户真会撞上的那批）=====
    // 选取依据是后端 codes.go 的实际使用处数，不是全部 171 个码——极冷门码补了
    // 也没人看得到，未命中时仍回落后端中文原文（见 translateErrcode）。
    token_version_mismatch: 'Your session was refreshed elsewhere. Please sign in again.',
    invalid_token: 'Your session is no longer valid. Please sign in again.',
    not_vip: 'This feature requires a HXPT VIP account.',
    vip_not_verified: 'The HXPT account has not completed verification.',
    deprecated: 'This sign-in method has been retired.',
    confirm_required: 'Please confirm before continuing.',
    auth_service_unavailable: 'The sign-in service is not enabled.',
    auth_user_rejected: 'This instance is already bound to its first account; new accounts can no longer sign in.',
    invalid_key: 'Invalid key.',
    oidc_not_configured: 'OIDC is not configured on this server.',
    oidc_disabled: 'SSO is disabled, or issuer / client_id is incomplete. Fill them in under Settings → OIDC/SSO.',
    site_invalid_id: 'Invalid site ID.',
    site_duplicate: 'This site already exists (only one site per domain).',
    downloader_invalid_input: 'The downloader settings are invalid.',
    plugin_market_unreachable:
      'The plugin market source is temporarily unreachable. Check that the market URL is reachable (Settings → Plugin Market) and retry.',
    plugin_not_installed: 'This plugin is not installed on this machine (it may have been uninstalled). Please refresh the plugin list.',
    plugin_market_sources_unreadable:
      'The plugin market sources could not be read (the system config read failed, or the record is corrupted). An empty list does NOT mean you have no sources configured. Fix this first — do not save market sources before they can be read, or the existing sources will be overwritten wholesale.',
    plugin_trust_config_unreadable:
      'The plugin signature trust config could not be read (the system config read failed, or the record is corrupted). This does NOT mean you have no trusted public keys. Fix this first — do not save the trust config before it can be read, or the existing keys will be wiped and unsigned packages will refuse to install.',
    registration_not_found: 'This registration-check target no longer exists (it may have been deleted). Please refresh the list.',
    cloud_drive_not_found: 'This cloud drive account no longer exists (it may have been deleted). Please pick another one.',
    strm_rule_not_found: 'This STRM rule or its cloud drive account no longer exists (it may have been deleted). Please refresh and retry.',
    filter_rule_not_found: 'This filter rule no longer exists (it may have been deleted). Please refresh the rule list.',
    filter_group_not_found: 'This rule group no longer exists (it may have been deleted). Please refresh the group list.',
    file_storage_not_found: 'This storage backend no longer exists (it may have been deleted). Please refresh the list.',
    ops_credential_not_found: 'This credential no longer exists (it may have been deleted). Please refresh the credential list.',
    transfer_history_not_found: 'This organize-history record no longer exists (it may have been deleted). Please refresh the list.',
    ops_config_not_found: 'This ops config item no longer exists (it may have been deleted). Please refresh the list.',
    monitor_alert_not_found: 'This alert no longer exists (it may have been cleaned up). Please refresh the alert list.',
    notice_not_found: 'This notice no longer exists (it may have been deleted). Please refresh the notice list.',
    contract_not_found: 'This seeding contract rule no longer exists (it may have been deleted). Please refresh the contract list.',
    risk_confirmation_required: 'This is a high-risk operation. Please confirm you understand the risk first.',
    monitor_target_not_found:
      'This ops target no longer exists (it may have been deleted). Please pick another host in the Ops Center.',
    // #402「这个目标不支持该操作」与「命令太长」是两档；各自的唯一下一步必须留全
    //（换一台已配 SSH 的主机 / 缩短命令）。漏译会让英文界面显示中文，且丢掉这两句。
    ops_target_unsupported:
      'This ops target does not support that operation (remote commands and terminals work on hosts and NAS devices that already have SSH configured). Pick another host with SSH in the Ops Center, or configure SSH for this NAS first, then retry. See the reason below.',
    ops_command_too_long:
      'That command is too long to send to the target host (the original command is below). Shorten the command and retry.',
    site_auth_rejected: "This site's session has expired. Update that site's cookie in Site Management.",
    agent_disabled: 'The AI assistant is not enabled. Configure the LLM provider and API key under Settings → AI Assistant first.',
    agent_no_api_key: 'No API key configured. Enter one under Settings → AI Assistant.',
    agent_key_unreadable:
      'The API key could not be decrypted (the master key may have changed). Re-save the API key under Settings → AI Assistant.',
    agent_config_unreadable:
      'The AI assistant config could not be read (the system settings read failed, or the record is corrupted). This does NOT mean "not enabled", nor "no API key". Fix this first — do not save under Settings → AI Assistant before it can be read, or the existing API key will be overwritten.',
    docker_local_not_ready: 'The local Docker client is not initialised.',
    docker_container_name_required: 'Container name is required.',
    auth_token_failed: 'Failed to issue a sign-in token.',
    ticket_invalid_or_expired: 'The ticket is invalid or has expired.',
    disk_invalid_mountpoint: 'Invalid mount point.',
    torrent_hash_required: 'Torrent hash is required.',
    media_library_required: 'library_id is required.',
    url_required: 'url is required.',
    servarr_create_failed:
      'Creating the subscription failed (Radarr/Sonarr compatibility API). Check whether the item already exists, or add it manually on the Subscriptions page.',
    backup_read_failed: 'Failed to read the file.',
    media_keyword_required: 'Search keyword is required.',
    media_source_id_required: 'source and id are required.',
    docker_image_ref_required: 'Image reference is required.',
    compose_dir_invalid: 'The directory must be inside the configured scan root.',
    compose_file_not_found:
      'No compose file in this directory. Check that the directory is right, or create a compose file here first.',
    compose_write_failed: 'Failed to write the compose file.',
    name_required: 'Name is required.',
    resource_state_invalid: "The item's current state does not allow this operation.",
    dependency_missing: 'A required dependency service is not configured or not initialised.',
    unauthorized_op: 'Authentication or administrator privileges are required.',
    upstream_failed: 'The upstream service call failed.',
    backup_write_failed:
      'The sites in the backup could not be written to the database. No sites were imported (the batch runs in one transaction and has been rolled back). See the server log for the cause, fix it, then re-import.',
    plugin_action_failed:
      'The plugin did not perform this action successfully. The reason is in the plugin log; retrying the same action will not help. Check the plugin output first, and contact the plugin author if needed.',
    plugin_state_change_rejected:
      'The plugin state could not be changed: dependencies do not allow it. Adjust the other plugins as the message indicates.',
    database_query_failed: 'This SQL query did not run. Fix it using the reason the database gave below, then run it again.',
    trakt_upstream_failed:
      'Trakt did not complete this request. Try again later, or check the app credentials and authorisation status on trakt.tv.',

    // —— #376 补齐后端**确实在用**但此前未映射的 87 个码 ——
    // 口径：静态扫 backend/internal（剔 _test.go）里 `errcode.<标识符>` 的实际引用处数，
    // 列出引用 ≥1 次却查不到的码。上一轮那句「只补使用 ≥2 次」的取舍是 2026-10 定的，
    // 此后 #366–#375 又新增了若干码；更重要的是：≥1 次就意味着这条错误**真的会出现在
    // 某个用户面前**，而英文界面下它会原样显示中文 —— 这与 #355 要治的正是同一件事。
    // 全冷门（引用 0 次、只注册）的码仍然不补：没人看得到，补了也只是噪音。

    // —— 通用/状态 ——
    conflict: 'The item already exists or conflicts with another change. Refresh the list and try again.',
    unavailable: 'The service is temporarily unavailable. Try again shortly.',
    ssrf_blocked:
      'That address points inside your private network and was blocked by the SSRF guard. Use a publicly reachable address, or set ALLOW_PRIVATE_SITES=true and restart.',
    forbidden_sql:
      'That SQL statement was rejected because it is not allowed here. Use read-only statements, or adjust it using the reason below.',
    write_sql_forbidden: 'That SQL statement writes data and was rejected. Read-only statements are allowed here.',
    multi_statement_blocked:
      'Multiple SQL statements separated by ";" are not allowed. Send one statement per request.',
    file_not_found: 'The file no longer exists. Refresh the file list and try again.',

    // —— 鉴权 / 账号 ——
    admin_required: 'Administrator privileges are required for this action.',
    not_activated: 'This account has not been activated yet. Open the activation email and finish signing up.',
    vip_unknown: 'The VIP status of this account could not be determined. Sign in again, then retry.',
    vip_expired:
      'The VIP verification is too old. Sign in again to refresh it; until then VIP-only features stay unavailable.',
    vip_check_disabled: 'Periodic VIP re-verification is turned off, so VIP status is not being refreshed.',
    challenge_mode:
      'The site is showing a captcha challenge, so the request could not complete. Open the site in a browser, pass the challenge, and try again.',
    auth_server_busy: 'The site is temporarily refusing login requests. Try again in a few minutes.',
    auth_ticket_failed:
      'The login ticket could not be issued. Reload the page to get a new ticket, then sign in again.',
    auth_captcha_failed: 'The captcha could not be loaded. Check whether the site is reachable, then retry.',
    auth_bad_credentials: 'That username or password is not accepted. Check them and try again.',
    auth_cookie_required:
      'This operation needs the site login cookie. Sign in to the site from the extension, then retry.',
    demo_disabled: 'Demo mode is on, so this action is disabled. Turn demo mode off in the system settings.',
    apikey_no_refresh: 'API keys cannot be refreshed. Create a new key instead.',
    apikey_no_logout: 'API-key sessions cannot be signed out. Revoke the key instead.',
    oidc_state_mismatch:
      'The single sign-on state did not match, so the sign-in was cancelled. Start the sign-in again from this site.',
    oidc_provider_unreachable:
      'The single sign-on provider could not be reached. Check the provider address and that it is running, then retry.',
    oidc_login_failed: 'Single sign-on did not complete. Check the account mapping and try again.',
    value_too_large: 'The value is larger than this field allows. Enter a smaller one.',
    value_invalid_json: 'That value is not valid JSON. Check the brackets, commas and quoting.',

    // —— 备份 ——
    backup_password_required: 'This backup is encrypted. Enter the backup password to import it.',
    backup_password_incorrect: 'That backup password is not correct. Enter the password used when the backup was exported.',
    backup_not_our_file: 'This is not a PTPatronus backup. Choose a file exported from "Backup and restore".',
    backup_file_invalid: 'This backup file is not valid and could not be parsed. Export a new one and import that.',
    backup_file_too_large: 'This backup file is too large to process. Split it, or raise the server size limit.',
    backup_file_corrupt:
      'This backup file is damaged and cannot be decrypted. It may have been truncated or modified in transit; export a fresh backup and import that instead.',
    backup_version_too_new:
      'This backup was made by a newer version than this server. Upgrade the server, then import it again.',
    backup_not_moviepilot_file: 'This is not a MoviePilot backup file. Choose an export from MoviePilot.',

    // —— 站点校准诊断（#366）——
    site_inspect_unreachable:
      'That address could not be reached. Check the URL, the port and local DNS; if the address is on your own network, allow private sites explicitly.',

    // —— 通用上游（#366）——
    upstream_timeout: 'The upstream request timed out. Check whether the service is busy, then try again.',

    // —— 媒体 / 封面 ——
    media_image_too_large: 'That image is too large. Provide a smaller one.',
    media_cover_no_image: 'No cover image was provided. Choose an image, or clear the cover.',
    media_search_unconfigured:
      'No media search provider is configured. Add one in the media settings before searching.',

    // —— OCR（#353）——
    ocr_service_unreachable:
      'The OCR service could not be reached. Check the endpoint address and that the service is running, then retry.',
    ocr_service_bad_answer:
      'The OCR service replied but the answer could not be read. Check that the recognition mode matches the format it returns.',

    // —— Webhook ——
    webhook_invalid: 'This webhook is not valid. Check its URL and settings.',
    webhook_bad_payload: 'The webhook request body could not be read. Check the sender format.',
    webhook_bad_signature:
      'The webhook signature did not match. Check that the sender uses the current signing secret.',
    webhook_name_required: 'Give the webhook a name.',
    webhook_secret_unavailable:
      'The signing secret for this webhook could not be decrypted (the master key may have changed). Re-save the secret in the webhook settings.',

    // —— 娱乐域 ——
    entertainment_game_unsupported: 'This game does not support that view.',
    entertainment_game_not_found: 'This game is not registered on this site.',
    entertainment_spend_failed:
      'The in-game action was not completed, so nothing was spent. Check the reason below and the in-game balance.',
    entertainment_data_unreadable:
      'The in-game data could not be read. The login may have expired; sign in to the game again.',
    // #398 站点凭据解不开。与娱乐域那条同型但不同码（站点抓取面共用 respondFetchError）。
    site_credential_unreadable:
      'This site\u2019s credentials could not be decrypted (the master key SECRET_KEY may have changed). No request was sent to the site, so nothing happened there. Re-save that site\u2019s credentials in the Sites page, then try again. See the reason below.',
    entertainment_credential_unreadable:
      'The site credentials could not be decrypted (the master key may have changed). No request was sent to the site. Re-save the cookie in the site settings.',
    entertainment_disabled: 'This site is disabled, so that action was not sent.',

    // —— 插件 ——
    plugin_not_reloadable: 'This plugin cannot be reloaded. Restart the server to apply changes to it.',
    plugin_not_in_market: 'This plugin is no longer listed in the market. Refresh the list and pick another one.',
    plugin_instance_not_found: 'This plugin instance no longer exists. Refresh the instance list.',
    plugin_disabled: 'This plugin is disabled. Enable it in the plugin list first.',
    plugin_config_unreadable:
      'The plugin configuration could not be read. Check whether the configuration file was edited or damaged.',
    plugin_not_installable: 'This plugin cannot run on this host (missing capability, or the install plan conflicts).',
    plugin_package_untrusted:
      'This plugin package does not match what the market published; it may have been tampered with. Do not install it — refresh the market source instead.',
    plugin_package_invalid: 'The plugin package could not be read as a valid plugin. Re-download it from the market.',
    plugin_install_failed: 'The plugin could not be written to this machine. Check disk space and permissions, then retry.',
    plugin_state_not_cleared:
      'The plugin left some state behind, so the action was not completed. Restart the server, then retry.',
    plugin_release_not_found: 'That plugin release no longer exists. Refresh the release list.',
    plugin_release_mismatch:
      'The installed version does not match the release being rolled back to. Refresh the release list and try again.',
    plugin_rollback_no_snapshot:
      'There is no snapshot to roll back to, because none was taken before the upgrade. Upgrade again after taking a backup.',
    plugin_root_unconfigured:
      'The plugin directory has not been set. Choose one in the plugin settings before installing.',
    plugin_rollback_restore_failed:
      'Rolling the plugin back failed, so the current version is unchanged. Check the reason below, or install the new version again.',
    plugin_rollback_ledger_unwritten:
      'The rollback succeeded but its record could not be written, so the history is incomplete. Check the reason below.',
    plugin_permissions_pending:
      'Confirm the permissions this plugin asks for before it can be installed.',
    plugin_single_instance:
      'This is a single-instance plugin, so it cannot run as a separate instance. Use the main instance instead.',

    // —— 字幕 / STRM ——
    subtitle_sources_failed:
      'None of the subtitle sources replied. Check network access and the configured tokens, then try again.',
    strm_record_not_found: 'This STRM record no longer exists. Refresh the list and pick another one.',
    contract_rules_unreadable:
      'The contract rules could not be read. Check whether the rules file was edited or damaged.',

    // —— 刷流（#352）——
    brushflow_tasks_unreadable: 'The brush-flow tasks could not be read. Refresh the page and try again.',
    brushflow_seeds_unreadable: 'The brush-flow seed history could not be read. Refresh the page and try again.',
    brushflow_downloader_no_tags:
      'No downloader is tagged for brush-flow, so there is nothing to act on. Tag a downloader first.',

    // —— Docker ——
    docker_engine_unavailable:
      'The local Docker engine could not be reached. Check whether Docker is running and that the socket path is correct.',

    // —— 云盘 ——
    cloud_token_unreadable:
      'The cloud drive token could not be decrypted (the master key may have changed). Re-save the token in the cloud settings.',
    cloud_op_upstream_failed:
      'The cloud drive rejected the operation. Check the reason below, then retry.',

    // —— Trakt ——
    trakt_not_configured: 'Trakt is not set up. Add the client ID and secret in the media settings first.',
    trakt_not_authorized: 'Trakt authorisation has expired. Authorise the app again in the media settings.',
    trakt_code_expired:
      'That Trakt authorisation code has expired. Start the Trakt authorisation again.',

    // —— Agent ——
    agent_name_required: 'Give the agent a name.',
    agent_type_invalid: 'That agent type is not supported. Pick one of the supported types.',
    agent_conversation_not_found: 'This conversation no longer exists. Start a new one.',
    agent_message_required: 'Type a message before sending.',
    agent_confirm_gone:
      'This confirmation is no longer pending (it was already answered or has expired). Run the action again to get a fresh confirmation.',

    // —— 娱乐域消费去重 ——
    duplicate_spend:
      'This in-game action was already performed recently, so it was not repeated. Wait before trying again.',
  },
  'zh-CN': {} as Record<string, string>,
} as unknown as { en: Record<string, string>; 'zh-CN': Record<string, string> }

// translateErrcode 按 code 查当前界面的本地文案。
// locale 非 en-US（目前只有 zh-CN）时直接回原文：后端给的就是中文，
// 套一层「前端翻译」只会把更具体的原因磨掉——那正是本仓反复出的问题。
//
// specific=true 时后端给的是**这一条请求专属的原因**（信封带 specific 标记，
// 由 errcode.WithDetail 系列置上），一律原样显示：过去 en-US 下会把
// 「缺少必填项 platform」整句换成码的固定译文，后端刚说清楚的字段名又被吞掉。
export function translateErrcode(
  code: string | undefined,
  backendText: string,
  locale: string,
  specific = false,
): string {
  if (locale !== 'en-US') return backendText
  if (specific) return backendText
  const key = String(code || '')
  if (!key) return backendText
  const table = errcodeI18n.en as Record<string, string>
  return table[key] || backendText
}
