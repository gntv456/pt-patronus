import 'package:flutter/material.dart';
import 'package:ptpatronus/api/brushflow.dart';
import 'package:ptpatronus/api/downloader.dart';
import 'package:ptpatronus/api/site.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/glass_icon.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';

/// 刷流引擎页（对齐 Web BrushFlow.vue）：任务卡片列表 + 新建/编辑表单 +
/// 选种 / 扫删 / 历史种子 / 删除。复用后端 /brushflow/*。
class BrushFlowPage extends StatefulWidget {
  const BrushFlowPage({super.key});
  @override
  State<BrushFlowPage> createState() => _BrushFlowPageState();
}

class _BrushFlowPageState extends State<BrushFlowPage> {
  final _api = BrushFlowApi();
  final _siteApi = SiteApi();
  final _dlApi = DownloaderApi();

  List<BrushFlowTask> _list = [];
  bool _loading = true;
  String? _error;

  // 站点 / 下载器名查表缓存（拉取一次，表单下拉 + 卡片 meta 共用）。
  List<Site> _sites = [];
  List<Downloader> _downloaders = [];

  // 选种 / 扫删进行中的任务 id，禁用对应按钮。
  final _busy = <int>{};

  @override
  void initState() {
    super.initState();
    _load();
    _ensureRefs();
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

  Future<void> _ensureRefs() async {
    if (_sites.isNotEmpty && _downloaders.isNotEmpty) return;
    try {
      final results = await Future.wait([_siteApi.list(), _dlApi.list()]);
      if (!mounted) return;
      setState(() {
        _sites = results[0] as List<Site>;
        _downloaders = results[1] as List<Downloader>;
      });
    } catch (_) {
      // 名称解析回退到「#id」，不阻塞主流程。
    }
  }

  String _siteName(int id) {
    for (final s in _sites) {
      if (s.id == id) return s.name;
    }
    return '站点 #$id';
  }

  String _dlName(int id) {
    for (final d in _downloaders) {
      if (d.id == id) return d.name;
    }
    return '下载器 #$id';
  }

  String _fmtBytes(int n) {
    if (n <= 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    double s = n.toDouble();
    int i = 0;
    while (s >= 1024 && i < units.length - 1) {
      s /= 1024;
      i++;
    }
    return i <= 1 ? '${s.toInt()} ${units[i]}' : '${s.toStringAsFixed(1)} ${units[i]}';
  }

  String _fmtDate(DateTime? d) {
    if (d == null) return '-';
    String two(int v) => v.toString().padLeft(2, '0');
    return '${d.year}-${two(d.month)}-${two(d.day)} ${two(d.hour)}:${two(d.minute)}';
  }

  void _toast(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
  }

  // 切换启用：乐观更新 + 失败回滚。
  Future<void> _toggleEnabled(BrushFlowTask t, bool v) async {
    setState(() {
      _list = _list
          .map((e) => e.id == t.id ? e.copyWith(enabled: v) : e)
          .toList();
    });
    try {
      await _api.update(t.id, {'enabled': v});
    } catch (e) {
      _toast('更新失败：$e');
      await _load();
    }
  }

  Future<void> _select(BrushFlowTask t) async {
    setState(() => _busy.add(t.id));
    try {
      final r = await _api.select(t.id);
      _toast((r['message'] as String?) ?? '选种完成');
      await _load();
    } catch (e) {
      _toast('选种失败：$e');
    } finally {
      if (mounted) setState(() => _busy.remove(t.id));
    }
  }

  Future<void> _sweep(BrushFlowTask t) async {
    setState(() => _busy.add(t.id));
    try {
      final r = await _api.sweep(t.id);
      _toast((r['message'] as String?) ?? '扫删完成');
      await _load();
    } catch (e) {
      _toast('扫删失败：$e');
    } finally {
      if (mounted) setState(() => _busy.remove(t.id));
    }
  }

  Future<void> _delete(BrushFlowTask t) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('删除刷流任务'),
        content: Text('确认删除任务「${t.name}」？此操作不可撤销。'),
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
      await _api.remove(t.id);
      _toast('已删除');
      await _load();
    } catch (e) {
      _toast('删除失败：$e');
    }
  }

  Future<void> _showSeeds(BrushFlowTask t) async {
    List<BrushFlowSeed> seeds = [];
    String? err;
    try {
      seeds = await _api.seeds(t.id);
    } catch (e) {
      err = '$e';
    }
    if (!mounted) return;
    await showAdaptiveSheet(
      context,
      title: '种子历史 · ${t.name}',
      builder: (_) {
        if (err != null) {
          return Padding(
            padding: const EdgeInsets.all(16),
            child: Text(
              '加载失败：$err',
              style: const TextStyle(color: PTColors.danger, fontSize: 13),
            ),
          );
        }
        if (seeds.isEmpty) {
          return const Padding(
            padding: EdgeInsets.symmetric(vertical: 24),
            child: Center(
              child: Text('暂无种子记录', style: TextStyle(color: Colors.grey)),
            ),
          );
        }
        return Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          mainAxisSize: MainAxisSize.min,
          children: [
            for (final s in seeds)
              ListTile(
                dense: true,
                contentPadding: EdgeInsets.zero,
                title: Text(
                  s.title.isEmpty ? s.sourceKey : s.title,
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(fontSize: 13),
                ),
                subtitle: Text(
                  '${_fmtBytes(s.size)}  ·  加入 ${_fmtDate(s.addedAt)}',
                  style: const TextStyle(fontSize: 12, color: Colors.grey),
                ),
              ),
          ],
        );
      },
    );
  }

  Future<void> _openCreate() async {
    await _ensureRefs();
    if (!mounted) return;
    await showAdaptiveSheet(
      context,
      title: '新建刷流任务',
      builder: (_) => _BrushFlowForm(
        sites: _sites,
        downloaders: _downloaders,
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

  Future<void> _openEdit(BrushFlowTask t) async {
    await _ensureRefs();
    if (!mounted) return;
    await showAdaptiveSheet(
      context,
      title: '编辑刷流任务',
      builder: (_) => _BrushFlowForm(
        sites: _sites,
        downloaders: _downloaders,
        initial: t,
        onSubmit: (body) async {
          try {
            await _api.update(t.id, body);
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

  @override
  Widget build(BuildContext context) {
    return PTPageScaffold(
      title: '刷流',
      actions: [
        PTBarAction(
          icon: Icons.refresh,
          label: '刷新',
          loading: _loading,
          onPressed: _loading ? null : _load,
        ),
        PTBarAction(
          icon: Icons.add,
          label: '新建',
          onPressed: _openCreate,
        ),
      ],
      body: Column(
        children: [
          // 刷新已有列表时顶部细条；首次加载在 _content 居中 spinner。
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
        icon: Icons.bolt_outlined,
        text: '暂无刷流任务',
        description: '点右上「新建」创建第一个刷流任务',
      );
    }
    return PTMasonry<BrushFlowTask>(
      items: _list,
      minItemWidth: 300,
      padding: const EdgeInsets.fromLTRB(12, 8, 12, 16),
      itemBuilder: (_, t) => _taskCard(t),
    );
  }

  Widget _taskCard(BrushFlowTask t) {
    final busy = _busy.contains(t.id);
    return GlassCard(
      padding: const EdgeInsets.all(14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // 头部：图标 + 名称 + 站点→下载器 meta + 启用开关。
          Row(
            children: [
              const GlassIcon(
                glyph: 'brushflow',
                from: Color(0xFFF7971E),
                to: Color(0xFFFF6A00),
                size: 32,
              ),
              const SizedBox(width: 10),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      t.name,
                      style: const TextStyle(
                        fontWeight: FontWeight.w700,
                        fontSize: 15,
                      ),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                    ),
                    const SizedBox(height: 2),
                    Text(
                      '${_siteName(t.siteId)} → ${_dlName(t.downloaderId)}',
                      style: const TextStyle(color: Colors.grey, fontSize: 12),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                    ),
                  ],
                ),
              ),
              Switch(value: t.enabled, onChanged: (v) => _toggleEnabled(t, v)),
            ],
          ),
          const SizedBox(height: 10),
          // 统计三格：加入 / 删除 / 上传。
          Row(
            children: [
              _statCell('加入', '${t.totalAdded}'),
              _statCell('删除', '${t.totalDeleted}'),
              _statCell('上传', _fmtBytes(t.totalUpload)),
            ],
          ),
          if (t.lastError.isNotEmpty) ...[
            const SizedBox(height: 8),
            Text(
              t.lastError,
              style: const TextStyle(color: PTColors.danger, fontSize: 12),
              maxLines: 2,
              overflow: TextOverflow.ellipsis,
            ),
          ],
          const SizedBox(height: 10),
          // 操作按钮行：选种 / 扫删 / 历史 / 编辑 / 删除。
          Wrap(
            spacing: 6,
            runSpacing: 4,
            children: [
              FilledButton.tonal(
                onPressed: busy ? null : () => _select(t),
                child: const Text('选种', style: TextStyle(fontSize: 13)),
              ),
              FilledButton.tonal(
                onPressed: busy ? null : () => _sweep(t),
                child: const Text('扫删', style: TextStyle(fontSize: 13)),
              ),
              TextButton(
                onPressed: () => _showSeeds(t),
                child: const Text('历史', style: TextStyle(fontSize: 13)),
              ),
              TextButton(
                onPressed: () => _openEdit(t),
                child: const Text('编辑', style: TextStyle(fontSize: 13)),
              ),
              TextButton(
                onPressed: () => _delete(t),
                child: const Text(
                  '删除',
                  style: TextStyle(fontSize: 13, color: PTColors.danger),
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }

  Widget _statCell(String label, String value) {
    return Expanded(
      child: Column(
        children: [
          Text(
            value,
            style: const TextStyle(
              fontWeight: FontWeight.w700,
              fontSize: 16,
              color: PTColors.primary,
            ),
          ),
          const SizedBox(height: 2),
          Text(label, style: const TextStyle(color: Colors.grey, fontSize: 12)),
        ],
      ),
    );
  }
}

/// 新建 / 编辑刷流任务的表单（放进 showAdaptiveSheet）。
/// 自持所有 controller / 下拉 / 开关状态；[onSubmit] 返回 true 时表单自关闭。
class _BrushFlowForm extends StatefulWidget {
  final List<Site> sites;
  final List<Downloader> downloaders;
  final BrushFlowTask? initial;
  final Future<bool> Function(Map<String, dynamic> body) onSubmit;

  const _BrushFlowForm({
    required this.sites,
    required this.downloaders,
    required this.onSubmit,
    this.initial,
  });

  @override
  State<_BrushFlowForm> createState() => _BrushFlowFormState();
}

class _BrushFlowFormState extends State<_BrushFlowForm> {
  // 文本字段控制器（initial 非空 = 编辑，预填）。
  late final _nameCtrl = TextEditingController(text: widget.initial?.name ?? '');
  late final _keywordCtrl =
      TextEditingController(text: widget.initial?.keyword ?? '');
  late final _savePathCtrl =
      TextEditingController(text: widget.initial?.savePath ?? '');
  late final _categoryDlCtrl =
      TextEditingController(text: widget.initial?.categoryDl ?? '');
  late final _minSizeCtrl = TextEditingController(
      text: widget.initial == null ? '' : '${widget.initial!.minSizeMb}');
  late final _maxSizeCtrl = TextEditingController(
      text: widget.initial == null ? '' : '${widget.initial!.maxSizeMb}');
  late final _minSeedersCtrl = TextEditingController(
      text: widget.initial == null ? '' : '${widget.initial!.minSeeders}');
  late final _maxSeedersCtrl = TextEditingController(
      text: widget.initial == null ? '' : '${widget.initial!.maxSeeders}');
  late final _delRatioCtrl = TextEditingController(
      text: widget.initial == null ? '' : '${widget.initial!.deleteRatio}');
  late final _delHoursCtrl = TextEditingController(
      text: widget.initial == null ? '' : '${widget.initial!.deleteSeedHours}');
  late final _delUpMbCtrl = TextEditingController(
      text: widget.initial == null ? '' : '${widget.initial!.deleteUploadMb}');
  late final _maxSeedsCtrl = TextEditingController(
      text: widget.initial == null ? '' : '${widget.initial!.maxSeeds}');

  // 下拉 / 开关状态。
  int? _siteId;
  int? _dlId;
  String _category = '综合';
  bool _enabled = true;
  bool _freeOnly = false;
  bool _paused = false;
  bool _deleteFiles = false;
  bool _saving = false;

  static const _categories = ['综合', '电影', '电视剧', '音乐'];

  @override
  void initState() {
    super.initState();
    final ini = widget.initial;
    if (ini != null) {
      _siteId = widget.sites.any((s) => s.id == ini.siteId) ? ini.siteId : null;
      _dlId = widget.downloaders.any((d) => d.id == ini.downloaderId)
          ? ini.downloaderId
          : null;
      _category =
          ini.category.isNotEmpty ? ini.category : '综合';
      _enabled = ini.enabled;
      _freeOnly = ini.freeOnly;
      _paused = ini.paused;
      _deleteFiles = ini.deleteFiles;
    } else {
      _siteId = widget.sites.isNotEmpty ? widget.sites.first.id : null;
      _dlId =
          widget.downloaders.isNotEmpty ? widget.downloaders.first.id : null;
    }
  }

  @override
  void dispose() {
    _nameCtrl.dispose();
    _keywordCtrl.dispose();
    _savePathCtrl.dispose();
    _categoryDlCtrl.dispose();
    _minSizeCtrl.dispose();
    _maxSizeCtrl.dispose();
    _minSeedersCtrl.dispose();
    _maxSeedersCtrl.dispose();
    _delRatioCtrl.dispose();
    _delHoursCtrl.dispose();
    _delUpMbCtrl.dispose();
    _maxSeedsCtrl.dispose();
    super.dispose();
  }

  int _parseInt(String s, [int def = 0]) => int.tryParse(s.trim()) ?? def;
  double _parseDouble(String s, [double def = 0]) =>
      double.tryParse(s.trim()) ?? def;

  void _toast(String msg) {
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
  }

  Future<void> _submit() async {
    if (_nameCtrl.text.trim().isEmpty) {
      _toast('请输入任务名称');
      return;
    }
    if (_siteId == null) {
      _toast('请选择站点');
      return;
    }
    if (_dlId == null) {
      _toast('请选择下载器');
      return;
    }
    setState(() => _saving = true);
    final body = <String, dynamic>{
      'name': _nameCtrl.text.trim(),
      'site_id': _siteId,
      'downloader_id': _dlId,
      'enabled': _enabled,
      'keyword': _keywordCtrl.text.trim(),
      'category': _category,
      'min_size_mb': _parseInt(_minSizeCtrl.text),
      'max_size_mb': _parseInt(_maxSizeCtrl.text),
      'min_seeders': _parseInt(_minSeedersCtrl.text),
      'max_seeders': _parseInt(_maxSeedersCtrl.text),
      'free_only': _freeOnly,
      'paused': _paused,
      'delete_ratio': _parseDouble(_delRatioCtrl.text),
      'delete_seed_hours': _parseDouble(_delHoursCtrl.text),
      'delete_upload_mb': _parseInt(_delUpMbCtrl.text),
      'delete_files': _deleteFiles,
      'max_seeds': _parseInt(_maxSeedsCtrl.text),
      'save_path': _savePathCtrl.text.trim(),
      'category_dl': _categoryDlCtrl.text.trim(),
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
        _field('任务名称', TextField(
          controller: _nameCtrl,
          decoration: const InputDecoration(
            isDense: true,
            hintText: '如：馒头刷流',
            border: OutlineInputBorder(),
          ),
        )),
        _field(
          '站点',
          DropdownButton<int>(
            value: _siteId,
            isExpanded: true,
            hint: const Text('选择站点'),
            items: widget.sites
                .map((s) => DropdownMenuItem(value: s.id, child: Text(s.name)))
                .toList(),
            onChanged: (v) => setState(() => _siteId = v),
          ),
        ),
        _field(
          '下载器',
          DropdownButton<int>(
            value: _dlId,
            isExpanded: true,
            hint: const Text('选择下载器'),
            items: widget.downloaders
                .map((d) => DropdownMenuItem(value: d.id, child: Text(d.name)))
                .toList(),
            onChanged: (v) => setState(() => _dlId = v),
          ),
        ),
        _field(
          '分类',
          DropdownButton<String>(
            value: _category,
            isExpanded: true,
            items: _categories
                .map((c) => DropdownMenuItem(value: c, child: Text(c)))
                .toList(),
            onChanged: (v) => setState(() => _category = v ?? '综合'),
          ),
        ),
        _field('关键字', TextField(
          controller: _keywordCtrl,
          decoration: const InputDecoration(
            isDense: true,
            hintText: '标题关键字（留空=不限）',
            border: OutlineInputBorder(),
          ),
        )),
        _field(
          '大小范围（MB，留空=不限）',
          Row(
            children: [
              Expanded(
                child: TextField(
                  controller: _minSizeCtrl,
                  keyboardType: TextInputType.number,
                  decoration: const InputDecoration(
                    isDense: true,
                    hintText: '最小',
                    border: OutlineInputBorder(),
                  ),
                ),
              ),
              const Padding(
                padding: EdgeInsets.symmetric(horizontal: 8),
                child: Text('—'),
              ),
              Expanded(
                child: TextField(
                  controller: _maxSizeCtrl,
                  keyboardType: TextInputType.number,
                  decoration: const InputDecoration(
                    isDense: true,
                    hintText: '最大',
                    border: OutlineInputBorder(),
                  ),
                ),
              ),
            ],
          ),
        ),
        _field(
          '做种人数范围（留空=不限）',
          Row(
            children: [
              Expanded(
                child: TextField(
                  controller: _minSeedersCtrl,
                  keyboardType: TextInputType.number,
                  decoration: const InputDecoration(
                    isDense: true,
                    hintText: '最少',
                    border: OutlineInputBorder(),
                  ),
                ),
              ),
              const Padding(
                padding: EdgeInsets.symmetric(horizontal: 8),
                child: Text('—'),
              ),
              Expanded(
                child: TextField(
                  controller: _maxSeedersCtrl,
                  keyboardType: TextInputType.number,
                  decoration: const InputDecoration(
                    isDense: true,
                    hintText: '最多',
                    border: OutlineInputBorder(),
                  ),
                ),
              ),
            ],
          ),
        ),
        _switchRow('启用', _enabled, (v) => setState(() => _enabled = v)),
        _switchRow('仅免费', _freeOnly, (v) => setState(() => _freeOnly = v)),
        _switchRow(
            '添加后暂停', _paused, (v) => setState(() => _paused = v)),
        const Divider(height: 20),
        const Text(
          '删种规则',
          style: TextStyle(fontWeight: FontWeight.w600, fontSize: 13),
        ),
        const SizedBox(height: 8),
        _field(
          '分享率上限 / 保种小时 / 上传 MB（留空=不限）',
          Row(
            children: [
              Expanded(
                child: TextField(
                  controller: _delRatioCtrl,
                  keyboardType: const TextInputType.numberWithOptions(decimal: true),
                  decoration: const InputDecoration(
                    isDense: true,
                    hintText: '分享率',
                    border: OutlineInputBorder(),
                  ),
                ),
              ),
              const SizedBox(width: 8),
              Expanded(
                child: TextField(
                  controller: _delHoursCtrl,
                  keyboardType: const TextInputType.numberWithOptions(decimal: true),
                  decoration: const InputDecoration(
                    isDense: true,
                    hintText: '小时',
                    border: OutlineInputBorder(),
                  ),
                ),
              ),
              const SizedBox(width: 8),
              Expanded(
                child: TextField(
                  controller: _delUpMbCtrl,
                  keyboardType: TextInputType.number,
                  decoration: const InputDecoration(
                    isDense: true,
                    hintText: '上传MB',
                    border: OutlineInputBorder(),
                  ),
                ),
              ),
            ],
          ),
        ),
        _switchRow('删种同时删除文件', _deleteFiles,
            (v) => setState(() => _deleteFiles = v)),
        _field('最大做种数（留空=不限）', TextField(
          controller: _maxSeedsCtrl,
          keyboardType: TextInputType.number,
          decoration: const InputDecoration(
            isDense: true,
            hintText: '如：20',
            border: OutlineInputBorder(),
          ),
        )),
        _field('保存路径（留空=下载器默认）', TextField(
          controller: _savePathCtrl,
          decoration: const InputDecoration(
            isDense: true,
            hintText: '/downloads/brush',
            border: OutlineInputBorder(),
          ),
        )),
        _field('下载器分类（留空=不限）', TextField(
          controller: _categoryDlCtrl,
          decoration: const InputDecoration(
            isDense: true,
            hintText: '如：brush',
            border: OutlineInputBorder(),
          ),
        )),
        const SizedBox(height: 12),
        FilledButton.icon(
          onPressed: _saving ? null : _submit,
          icon: _saving
              ? const SizedBox(
                  width: 16,
                  height: 16,
                  child: CircularProgressIndicator(strokeWidth: 2),
                )
              : const Icon(Icons.save_outlined, size: 18),
          label: Text(widget.initial == null ? '创建任务' : '保存修改'),
        ),
      ],
    );
  }

  /// 统一的「标签 + 控件」字段行。
  Widget _field(String label, Widget child) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            label,
            style: const TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.w600,
              color: Colors.grey,
            ),
          ),
          const SizedBox(height: 6),
          child,
        ],
      ),
    );
  }

  /// 统一的开关行。
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
