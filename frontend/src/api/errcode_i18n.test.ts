// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { describe, it, expect, beforeEach } from 'vitest'
import { http } from './http'
import { i18n } from '@/i18n'
import { translateErrcode, errcodeI18n } from './errcodeI18n'

// #355 英文界面下，后端给的中文错误文案会原样显示（渲染层缺口）。
//
// http.ts 的响应拦截器：
//
//	const backend = body?.error || body?.message
//	if (typeof backend === 'string' && backend) {
//	    err.message = backend          ← 直接赋值，不查 i18n
//	} else if (...) { err.message = terr('apiErr.xxx', …) }
//
// 后一句的兜底是本地化的，前一句不是。注释写「英文界面不再混中文」——但那只在
// 「后端没给 message」时成立。而后端 errcode 信封**总是**给 message（error 与
// message 同值），所以：
//  1. 英文界面下用户看到的是后端的中文（本轮 #336/#340/#342/#344/#348/#353/#354
//     等新写的文案全在这一档）；
//  2. i18n 里那套 apiErr.* 的英文兜底几乎永远走不到（只有 blob 端点等少数场景）。
//
// 修法：给「后端机器可读 code」建一张 i18n 映射（errcode.*），按 code 查本地文案；
// 查不到就回落后端原文（那是不该被加工的唯一线索）。

// 用自定义 adapter 注入伪响应（与 http.test.ts 同法，不引新依赖）。
function rejectWithEnvelope(status: number, data: any) {
  const e: any = new Error('err')
  e.response = { status, data }
  http.defaults.adapter = (() => Promise.reject(e)) as any
}

describe('后端错误码的 i18n 映射（#355）', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('英文界面下后端中文不应原样显示（应按 code 翻成英文）', async () => {
    i18n.global.locale.value = 'en-US'
    rejectWithEnvelope(404, {
      code: 'site_not_found',
      message: '站点不存在，可能已被删除，请刷新站点列表后重新选择',
      error: '站点不存在，可能已被删除，请刷新站点列表后重新选择',
    })
    let seen = ''
    try {
      await http.get('/api/v1/sites/999999')
    } catch (e) {
      seen = (e as Error).message
    }
    expect(seen).not.toBe('')
    expect(/[一-龥]/.test(seen), `英文界面显示了中文：${seen}`).toBe(false)
  })

  it('中文界面照旧显示后端原文（那条文案比前端翻译更具体，不许被磨掉）', async () => {
    i18n.global.locale.value = 'zh-CN'
    const raw = '下载器（编号 2）里没有这颗种子：请在「下载器」页确认它是否已被删除'
    rejectWithEnvelope(404, { code: 'not_found', message: raw, error: raw })
    let seen = ''
    try {
      await http.get('/api/v1/transfer/organize')
    } catch (e) {
      seen = (e as Error).message
    }
    expect(seen).toBe(raw)
  })

  it('映射表要覆盖本轮修过的高频错误码', () => {
    for (const code of ['site_not_found', 'save_failed', 'root_path_protected', 'not_found']) {
      expect(errcodeI18n.en[code], `${code} 缺英文文案`).toBeTruthy()
    }
  })

  it('未知码必须回落到后端原文，不许吞掉原因', () => {
    const raw = '下载器（编号 2）里没有这颗种子：请在「下载器」页确认'
    expect(translateErrcode('some_future_code', raw, 'en-US')).toBe(raw)
    expect(translateErrcode('', raw, 'en-US')).toBe(raw)
    expect(translateErrcode(undefined, raw, 'zh-CN')).toBe(raw)
  })
})

// #356 映射表必须覆盖后端「使用 ≥2 次」的错误码。
//
// 后端有 171 个码，全补不现实（极冷门码没人看得到），但**用户真会撞上的那批**
// 必须有英文。选取标准是后端 codes.go 里被实际使用 ≥2 次的码——这个下限是量的。
//
// 清单写成静态数组而非运行时扫后端源码：浏览器 tsconfig 没有 node 类型，
// 运行时读文件在 vitest/jsdom 下也不稳（dynamic import 回调未指定）。
// 换来的代价是后端加码时要手工加一行——但那正是本判据存在的意义：
// 忘了加就会红，而不是悄悄漏掉。
//
// 复核口径（2026-10）：从 backend/internal 全量统计 errcode.X 的使用处数，
// 剔掉 <2 次的，得到下列 63 个；errcodeI18n.en 已逐条覆盖。
const HIGH_FREQ_CODES = [
  'not_found', 'site_not_found', 'downloader_not_found', 'subscription_not_found',
  'workflow_not_found', 'rss_subscription_not_found', 'brushflow_task_not_found',
  'transfer_task_not_found', 'mediaserver_not_found', 'webhook_not_found',
  'agent_not_found', 'resource_not_found',
  'save_failed', 'root_path_protected', 'operation_forbidden',
  'invalid_input', 'invalid_request', 'field_required', 'invalid_id',
  'internal', 'bad_gateway', 'downloader_conn_failed', 'timeout',
  'token_version_mismatch', 'invalid_token', 'not_vip', 'vip_not_verified',
  'deprecated', 'confirm_required', 'auth_service_unavailable', 'auth_user_rejected',
  'invalid_key', 'oidc_not_configured', 'oidc_disabled', 'site_invalid_id',
  'site_duplicate', 'downloader_invalid_input', 'plugin_market_unreachable',
  'plugin_not_installed', 'plugin_market_sources_unreadable',
  'plugin_trust_config_unreadable', 'registration_not_found', 'cloud_drive_not_found',
  'strm_rule_not_found', 'filter_rule_not_found', 'filter_group_not_found',
  'file_storage_not_found', 'ops_credential_not_found', 'transfer_history_not_found',
  'ops_config_not_found', 'monitor_alert_not_found', 'notice_not_found',
  'contract_not_found', 'risk_confirmation_required', 'monitor_target_not_found',
  'site_auth_rejected', 'agent_disabled', 'agent_no_api_key', 'agent_key_unreadable',
  'agent_config_unreadable', 'docker_local_not_ready', 'docker_container_name_required',
  'auth_token_failed', 'ticket_invalid_or_expired', 'disk_invalid_mountpoint',
  'torrent_hash_required', 'media_library_required', 'url_required',
  'servarr_create_failed', 'backup_read_failed', 'media_keyword_required',
  'media_source_id_required', 'docker_image_ref_required', 'compose_dir_invalid',
  'compose_file_not_found', 'compose_write_failed', 'name_required',
  'resource_state_invalid', 'dependency_missing', 'unauthorized_op', 'upstream_failed',
  'backup_write_failed', 'plugin_action_failed', 'plugin_state_change_rejected',
  'database_query_failed', 'trakt_upstream_failed',
]

describe('映射表覆盖度（#356）', () => {
  it('后端「使用 ≥2 次」的码都要有英文文案', () => {
    const missing = HIGH_FREQ_CODES.filter((c) => !errcodeI18n.en[c])
    expect(missing, `这些高频错误码没有英文文案：${missing.join(', ')}`).toEqual([])
  })

  it('英文文案不许含中文（否则等于没翻）', () => {
    const zh = Object.entries(errcodeI18n.en)
      .filter(([, v]) => /[一-龥]/.test(v))
      .map(([k]) => k)
    expect(zh, `这些英文文案里混进了中文：${zh.join(', ')}`).toEqual([])
  })

  // #376 口径从「≥2 次」放宽到「≥1 次」。
  //
  // 原取舍（2026-10）写在 HIGH_FREQ_CODES 的注释里，但那个统计早就过期了：
  // 此后 #366–#375 又新增了若干码，而更根本的是 —— **用 1 次就意味着这条错误真的会
  // 出现在某个用户面前**，英文界面下它会原样显示后端中文，与 #355 要治的正是同一件事。
  // 「2 次」是当时为了控制工作量的经验阈值，不是正确性边界。
  //
  // 下面这份清单是静态扫 backend/internal（剔 _test.go）里 `errcode.<标识符>` 的实际
  // 引用得到的：每个码引用 ≥1 次。查出：87 个当时未映射，现已补齐。
  //
  // 仍然**故意不补**的：`downloader_push_failed` —— 注册了但全仓引用 0 次，
  // 也就是这条错误码根本不会出现在任何响应里；补英文只是噪音。
  const REACHABLE_CODES = [
    'conflict', 'unavailable', 'admin_required', 'not_activated', 'vip_unknown',
    'vip_expired', 'vip_check_disabled', 'challenge_mode', 'auth_server_busy',
    'auth_ticket_failed', 'auth_captcha_failed', 'auth_bad_credentials',
    'auth_cookie_required', 'demo_disabled', 'apikey_no_refresh', 'apikey_no_logout',
    'oidc_state_mismatch', 'oidc_provider_unreachable', 'oidc_login_failed',
    'value_too_large', 'value_invalid_json', 'ssrf_blocked', 'forbidden_sql',
    'write_sql_forbidden', 'multi_statement_blocked', 'file_not_found',
    'backup_password_required', 'backup_password_incorrect', 'backup_not_our_file',
    'backup_file_invalid', 'backup_file_too_large', 'backup_file_corrupt',
    'backup_version_too_new', 'backup_not_moviepilot_file', 'site_inspect_unreachable',
    'upstream_timeout', 'media_image_too_large', 'media_cover_no_image',
    'media_search_unconfigured', 'ocr_service_unreachable', 'ocr_service_bad_answer',
    'webhook_invalid', 'webhook_bad_payload', 'webhook_bad_signature',
    'webhook_name_required', 'webhook_secret_unavailable',
    'entertainment_game_unsupported', 'entertainment_game_not_found',
    'entertainment_spend_failed', 'entertainment_data_unreadable',
    'entertainment_credential_unreadable', 'entertainment_disabled',
    'plugin_not_reloadable', 'plugin_not_in_market', 'plugin_instance_not_found',
    'plugin_disabled', 'plugin_config_unreadable', 'plugin_not_installable',
    'plugin_package_untrusted', 'plugin_package_invalid', 'plugin_install_failed',
    'plugin_state_not_cleared', 'plugin_release_not_found', 'plugin_release_mismatch',
    'plugin_rollback_no_snapshot', 'plugin_root_unconfigured',
    'plugin_rollback_restore_failed', 'plugin_rollback_ledger_unwritten',
    'plugin_permissions_pending', 'plugin_single_instance', 'subtitle_sources_failed',
    'strm_record_not_found', 'contract_rules_unreadable', 'brushflow_tasks_unreadable',
    'brushflow_seeds_unreadable', 'brushflow_downloader_no_tags',
    'docker_engine_unavailable', 'cloud_token_unreadable', 'cloud_op_upstream_failed',
    'trakt_not_configured', 'trakt_not_authorized', 'trakt_code_expired',
    'agent_name_required', 'agent_type_invalid', 'agent_conversation_not_found',
    'agent_message_required', 'agent_confirm_gone', 'duplicate_spend',
  ]

  it('后端「用得到」的码都要有英文文案（#376：口径 ≥1 次）', () => {
    const missing = REACHABLE_CODES.filter((c) => !errcodeI18n.en[c])
    expect(missing, `这些后端在用、却会让英文界面显示中文的码缺英文：${missing.join(', ')}`).toEqual([])
  })

  it('不许出现「翻译了等于没翻译」的兜底话术', () => {
    // 判据只拦真正的空话（「Request failed.」「An error occurred.」）——
    // 它把用户知道的「请求失败了」原样还回去，什么都没增加。
    //
    // 刻意**不**按长度一刀切：第一版写成「少于 4 个词就算短」，结果把
    // `Name is required.` / `Invalid site ID.` 这类一起判红 —— 它们虽然短，
    // 但已经把「下一步做什么」说全了（去填名字 / 这个 ID 不对）。过宽判据会逼人
    // 把正确的短句注水成废话，那比短句更糟。
    const empty = Object.entries(errcodeI18n.en)
      .filter(([, v]) =>
        /^(request|operation|action)\s+failed\.?$/i.test(v.trim()) ||
        /^an error (occurred|happened)\.?$/i.test(v.trim()) ||
        /^(error|failed)\.?$/i.test(v.trim()),
      )
      .map(([k, v]) => `${k}: ${v}`)
    expect(empty, `这些英文文案是空话，等于没翻译：\n${empty.join('\n')}`).toEqual([])
  })
})

