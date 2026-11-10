// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter/material.dart';
import 'package:ptpatronus/api/cookiecloud.dart';
import 'package:ptpatronus/api/dirwatch.dart';
import 'package:ptpatronus/api/llm_gateway.dart';
import 'package:ptpatronus/api/tgbot.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/glass_icon.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';
import '../system/logs_page.dart';

part 'llm_gateway_card.dart';
part 'tg_bot_card.dart';
part 'dirwatch_card.dart';
part 'cookie_cloud_card.dart';

/// 系统设置页：聚合三个管理员级配置卡片（后端均已存在）。
///  1. AI 网关（/llmgateway/config）—— 把内置 Agent 以 OpenAI/Anthropic 兼容协议暴露。
///  2. Telegram Bot（/tgbot/config）—— 入站 bot 配置。
///  3. 目录监视（/transfer/dirwatch）—— 本地目录 → 整理流水线。
///
/// 整页受 [ApiClient.isAdmin] 守卫；三卡初始化并发加载，各自独立保存（失败回滚+重载）。
/// 设计范式同 [MediaServerPage]：PTPageScaffold + GlassCard + 脱敏令牌（留空=保留）。
class SystemSettingsPage extends StatefulWidget {
  const SystemSettingsPage({super.key});
  @override
  State<SystemSettingsPage> createState() => _SystemSettingsPageState();
}

class _SystemSettingsPageState extends State<SystemSettingsPage> {
  final _llmApi = LlmGatewayApi();
  final _tgApi = TgBotApi();
  final _dirApi = DirwatchApi();
  final _ccApi = CookieCloudApi();

  LlmGatewayConfig? _llm;
  TgBotConfig? _tg;
  DirwatchConfig? _dir;
  CookieCloudConfig? _cc;
  bool _loading = true;
  String? _error;

  @override
  void initState() {
    super.initState();
    _loadAll();
  }

  /// 并发拉取三份配置；任一失败 → 全局错误态（带重试，对齐 MediaServerPage 单错误态范式）。
  Future<void> _loadAll() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final results = await Future.wait<dynamic>([
        _llmApi.get(),
        _tgApi.get(),
        _dirApi.get(),
        _ccApi.get(),
      ]);
      if (!mounted) return;
      _llm = results[0] as LlmGatewayConfig;
      _tg = results[1] as TgBotConfig;
      _dir = results[2] as DirwatchConfig;
      _cc = results[3] as CookieCloudConfig;
    } catch (e) {
      if (!mounted) return;
      _error = '$e';
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    // 非管理员：整页拦截（后端仍是权威鉴权，此处仅 UI 显隐）。
    if (!ApiClient().isAdmin) {
      return PTPageScaffold(
        title: '系统设置',
        body: const PTEmptyState(
          icon: Icons.admin_panel_settings_outlined,
          text: '仅管理员可访问',
        ),
      );
    }
    return PTPageScaffold(
      title: '系统设置',
      actions: [
        PTBarAction(
          icon: Icons.receipt_long_outlined,
          label: '日志',
          onPressed: () => Navigator.of(context).push(
            MaterialPageRoute(builder: (_) => const LogsPage()),
          ),
        ),
        PTBarAction(
          icon: Icons.refresh,
          label: '刷新',
          loading: _loading,
          onPressed: _loading ? null : _loadAll,
        ),
      ],
      body: Column(
        children: [
          if (_loading && _llm == null && _tg == null && _dir == null && _cc == null)
            const LinearProgressIndicator(minHeight: 2),
          Expanded(child: _content()),
        ],
      ),
    );
  }

  Widget _content() {
    if (_loading && _llm == null && _tg == null && _dir == null && _cc == null) {
      return const PTLoadingView();
    }
    if (_error != null && _llm == null && _tg == null && _dir == null && _cc == null) {
      return PTEmptyState(
        icon: Icons.error_outline,
        text: '加载失败',
        description: _error,
        onRetry: _loadAll,
      );
    }
    return RefreshIndicator(
      onRefresh: _loadAll,
      child: ListView(
        padding: const EdgeInsets.fromLTRB(12, 8, 12, 24),
        children: [
          if (_llm != null)
            _LlmGatewayCard(api: _llmApi, initial: _llm!)
          else
            _failedCard('AI 网关'),
          const SizedBox(height: 12),
          if (_tg != null)
            _TgBotCard(api: _tgApi, initial: _tg!)
          else
            _failedCard('Telegram Bot'),
          const SizedBox(height: 12),
          if (_dir != null)
            _DirwatchCard(api: _dirApi, initial: _dir!)
          else
            _failedCard('目录监视'),
          const SizedBox(height: 12),
          if (_cc != null)
            _CookieCloudCard(api: _ccApi, initial: _cc!)
          else
            _failedCard('CookieCloud'),
        ],
      ),
    );
  }

  Widget _failedCard(String name) => GlassCard(
        padding: const EdgeInsets.all(14),
        child: Row(
          children: [
            const Icon(Icons.error_outline, color: PTColors.danger, size: 18),
            const SizedBox(width: 8),
            Expanded(
              child: Text('$name 配置加载失败，点右上「刷新」重试',
                  style: const TextStyle(fontSize: 13, color: PTColors.danger)),
            ),
          ],
        ),
      );
}

// ─────────────────────────────────────────────────────────────────────────
// Card 1 — AI 网关
// ─────────────────────────────────────────────────────────────────────────

// ─────────────────────────────────────────────────────────────────────────
// Card 2 — Telegram Bot
// ─────────────────────────────────────────────────────────────────────────

// ─────────────────────────────────────────────────────────────────────────
// Card 3 — 目录监视
// ─────────────────────────────────────────────────────────────────────────

// ─────────────────────────────────────────────────────────────────────────
// Card 4 — CookieCloud 同步
// ─────────────────────────────────────────────────────────────────────────
