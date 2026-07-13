import 'dart:io';

import 'package:flutter/material.dart';
import 'package:path_provider/path_provider.dart';
import 'package:ptpatronus/api/subtitle.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/glass_icon.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';
import 'package:share_plus/share_plus.dart';

/// 字幕页（对齐 Web Subtitle.vue）：跨 ASSRT/OpenSubtitles/SubHD/字幕库 聚合搜索 + 下载 + 源配置。
class SubtitlePage extends StatefulWidget {
  const SubtitlePage({super.key});
  @override
  State<SubtitlePage> createState() => _SubtitlePageState();
}

class _SubtitlePageState extends State<SubtitlePage> {
  final _api = SubtitleApi();
  final _qCtrl = TextEditingController();

  List<SubtitleResult> _results = [];
  bool _loading = false;
  bool _searched = false;
  String _error = ''; // 搜索失败文案；非空时显示错误态而非「未找到字幕」（区分失败与无果）
  String _language = '';
  final _downloading = <String>{};

  @override
  void dispose() {
    _qCtrl.dispose();
    super.dispose();
  }

  void _toast(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
  }

  Future<void> _search() async {
    final q = _qCtrl.text.trim();
    if (q.isEmpty) {
      _toast('请输入搜索词');
      return;
    }
    setState(() {
      _loading = true;
      _searched = true;
      _error = '';
    });
    try {
      final res = await _api.search(q, language: _language);
      if (!mounted) return;
      setState(() => _results = res);
      if (res.isEmpty) _toast('未找到字幕');
    } catch (e) {
      _toast('搜索失败：$e');
      if (mounted) {
        setState(() {
          _results = [];
          _error = e.toString();
          _searched = false; // 失败时不显示「未找到」，由 _error 态承接
        });
      }
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _download(SubtitleResult sub) async {
    final key = '${sub.source}:${sub.sourceId}';
    setState(() => _downloading.add(key));
    try {
      final bytes = await _api.download(sub);
      // 写临时文件并分享（用户存到下载/媒体服务器目录）
      final dir = await getTemporaryDirectory();
      final name = _ensureExt(sub.fileName?.isNotEmpty == true ? sub.fileName! : (sub.title.isNotEmpty ? sub.title : 'subtitle'));
      final file = File('${dir.path}/$name');
      await file.writeAsBytes(bytes);
      await Share.shareXFiles([XFile(file.path)]);
      _toast('已下载 $name');
    } catch (e) {
      _toast('下载失败：$e');
    } finally {
      if (mounted) setState(() => _downloading.remove(key));
    }
  }

  String _ensureExt(String name) {
    final lower = name.toLowerCase();
    for (final e in const ['.srt', '.ass', '.ssa', '.vtt']) {
      if (lower.endsWith(e)) return name;
    }
    return '$name.srt';
  }

  Future<void> _openConfig() async {
    SubtitleConfig? cfg;
    try {
      cfg = await _api.getConfig();
    } catch (e) {
      _toast('加载配置失败：$e');
      return;
    }
    if (!mounted) return;
    await showAdaptiveSheet(
      context,
      title: '字幕源设置',
      builder: (_) => _ConfigForm(
        cfg: cfg!,
        onSubmit: (body) async {
          try {
            await _api.setConfig(body);
            _toast('已保存');
            return true;
          } catch (e) {
            _toast('保存失败：$e');
            return false;
          }
        },
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return PTPageScaffold(
      title: '字幕',
      actions: [
        PTBarAction(icon: Icons.tune, label: '源设置', onPressed: _openConfig),
      ],
      body: Column(
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
            child: Row(children: [
              Expanded(
                child: TextField(
                  controller: _qCtrl,
                  decoration: const InputDecoration(
                    isDense: true,
                    hintText: '搜索电影 / 剧集名',
                    prefixIcon: Icon(Icons.search, size: 18),
                    border: OutlineInputBorder(),
                  ),
                  onSubmitted: (_) => _search(),
                ),
              ),
              const SizedBox(width: 8),
              DropdownButton<String>(
                value: _language,
                items: const [
                  DropdownMenuItem(value: '', child: Text('全部')),
                  DropdownMenuItem(value: 'chs', child: Text('简体')),
                  DropdownMenuItem(value: 'cht', child: Text('繁体')),
                  DropdownMenuItem(value: 'eng', child: Text('英文')),
                ],
                onChanged: (v) => setState(() => _language = v ?? ''),
              ),
              const SizedBox(width: 8),
              FilledButton(onPressed: _loading ? null : _search, child: const Text('搜索')),
            ]),
          ),
          if (_loading) const LinearProgressIndicator(minHeight: 2),
          Expanded(child: _content()),
        ],
      ),
    );
  }

  Widget _content() {
    if (_error.isNotEmpty) {
      return Center(
        child: Padding(
          padding: const EdgeInsets.all(20),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(Icons.error_outline, size: 40, color: Colors.redAccent),
              const SizedBox(height: 8),
              Text('搜索失败：$_error', textAlign: TextAlign.center,
                  style: const TextStyle(color: Colors.redAccent, fontSize: 13)),
              const SizedBox(height: 10),
              OutlinedButton(onPressed: _search, child: const Text('重试')),
            ],
          ),
        ),
      );
    }
    if (_results.isEmpty) {
      return PTEmptyState(
        icon: Icons.subtitles_outlined,
        text: _searched ? '未找到字幕' : '搜索字幕',
        description: _searched ? '试试更换关键词；部分源需在「源设置」配置 token/cookie' : '输入电影/剧集名，跨源聚合搜索',
      );
    }
    return ListView.separated(
      padding: const EdgeInsets.fromLTRB(12, 4, 12, 16),
      itemCount: _results.length,
      separatorBuilder: (_, _) => const Divider(height: 1),
      itemBuilder: (_, i) => _resultTile(_results[i]),
    );
  }

  Widget _resultTile(SubtitleResult s) {
    final key = '${s.source}:${s.sourceId}';
    return ListTile(
      leading: const GlassIcon(glyph: 'subtitle', from: Color(0xFF4361EE), to: Color(0xFF7209B7), size: 30),
      title: Text(s.title, style: const TextStyle(fontSize: 13), maxLines: 2, overflow: TextOverflow.ellipsis),
      subtitle: Row(children: [
        _tag(subtitleSourceLabel[s.source] ?? s.source),
        const SizedBox(width: 4),
        _tag(subtitleLangLabel[s.langCode] ?? s.language, accent: true),
      ]),
      trailing: _downloading.contains(key)
          ? const SizedBox(width: 18, height: 18, child: CircularProgressIndicator(strokeWidth: 2))
          : TextButton.icon(
              icon: const Icon(Icons.download_outlined, size: 16),
              label: const Text('下载'),
              onPressed: () => _download(s),
            ),
    );
  }

  Widget _tag(String text, {bool accent = false}) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: accent ? PTColors.success.withValues(alpha: 0.18) : Colors.grey.withValues(alpha: 0.16),
        borderRadius: BorderRadius.circular(6),
      ),
      child: Text(text, style: const TextStyle(fontSize: 11, fontWeight: FontWeight.w600)),
    );
  }
}

/// 字幕源配置表单。
class _ConfigForm extends StatefulWidget {
  final SubtitleConfig cfg;
  final Future<bool> Function(Map<String, dynamic>) onSubmit;
  const _ConfigForm({required this.cfg, required this.onSubmit});
  @override
  State<_ConfigForm> createState() => _ConfigFormState();
}

class _ConfigFormState extends State<_ConfigForm> {
  late final _assrtCtrl = TextEditingController();
  late final _osdCtrl = TextEditingController();
  late final _subhdCtrl = TextEditingController();
  late final _zimukuCtrl = TextEditingController();
  late String _target = widget.cfg.targetLang;
  bool _saving = false;

  @override
  void dispose() {
    _assrtCtrl.dispose();
    _osdCtrl.dispose();
    _subhdCtrl.dispose();
    _zimukuCtrl.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    setState(() => _saving = true);
    final body = <String, dynamic>{
      if (_assrtCtrl.text.trim().isNotEmpty) 'assrt_token': _assrtCtrl.text.trim(),
      if (_osdCtrl.text.trim().isNotEmpty) 'opensubtitles_key': _osdCtrl.text.trim(),
      if (_subhdCtrl.text.trim().isNotEmpty) 'subhd_cookie': _subhdCtrl.text.trim(),
      if (_zimukuCtrl.text.trim().isNotEmpty) 'zimuku_cookie': _zimukuCtrl.text.trim(),
      'target_lang': _target,
    };
    final ok = await widget.onSubmit(body);
    if (!mounted) return;
    setState(() => _saving = false);
    if (ok) Navigator.of(context).pop();
  }

  @override
  Widget build(BuildContext context) {
    return SingleChildScrollView(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        mainAxisSize: MainAxisSize.min,
        children: [
          _field('ASSRT Token', TextField(
            controller: _assrtCtrl,
            obscureText: true,
            decoration: InputDecoration(isDense: true, hintText: widget.cfg.assrtConfigured ? '已配置（留空=不修改）' : 'assrt.net token', border: const OutlineInputBorder()),
          )),
          _field('OpenSubtitles Key', TextField(
            controller: _osdCtrl,
            obscureText: true,
            decoration: InputDecoration(isDense: true, hintText: widget.cfg.opensubtitlesConfigured ? '已配置（留空=不修改）' : 'Api-Key', border: const OutlineInputBorder()),
          )),
          _field('SubHD Cookie', TextField(
            controller: _subhdCtrl,
            maxLines: 2,
            decoration: InputDecoration(isDense: true, hintText: widget.cfg.subhdConfigured ? '已配置（留空=不修改）' : '可选，绕过反爬', border: const OutlineInputBorder()),
          )),
          _field('字幕库 Cookie', TextField(
            controller: _zimukuCtrl,
            maxLines: 2,
            decoration: InputDecoration(isDense: true, hintText: widget.cfg.zimukuConfigured ? '已配置（留空=不修改）' : '可选，绕过反爬', border: const OutlineInputBorder()),
          )),
          _field('繁简转换', DropdownButton<String>(
            value: _target,
            isExpanded: true,
            items: const [
              DropdownMenuItem(value: '', child: Text('不转换')),
              DropdownMenuItem(value: 'chs', child: Text('转为简体')),
              DropdownMenuItem(value: 'cht', child: Text('转为繁体')),
            ],
            onChanged: (v) => setState(() => _target = v ?? ''),
          )),
          const SizedBox(height: 8),
          FilledButton(
            onPressed: _saving ? null : _submit,
            child: _saving
                ? const SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white))
                : const Text('保存'),
          ),
        ],
      ),
    );
  }

  Widget _field(String label, Widget child) => Padding(
        padding: const EdgeInsets.only(bottom: 12),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Text(label, style: const TextStyle(fontSize: 12, fontWeight: FontWeight.w600, color: Colors.grey)),
          const SizedBox(height: 6),
          child,
        ]),
      );
}
