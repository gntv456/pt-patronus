import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:ptpatronus/api/media_server.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/glass_icon.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';

part 'libraries_body.dart';
part 'media_server_form.dart';
part 'playback_report_body.dart';
part 'duplicates_body.dart';

/// 媒体服务器页（对齐 Web MediaServer.vue）：Emby/Jellyfin/Plex 多实例 +
/// 连接测试 / 库扫描 / 刷新 / 封面生成 / 事件开关。复用后端 /mediaserver/*。
class MediaServerPage extends StatefulWidget {
  const MediaServerPage({super.key});
  @override
  State<MediaServerPage> createState() => _MediaServerPageState();
}

class _MediaServerPageState extends State<MediaServerPage> {
  final _api = MediaServerApi();

  List<MediaServer> _list = [];
  bool _loading = true;
  String? _error;
  final _testing = <int>{};

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final list = await _api.list();
      if (!mounted) return;
      setState(() => _list = list);
    } catch (e) {
      if (!mounted) return;
      setState(() => _error = '$e');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  void _toast(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
  }

  String _typeLabel(String t) => const {'emby': 'Emby', 'jellyfin': 'Jellyfin', 'plex': 'Plex'}[t] ?? t;

  Future<void> _test(MediaServer s) async {
    setState(() => _testing.add(s.id));
    try {
      final r = await _api.test(s.id);
      final ok = r['ok'] == true;
      _toast(ok ? '${s.name}：${r['info'] ?? '连接正常'}' : '连接失败：${r['error'] ?? '未知错误'}');
    } catch (e) {
      _toast('测试失败：$e');
    } finally {
      if (mounted) setState(() => _testing.remove(s.id));
    }
  }

  Future<void> _toggle(MediaServer s, bool v, String field) async {
    setState(() {
      _list = _list
          .map((e) => e.id == s.id
              ? e.copyWith(
                  refreshOnDownload: field == 'refresh_on_download' ? v : null,
                  notifyOnPlay: field == 'notify_on_play' ? v : null,
                )
              : e)
          .toList();
    });
    try {
      await _api.update(s.id, {field: v});
    } catch (e) {
      _toast('更新失败：$e');
      await _load();
    }
  }

  Future<void> _delete(MediaServer s) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('删除媒体服务器'),
        content: Text('确认删除「${s.name}」？此操作不可撤销。'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          FilledButton(
            style: FilledButton.styleFrom(backgroundColor: PTColors.danger),
            onPressed: () => Navigator.pop(context, true),
            child: const Text('删除'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await _api.remove(s.id);
      _toast('已删除');
      await _load();
    } catch (e) {
      _toast('删除失败：$e');
    }
  }

  Future<void> _openCreate() async {
    await showAdaptiveSheet(
      context,
      title: '添加媒体服务器',
      builder: (_) => _MediaServerForm(
        onSubmit: (body) async {
          try {
            await _api.create(body);
            _toast('已创建');
            await _load();
            return true;
          } catch (e) {
            _toast('创建失败：$e');
            return false;
          }
        },
      ),
    );
  }

  Future<void> _openEdit(MediaServer s) async {
    await showAdaptiveSheet(
      context,
      title: '编辑媒体服务器',
      builder: (_) => _MediaServerForm(
        initial: s,
        onSubmit: (body) async {
          try {
            await _api.update(s.id, body);
            _toast('已保存');
            await _load();
            return true;
          } catch (e) {
            _toast('保存失败：$e');
            return false;
          }
        },
      ),
    );
  }

  Future<void> _openLibs(MediaServer s) async {
    List<MediaLibrary> libs = [];
    String? err;
    try {
      libs = await _api.libraries(s.id);
    } catch (e) {
      err = '$e';
    }
    if (!mounted) return;
    await showAdaptiveSheet(
      context,
      title: '媒体库 · ${s.name}',
      builder: (_) {
        if (err != null) {
          return Padding(
            padding: const EdgeInsets.all(16),
            child: Text('加载失败：$err',
                style: const TextStyle(color: PTColors.danger, fontSize: 13)),
          );
        }
        return _LibrariesBody(serverId: s.id, libs: libs, api: _api);
      },
    );
  }

  /// 观影报告 sheet
  Future<void> _openReport(MediaServer s) async {
    await showAdaptiveSheet(
      context,
      title: '观影报告 · ${s.name}',
      builder: (_) => _PlaybackReportBody(serverId: s.id, api: _api),
    );
  }

  /// 重复检测 sheet
  Future<void> _openDup(MediaServer s) async {
    await showAdaptiveSheet(
      context,
      title: '重复检测 · ${s.name}',
      builder: (_) => _DuplicatesBody(serverId: s.id, api: _api),
    );
  }

  @override
  Widget build(BuildContext context) {
    return PTPageScaffold(
      title: '媒体服务器',
      actions: [
        PTBarAction(
          icon: Icons.refresh,
          label: '刷新',
          loading: _loading,
          onPressed: _loading ? null : _load,
        ),
        PTBarAction(
          icon: Icons.add,
          label: '添加',
          onPressed: _openCreate,
        ),
      ],
      body: Column(
        children: [
          if (_loading && _list.isNotEmpty) const LinearProgressIndicator(minHeight: 2),
          Expanded(child: _content()),
        ],
      ),
    );
  }

  Widget _content() {
    if (_loading && _list.isEmpty) {
      return const PTLoadingView();
    }
    if (_error != null && _list.isEmpty) {
      return PTEmptyState(
        icon: Icons.error_outline,
        text: '加载失败',
        description: _error,
        onRetry: _load,
      );
    }
    if (_list.isEmpty) {
      return const PTEmptyState(
        icon: Icons.video_library_outlined,
        text: '尚未添加媒体服务器',
        description: '点右上「添加」接入 Emby / Jellyfin / Plex',
      );
    }
    return RefreshIndicator(
      onRefresh: _load,
      child: PTMasonry<MediaServer>(
        items: _list,
        minItemWidth: 300,
        padding: const EdgeInsets.fromLTRB(12, 8, 12, 16),
        itemBuilder: (_, s) => _serverCard(s),
      ),
    );
  }

  Widget _serverCard(MediaServer s) {
    return GlassCard(
      padding: const EdgeInsets.all(14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              const GlassIcon(
                glyph: 'mediaserver',
                from: Color(0xFF52B788),
                to: Color(0xFF2D6A4F),
                size: 32,
              ),
              const SizedBox(width: 10),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        Flexible(
                          child: Text(
                            s.name,
                            style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 15),
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                          ),
                        ),
                        const SizedBox(width: 6),
                        _tag(_typeLabel(s.type)),
                        if (s.isDefault) _tag('默认', accent: true),
                      ],
                    ),
                    const SizedBox(height: 2),
                    Text(
                      s.url,
                      style: TextStyle(color: context.dimText, fontSize: 12),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                    ),
                  ],
                ),
              ),
            ],
          ),
          const SizedBox(height: 10),
          _switchRow('下载完成刷新', s.refreshOnDownload,
              (v) => _toggle(s, v, 'refresh_on_download')),
          _switchRow('播放通知', s.notifyOnPlay,
              (v) => _toggle(s, v, 'notify_on_play')),
          const SizedBox(height: 8),
          Wrap(
            spacing: 8,
            runSpacing: 8,
            children: [
              OutlinedButton.icon(
                icon: _testing.contains(s.id)
                    ? const SizedBox(width: 14, height: 14, child: CircularProgressIndicator(strokeWidth: 2))
                    : const Icon(Icons.cable, size: 16),
                label: const Text('连接测试'),
                onPressed: _testing.contains(s.id) ? null : () => _test(s),
              ),
              OutlinedButton.icon(
                icon: const Icon(Icons.video_library_outlined, size: 16),
                label: const Text('媒体库'),
                onPressed: () => _openLibs(s),
              ),
              OutlinedButton.icon(
                icon: const Icon(Icons.bar_chart, size: 16),
                label: const Text('观影报告'),
                onPressed: () => _openReport(s),
              ),
              OutlinedButton.icon(
                icon: const Icon(Icons.copy_all_outlined, size: 16),
                label: const Text('查重'),
                onPressed: () => _openDup(s),
              ),
              TextButton.icon(
                icon: const Icon(Icons.edit_outlined, size: 16),
                label: const Text('编辑'),
                onPressed: () => _openEdit(s),
              ),
              TextButton.icon(
                icon: Icon(Icons.delete_outline, size: 16, color: PTColors.danger),
                label: Text('删除', style: TextStyle(color: PTColors.danger)),
                onPressed: () => _delete(s),
              ),
            ],
          ),
        ],
      ),
    );
  }

  Widget _tag(String text, {bool accent = false}) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: accent ? PTColors.success.withValues(alpha: 0.18) : context.dimText.withValues(alpha: 0.16),
        borderRadius: BorderRadius.circular(6),
      ),
      child: Text(text, style: const TextStyle(fontSize: 11, fontWeight: FontWeight.w600)),
    );
  }

  Widget _switchRow(String label, bool value, ValueChanged<bool> onChanged) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 4),
      child: Row(
        children: [
          Expanded(child: Text(label, style: const TextStyle(fontSize: 13))),
          Switch(value: value, onChanged: onChanged),
        ],
      ),
    );
  }
}
