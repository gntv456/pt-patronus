// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter/material.dart';
import 'package:ptpatronus/api/media.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';

// 媒体源设置（对齐 Web MediaSettings）：TMDB/豆瓣/Bangumi 凭据。
// secret 回显脱敏 + configured 标记；提交时空字段=不修改（可只改部分）。保存即时 rebuild，无需重启。
class MediaSettingsPage extends StatefulWidget {
  const MediaSettingsPage({super.key});
  @override
  State<MediaSettingsPage> createState() => _MediaSettingsPageState();
}

class _MediaSettingsPageState extends State<MediaSettingsPage> {
  final _api = MediaConfigApi();
  MediaSourceConfig? _config;
  bool _loading = true;
  bool _saving = false;
  String? _error;

  // 编辑态：secret 留空=不修改；域名预填当前值便于原地编辑。
  final _tmdbKey = TextEditingController();
  final _tmdbDomain = TextEditingController();
  final _doubanSecret = TextEditingController();
  final _doubanAppKey = TextEditingController();
  final _bangumiToken = TextEditingController();

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _tmdbKey.dispose();
    _tmdbDomain.dispose();
    _doubanSecret.dispose();
    _doubanAppKey.dispose();
    _bangumiToken.dispose();
    super.dispose();
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      _config = await _api.get();
      _tmdbDomain.text = _config?.tmdbApiDomain ?? '';
    } catch (e) {
      _error = '$e';
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _save() async {
    setState(() => _saving = true);
    try {
      await _api.set(
        tmdbApiKey: _tmdbKey.text.trim(),
        tmdbApiDomain: _tmdbDomain.text.trim(),
        doubanSecret: _doubanSecret.text.trim(),
        doubanAppKey: _doubanAppKey.text.trim(),
        bangumiToken: _bangumiToken.text.trim(),
      );
      _toast('已保存，即时生效');
      // 清空 secret 输入（已入库），域名保留回显
      _tmdbKey.clear();
      _doubanSecret.clear();
      _doubanAppKey.clear();
      _bangumiToken.clear();
      await _load();
    } catch (e) {
      _toast('$e');
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return GlassBackground(
      child: Scaffold(
        backgroundColor: Colors.transparent,
        appBar: AppBar(
          title: const Text('媒体源设置'),
          backgroundColor: Colors.transparent,
          elevation: 0,
          actions: [
            IconButton(
              icon: const Icon(Icons.refresh),
              tooltip: '刷新',
              onPressed: _loading ? null : _load,
            ),
          ],
        ),
        body: SafeArea(
          child: _loading
              ? const PTLoadingView()
              : _error != null
                  ? _errorView
                  : _body,
        ),
      ),
    );
  }

  Widget get _errorView => Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text('加载失败：$_error',
                textAlign: TextAlign.center,
                style: const TextStyle(color: Colors.redAccent)),
            const SizedBox(height: 12),
            FilledButton(onPressed: _load, child: const Text('重试')),
          ],
        ),
      );

  Widget get _body {
    final c = _config!;
    return ListView(
      padding: const EdgeInsets.fromLTRB(12, 4, 12, 32),
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(4, 0, 4, 12),
          child: GlassCard(
            padding: const EdgeInsets.all(14),
            child: Text(
              '影视搜索 / 推荐从这里取 TMDB、豆瓣、Bangumi 的凭据。留空 = 沿用默认（不修改）；'
              '国内访问 api.themoviedb.org 超时时，可在「TMDB 域名 / 镜像」填一个可达镜像。'
              '保存后即时生效，无需重启。',
              style: TextStyle(fontSize: 13, height: 1.7, color: context.dimText),
            ),
          ),
        ),
        _group('TMDB', '影视主源 · 国际片元数据', [
          _secretField(
            label: 'API Key',
            ctrl: _tmdbKey,
            configured: c.tmdbApiKeyConfigured,
            masked: c.tmdbApiKey,
            unconfigured: '未配置',
          ),
          _plainField(
            label: '域名 / 镜像',
            ctrl: _tmdbDomain,
            hint: 'api.themoviedb.org',
            help: '官方 api.themoviedb.org 国内常被墙；可换镜像',
          ),
        ]),
        _group('豆瓣', '国产影视 · 零配置可用（公开默认凭据）', [
          _secretField(
            label: 'Secret（签名密钥）',
            ctrl: _doubanSecret,
            configured: c.doubanSecretConfigured,
            masked: c.doubanSecret,
            placeholder: '留空用公开默认',
            unconfigured: '未配置（用公开默认）',
          ),
          _secretField(
            label: 'AppKey',
            ctrl: _doubanAppKey,
            configured: c.doubanAppKeyConfigured,
            masked: c.doubanAppKey,
            placeholder: '留空用公开默认',
            unconfigured: '未配置（用公开默认）',
          ),
        ]),
        _group('Bangumi', '动漫 · 匿名可用（可选 token 提速）', [
          _secretField(
            label: 'Token（可选）',
            ctrl: _bangumiToken,
            configured: c.bangumiTokenConfigured,
            masked: c.bangumiToken,
            placeholder: '留空走匿名配额',
            unconfigured: '未配置（匿名）',
          ),
        ]),
        const SizedBox(height: 8),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 4),
          child: FilledButton.icon(
            onPressed: _saving ? null : _save,
            icon: _saving
                ? const SizedBox(
                    width: 18,
                    height: 18,
                    child: CircularProgressIndicator(strokeWidth: 2))
                : const Icon(Icons.save_outlined),
            label: const Text('保存（即时生效）'),
          ),
        ),
      ],
    );
  }

  Widget _group(String title, String subtitle, List<Widget> children) {
    return Padding(
      padding: const EdgeInsets.only(top: 12),
      child: GlassCard(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(title,
                style: const TextStyle(fontSize: 15, fontWeight: FontWeight.w700)),
            const SizedBox(height: 2),
            Text(subtitle,
                style: TextStyle(fontSize: 12, color: context.dimText)),
            const SizedBox(height: 14),
            ...children,
          ],
        ),
      ),
    );
  }

  // secret 字段：密码态输入 + 右上配置态彩签（已配置显脱敏值 / 未配置显提示）。
  Widget _secretField({
    required String label,
    required TextEditingController ctrl,
    required bool configured,
    required String masked,
    String placeholder = '留空不修改',
    String unconfigured = '未配置',
  }) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Expanded(
                child: Text(label,
                    style: const TextStyle(
                        fontSize: 13, fontWeight: FontWeight.w600)),
              ),
              _statusChip(configured, configured ? '已配置 $masked' : unconfigured),
            ],
          ),
          const SizedBox(height: 6),
          TextField(
            controller: ctrl,
            obscureText: true,
            decoration: InputDecoration(
              hintText: placeholder,
              isDense: true,
              border: OutlineInputBorder(borderRadius: BorderRadius.circular(12)),
            ),
          ),
        ],
      ),
    );
  }

  // 明文字段（如 TMDB 域名）：输入 + 下方灰色说明。
  Widget _plainField({
    required String label,
    required TextEditingController ctrl,
    String hint = '',
    String help = '',
  }) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(label,
            style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w600)),
        const SizedBox(height: 6),
        TextField(
          controller: ctrl,
          keyboardType: TextInputType.url,
          decoration: InputDecoration(
            hintText: hint,
            isDense: true,
            border: OutlineInputBorder(borderRadius: BorderRadius.circular(12)),
          ),
        ),
        if (help.isNotEmpty)
          Padding(
            padding: const EdgeInsets.only(top: 4),
            child: Text(help,
                style: TextStyle(fontSize: 11, color: context.dimText)),
          ),
      ],
    );
  }

  Widget _statusChip(bool configured, String text) {
    final color = configured ? PTColors.success : PTColors.warn;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 9, vertical: 2),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.16),
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(text,
          style: TextStyle(
              color: color, fontSize: 11, fontWeight: FontWeight.w600)),
    );
  }
}
