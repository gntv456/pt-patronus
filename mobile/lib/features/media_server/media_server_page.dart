import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:ptpatronus/api/media_server.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/glass_icon.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';

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
      return const Center(child: CircularProgressIndicator());
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
                      style: const TextStyle(color: Colors.grey, fontSize: 12),
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
        color: accent ? PTColors.success.withValues(alpha: 0.18) : Colors.grey.withValues(alpha: 0.16),
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

/// 媒体库列表 + 每库扫描/刷新/封面预览。
class _LibrariesBody extends StatefulWidget {
  final int serverId;
  final List<MediaLibrary> libs;
  final MediaServerApi api;
  const _LibrariesBody({required this.serverId, required this.libs, required this.api});
  @override
  State<_LibrariesBody> createState() => _LibrariesBodyState();
}

class _LibrariesBodyState extends State<_LibrariesBody> {
  final _busy = <String>{}; // libraryId 正在操作
  Uint8List? _cover;
  bool _coverLoading = false;

  void _toast(String msg) =>
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));

  Future<void> _scan(MediaLibrary lib, {bool all = false}) async {
    final key = all ? '__all__' : lib.id;
    setState(() => _busy.add(key));
    try {
      final r = await widget.api.scan(widget.serverId, all ? '' : lib.id);
      _toast((r['message'] as String?) ?? (all ? '已触发全库扫描' : '已扫描：${lib.name}'));
    } catch (e) {
      _toast('扫描失败：$e');
    } finally {
      if (mounted) setState(() => _busy.remove(key));
    }
  }

  Future<void> _refresh(MediaLibrary lib) async {
    setState(() => _busy.add(lib.id));
    try {
      final r = await widget.api.refresh(widget.serverId, lib.id);
      _toast((r['message'] as String?) ?? '已刷新：${lib.name}');
    } catch (e) {
      _toast('刷新失败：$e');
    } finally {
      if (mounted) setState(() => _busy.remove(lib.id));
    }
  }

  Future<void> _cover_(MediaLibrary lib) async {
    setState(() {
      _coverLoading = true;
      _cover = null;
    });
    try {
      final bytes = await widget.api.cover(widget.serverId, lib.id);
      if (mounted) setState(() => _cover = bytes);
    } catch (e) {
      _toast('封面生成失败：$e');
    } finally {
      if (mounted) setState(() => _coverLoading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final webhook = 'POST /api/v1/mediaserver/${widget.serverId}/webhook';
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      mainAxisSize: MainAxisSize.min,
      children: [
        Row(
          children: [
            TextButton.icon(
              icon: const Icon(Icons.refresh, size: 16),
              label: const Text('全库扫描'),
              onPressed: _busy.contains('__all__') ? null : () => _scan(widget.libs.isNotEmpty ? widget.libs.first : MediaLibrary(id: '', name: '', type: ''), all: true),
            ),
          ],
        ),
        Padding(
          padding: const EdgeInsets.only(bottom: 8),
          child: Text('播放回调：$webhook', style: const TextStyle(fontSize: 11, color: Colors.grey)),
        ),
        for (final lib in widget.libs)
          Padding(
            padding: const EdgeInsets.only(bottom: 8),
            child: Row(
              children: [
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(lib.name, style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w600)),
                      Text(lib.type.isEmpty ? '库 #${lib.id}' : '${lib.type} · #${lib.id}',
                          style: const TextStyle(fontSize: 11, color: Colors.grey)),
                    ],
                  ),
                ),
                if (_busy.contains(lib.id))
                  const Padding(
                    padding: EdgeInsets.only(right: 8),
                    child: SizedBox(width: 14, height: 14, child: CircularProgressIndicator(strokeWidth: 2)),
                  ),
                TextButton(onPressed: _busy.contains(lib.id) ? null : () => _scan(lib), child: const Text('扫描')),
                TextButton(onPressed: _busy.contains(lib.id) ? null : () => _refresh(lib), child: const Text('刷新')),
                TextButton(onPressed: _busy.contains(lib.id) ? null : () => _cover_(lib), child: const Text('封面')),
              ],
            ),
          ),
        if (_coverLoading)
          const Padding(padding: EdgeInsets.all(16), child: Center(child: CircularProgressIndicator())),
        if (_cover != null)
          Padding(
            padding: const EdgeInsets.only(top: 8),
            child: ClipRRect(
              borderRadius: BorderRadius.circular(8),
              child: Image.memory(_cover!, fit: BoxFit.contain),
            ),
          ),
      ],
    );
  }
}

/// 添加 / 编辑表单。
class _MediaServerForm extends StatefulWidget {
  final MediaServer? initial;
  final Future<bool> Function(Map<String, dynamic>) onSubmit;
  const _MediaServerForm({this.initial, required this.onSubmit});
  @override
  State<_MediaServerForm> createState() => _MediaServerFormState();
}

class _MediaServerFormState extends State<_MediaServerForm> {
  late final _nameCtrl = TextEditingController(text: widget.initial?.name ?? '');
  late final _urlCtrl = TextEditingController(text: widget.initial?.url ?? '');
  late final _tokenCtrl = TextEditingController(); // 编辑时不回显秘密
  late final _userCtrl = TextEditingController(text: widget.initial?.username ?? '');
  late final _directLinkBaseCtrl =
      TextEditingController(text: widget.initial?.directLinkBase ?? '');
  // late 字段惰性求值，可在初始化式引用 widget.initial（同 brushflow 表单范式）。
  late String _type = widget.initial?.type ?? 'emby';
  late bool _isDefault = widget.initial?.isDefault ?? false;
  late bool _refreshOnDownload = widget.initial?.refreshOnDownload ?? true;
  late bool _notifyOnPlay = widget.initial?.notifyOnPlay ?? false;
  late bool _directLinkEnabled = widget.initial?.directLinkEnabled ?? false;
  bool _saving = false;

  @override
  void dispose() {
    _nameCtrl.dispose();
    _urlCtrl.dispose();
    _tokenCtrl.dispose();
    _userCtrl.dispose();
    _directLinkBaseCtrl.dispose();
    super.dispose();
  }

  void _toast(String msg) =>
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));

  Future<void> _submit() async {
    if (_nameCtrl.text.trim().isEmpty) {
      _toast('请输入名称');
      return;
    }
    if (_urlCtrl.text.trim().isEmpty) {
      _toast('请输入服务器地址');
      return;
    }
    if (widget.initial == null && _tokenCtrl.text.trim().isEmpty) {
      _toast('新建需填写 API Key / Token');
      return;
    }
    setState(() => _saving = true);
    final body = <String, dynamic>{
      'name': _nameCtrl.text.trim(),
      'type': _type,
      'url': _urlCtrl.text.trim(),
      'username': _userCtrl.text.trim(),
      'is_default': _isDefault,
      'refresh_on_download': _refreshOnDownload,
      'notify_on_play': _notifyOnPlay,
      'direct_link_enabled': _directLinkEnabled,
      'direct_link_base': _directLinkBaseCtrl.text.trim(),
      if (_tokenCtrl.text.trim().isNotEmpty) 'token': _tokenCtrl.text.trim(),
    };
    final ok = await widget.onSubmit(body);
    if (!mounted) return;
    setState(() => _saving = false);
    if (ok) Navigator.of(context).pop();
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      mainAxisSize: MainAxisSize.min,
      children: [
        _field('名称', TextField(
          controller: _nameCtrl,
          decoration: const InputDecoration(isDense: true, hintText: '如：客厅 Emby', border: OutlineInputBorder()),
        )),
        _field('类型', DropdownButton<String>(
          value: _type,
          isExpanded: true,
          items: const [
            DropdownMenuItem(value: 'emby', child: Text('Emby')),
            DropdownMenuItem(value: 'jellyfin', child: Text('Jellyfin')),
            DropdownMenuItem(value: 'plex', child: Text('Plex')),
          ],
          onChanged: (v) => setState(() => _type = v ?? 'emby'),
        )),
        _field('地址', TextField(
          controller: _urlCtrl,
          decoration: const InputDecoration(isDense: true, hintText: 'http://192.168.1.10:8096', border: OutlineInputBorder()),
        )),
        _field('API Key / Token', TextField(
          controller: _tokenCtrl,
          obscureText: true,
          decoration: InputDecoration(
            isDense: true,
            hintText: widget.initial == null ? 'Emby/JF API Key 或 Plex X-Plex-Token' : '留空=保留原凭据',
            border: const OutlineInputBorder(),
          ),
        )),
        _field('用户名（可选）', TextField(
          controller: _userCtrl,
          decoration: const InputDecoration(isDense: true, hintText: 'Emby/JF 管理员（通知目标）', border: OutlineInputBorder()),
        )),
        _switchRow('设为默认', _isDefault, (v) => setState(() => _isDefault = v)),
        _switchRow('下载完成刷新', _refreshOnDownload, (v) => setState(() => _refreshOnDownload = v)),
        _switchRow('播放通知', _notifyOnPlay, (v) => setState(() => _notifyOnPlay = v)),
        _switchRow('启用直链反代', _directLinkEnabled, (v) => setState(() => _directLinkEnabled = v)),
        if (_directLinkEnabled)
          _field('直链基址', TextField(
            controller: _directLinkBaseCtrl,
            decoration: const InputDecoration(
              isDense: true,
              hintText: 'https://cdn.example.com/library',
              border: OutlineInputBorder(),
            ),
          )),
        const SizedBox(height: 8),
        FilledButton(
          onPressed: _saving ? null : _submit,
          child: _saving
              ? const SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white))
              : const Text('保存'),
        ),
      ],
    );
  }

  Widget _field(String label, Widget child) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(label, style: const TextStyle(fontSize: 12, fontWeight: FontWeight.w600, color: Colors.grey)),
          const SizedBox(height: 6),
          child,
        ],
      ),
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

/// 观影报告 sheet 主体：加载 + 展示用户/热门/最近播放。
class _PlaybackReportBody extends StatefulWidget {
  final int serverId;
  final MediaServerApi api;
  const _PlaybackReportBody({required this.serverId, required this.api});
  @override
  State<_PlaybackReportBody> createState() => _PlaybackReportBodyState();
}

class _PlaybackReportBodyState extends State<_PlaybackReportBody> {
  static const _days = 30;
  PlaybackReport? _report;
  bool _loading = true;
  String? _error;

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
      _report = await widget.api.playbackReport(widget.serverId, days: _days);
    } catch (e) {
      _error = '$e';
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  String _fmtDur(int sec) {
    if (sec <= 0) return '-';
    if (sec < 3600) return '${(sec / 60).round()} 分钟';
    return '${(sec / 3600).toStringAsFixed(1)} 小时';
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) return const Center(child: CircularProgressIndicator());
    if (_error != null) {
      return PTEmptyState(text: '加载失败：$_error', onRetry: _load);
    }
    final r = _report;
    if (r == null) return const PTEmptyState(text: '无数据');
    return RefreshIndicator(
      onRefresh: _load,
      child: ListView(padding: const EdgeInsets.all(12), children: [
        Text('近 $_days 天 · 共 ${r.totalPlays} 次播放',
            style: const TextStyle(fontSize: 13, color: Colors.grey)),
        const SizedBox(height: 8),
        const Text('用户', style: TextStyle(fontSize: 14, fontWeight: FontWeight.w700)),
        const SizedBox(height: 4),
        if (r.users.isEmpty)
          const Text('无用户数据', style: TextStyle(fontSize: 12, color: Colors.grey))
        else
          Wrap(
            spacing: 6,
            children: r.users
                .map((u) => Chip(
                      label: Text('${u.name} · ${u.playCount} 次', style: const TextStyle(fontSize: 12)),
                    ))
                .toList(),
          ),
        const SizedBox(height: 12),
        const Text('热门条目', style: TextStyle(fontSize: 14, fontWeight: FontWeight.w700)),
        ...r.topItems.map((p) => ListTile(
              dense: true,
              contentPadding: EdgeInsets.zero,
              title: Text(p.name, style: const TextStyle(fontSize: 13)),
              trailing: Text('${p.playCount} 次 · ${_fmtDur(p.duration)}',
                  style: const TextStyle(fontSize: 12, color: Colors.grey)),
            )),
        const SizedBox(height: 8),
        const Text('最近播放', style: TextStyle(fontSize: 14, fontWeight: FontWeight.w700)),
        ...r.recentPlays.map((p) => ListTile(
              dense: true,
              contentPadding: EdgeInsets.zero,
              title: Text(p.name, style: const TextStyle(fontSize: 13)),
              subtitle: Text(
                p.lastPlayed.isEmpty
                    ? p.userName
                    : '${p.userName} · ${DateTime.tryParse(p.lastPlayed)?.toLocal() ?? p.lastPlayed}',
                style: const TextStyle(fontSize: 11, color: Colors.grey),
              ),
              trailing: Text('${p.playCount} 次', style: const TextStyle(fontSize: 12, color: Colors.grey)),
            )),
      ]),
    );
  }
}

/// 重复检测 sheet 主体：选库 → 检测 → 展示重复组。
class _DuplicatesBody extends StatefulWidget {
  final int serverId;
  final MediaServerApi api;
  const _DuplicatesBody({required this.serverId, required this.api});
  @override
  State<_DuplicatesBody> createState() => _DuplicatesBodyState();
}

class _DuplicatesBodyState extends State<_DuplicatesBody> {
  List<MediaLibrary> _libs = const [];
  String? _libId;
  List<DuplicateGroup> _groups = const [];
  bool _loading = true;
  bool _checking = false;

  @override
  void initState() {
    super.initState();
    _loadLibs();
  }

  Future<void> _loadLibs() async {
    try {
      _libs = await widget.api.libraries(widget.serverId);
      _libId = _libs.isNotEmpty ? _libs.first.id : null;
    } catch (_) {}
    if (mounted) setState(() => _loading = false);
  }

  Future<void> _run() async {
    if (_libId == null) return;
    setState(() => _checking = true);
    try {
      _groups = await widget.api.duplicates(widget.serverId, _libId!);
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('查重失败：$e')));
      }
    } finally {
      if (mounted) setState(() => _checking = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) return const Center(child: CircularProgressIndicator());
    return Column(mainAxisSize: MainAxisSize.min, crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      Row(children: [
        Expanded(
          child: DropdownButton<String>(
            value: _libId,
            isExpanded: true,
            hint: const Text('选择媒体库'),
            items: _libs
                .map((l) => DropdownMenuItem(value: l.id, child: Text(l.name)))
                .toList(),
            onChanged: (v) => setState(() => _libId = v),
          ),
        ),
        const SizedBox(width: 8),
        FilledButton(onPressed: _checking ? null : _run, child: const Text('检测')),
      ]),
      const SizedBox(height: 8),
      Flexible(
        child: _checking
            ? const Center(child: CircularProgressIndicator())
            : ListView(shrinkWrap: true, children: [
                Text('共 ${_groups.length} 组疑似重复',
                    style: const TextStyle(fontSize: 12, color: Colors.grey)),
                ..._groups.map((g) => Card(
                      margin: const EdgeInsets.symmetric(vertical: 4),
                      child: Padding(
                        padding: const EdgeInsets.all(8),
                        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                          Row(children: [
                            Expanded(
                              child: Text('${g.title}${g.year > 0 ? ' (${g.year})' : ''}',
                                  style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w600)),
                            ),
                            Text('${g.count} 份',
                                style: const TextStyle(fontSize: 12, color: Colors.orange)),
                          ]),
                          ...g.items.map((n) => Text('· $n',
                              style: const TextStyle(fontSize: 11, color: Colors.grey))),
                        ]),
                      ),
                    )),
              ]),
      ),
    ]);
  }
}
