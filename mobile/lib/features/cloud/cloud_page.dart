import 'package:flutter/material.dart';
import 'package:ptpatronus/api/cloud.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/glass_icon.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';

/// 云盘 · STRM 页（对齐 Web Cloud.vue）：115/夸克/123 网盘 CRUD + 浏览 + STRM 规则 + 生成 + 订阅追更。
/// 三 Tab：网盘 / STRM 规则 / STRM 记录。复用后端 /cloud/*。
class CloudPage extends StatefulWidget {
  const CloudPage({super.key});
  @override
  State<CloudPage> createState() => _CloudPageState();
}

class _CloudPageState extends State<CloudPage> with SingleTickerProviderStateMixin {
  final _api = CloudApi();
  late final TabController _tab = TabController(length: 3, vsync: this);

  List<CloudDrive> _drives = [];
  List<StrmRule> _rules = [];
  List<StrmRecord> _records = [];
  bool _loading = false;
  String? _error;
  final _testing = <int>{};
  final _quotas = <int, String>{};

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _tab.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final results = await Future.wait([_api.listDrives(), _api.listRules(), _api.listRecords()]);
      if (!mounted) return;
      setState(() {
        _drives = results[0] as List<CloudDrive>;
        _rules = results[1] as List<StrmRule>;
        _records = results[2] as List<StrmRecord>;
      });
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

  String _driveName(int id) => _drives.any((d) => d.id == id)
      ? _drives.firstWhere((d) => d.id == id).name
      : '#$id';

  // ---- 网盘 ----
  Future<void> _test(CloudDrive d) async {
    setState(() => _testing.add(d.id));
    try {
      final r = await _api.test(d.id);
      final ok = r['ok'] == true;
      _toast(ok ? '${d.name}：${r['info'] ?? '连接正常'}' : '连接失败：${r['error'] ?? '未知错误'}');
    } catch (e) {
      _toast('测试失败：$e');
    } finally {
      if (mounted) setState(() => _testing.remove(d.id));
    }
  }

  Future<void> _quota(CloudDrive d) async {
    try {
      final q = await _api.quota(d.id);
      final used = (q?['used'] as num?)?.toInt() ?? 0;
      final total = (q?['total'] as num?)?.toInt() ?? 0;
      if (!mounted) return;
      setState(() => _quotas[d.id] = '${_fmtSize(used)} / ${_fmtSize(total)}');
    } catch (e) {
      _toast('获取容量失败：$e');
    }
  }

  Future<void> _deleteDrive(CloudDrive d) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('删除网盘'),
        content: Text('确认删除「${d.name}」？其下 STRM 规则与记录将一并清除。'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('取消')),
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
      await _api.removeDrive(d.id);
      _toast('已删除');
      await _load();
    } catch (e) {
      _toast('删除失败：$e');
    }
  }

  Future<void> _openCreateDrive() async {
    await showAdaptiveSheet(
      context,
      title: '添加网盘',
      builder: (_) => _DriveForm(
        onSubmit: (body) async {
          try {
            await _api.createDrive(body);
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

  Future<void> _openEditDrive(CloudDrive d) async {
    await showAdaptiveSheet(
      context,
      title: '编辑网盘',
      builder: (_) => _DriveForm(
        initial: d,
        onSubmit: (body) async {
          try {
            await _api.updateDrive(d.id, body);
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

  Future<void> _browse(CloudDrive d) async {
    await showAdaptiveSheet(
      context,
      title: '浏览 · ${d.name}',
      builder: (_) => _BrowseBody(drive: d, api: _api),
    );
  }

  // ---- 规则 ----
  Future<void> _generate(StrmRule r) async {
    _toast('正在生成…');
    try {
      final res = await _api.generateRule(r.id);
      final gen = res['generated'] ?? 0;
      final skip = res['skipped'] ?? 0;
      final errs = (res['errors'] as List?) ?? const [];
      _toast('${r.name}：新增 $gen，跳过 $skip${errs.isNotEmpty ? '，失败 ${errs.length}' : ''}');
      await _load();
    } catch (e) {
      _toast('生成失败：$e');
    }
  }

  Future<void> _generateAll() async {
    try {
      await _api.generateAll();
      _toast('已触发全部规则生成');
      await Future.delayed(const Duration(seconds: 1));
      await _load();
    } catch (e) {
      _toast('请求失败：$e');
    }
  }

  Future<void> _syncSubs() async {
    try {
      await _api.syncSubscriptions();
      _toast('订阅追更匹配完成');
      await _load();
    } catch (e) {
      _toast('请求失败：$e');
    }
  }

  Future<void> _deleteRule(StrmRule r) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('删除规则'),
        content: Text('确认删除规则「${r.name}」？'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('取消')),
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
      await _api.removeRule(r.id);
      _toast('已删除');
      await _load();
    } catch (e) {
      _toast('删除失败：$e');
    }
  }

  Future<void> _openCreateRule() async {
    if (_drives.isEmpty) {
      _toast('请先添加网盘');
      return;
    }
    await showAdaptiveSheet(
      context,
      title: '添加 STRM 规则',
      builder: (_) => _RuleForm(
        drives: _drives,
        onSubmit: (body) async {
          try {
            await _api.createRule(body);
            _toast('已创建');
            await _load();
            return true;
          } catch (e) {
            _toast('创建失败：$e');
            return false;
          }
        },
        onBrowse: (d) => _pickDir(d),
      ),
    );
  }

  Future<void> _openEditRule(StrmRule r) async {
    await showAdaptiveSheet(
      context,
      title: '编辑规则',
      builder: (_) => _RuleForm(
        drives: _drives,
        initial: r,
        onSubmit: (body) async {
          try {
            await _api.updateRule(r.id, body);
            _toast('已保存');
            await _load();
            return true;
          } catch (e) {
            _toast('保存失败：$e');
            return false;
          }
        },
        onBrowse: (d) => _pickDir(d),
      ),
    );
  }

  Future<String?> _pickDir(CloudDrive d) async {
    String? picked;
    await showAdaptiveSheet(
      context,
      title: '选择目录 · ${d.name}',
      builder: (_) => _BrowseBody(
        drive: d,
        api: _api,
        onPick: (p) {
          picked = p;
          Navigator.of(context).pop();
        },
      ),
    );
    return picked;
  }

  Future<void> _deleteRecord(StrmRecord r) async {
    try {
      await _api.removeRecord(r.id);
      await _load();
    } catch (e) {
      _toast('删除失败：$e');
    }
  }

  void _onAdd() {
    if (_tab.index == 0) {
      _openCreateDrive();
    } else if (_tab.index == 1) {
      _openCreateRule();
    } else {
      _toast('记录页无需添加');
    }
  }

  @override
  Widget build(BuildContext context) {
    return PTPageScaffold(
      title: '云盘 · STRM',
      actions: [
        PTBarAction(icon: Icons.refresh, label: '刷新', loading: _loading, onPressed: _loading ? null : _load),
        PTBarAction(icon: Icons.add, label: '添加', onPressed: _onAdd),
      ],
      body: Column(
        children: [
          if (_loading && _drives.isNotEmpty) const LinearProgressIndicator(minHeight: 2),
          Material(
            color: Colors.transparent,
            child: TabBar(
              controller: _tab,
              tabs: const [
                Tab(text: '网盘'),
                Tab(text: 'STRM 规则'),
                Tab(text: '记录'),
              ],
            ),
          ),
          Expanded(
            child: _error != null && _drives.isEmpty
                ? PTEmptyState(icon: Icons.error_outline, text: '加载失败', description: _error, onRetry: _load)
                : TabBarView(
                    controller: _tab,
                    children: [_drivesTab(), _rulesTab(), _recordsTab()],
                  ),
          ),
        ],
      ),
    );
  }

  // ---- Tabs ----
  Widget _drivesTab() {
    if (_drives.isEmpty) {
      return const PTEmptyState(
        icon: Icons.cloud_outlined,
        text: '尚未添加网盘',
        description: '点右上「添加」接入 115 / 夸克 / 123',
      );
    }
    return RefreshIndicator(
      onRefresh: _load,
      child: PTMasonry<CloudDrive>(
        items: _drives,
        minItemWidth: 300,
        padding: const EdgeInsets.fromLTRB(12, 8, 12, 16),
        itemBuilder: (_, d) => _driveCard(d),
      ),
    );
  }

  Widget _driveCard(CloudDrive d) {
    return GlassCard(
      padding: const EdgeInsets.all(14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(children: [
            const GlassIcon(glyph: 'cloud', from: Color(0xFF4CC9F0), to: Color(0xFF4361EE), size: 32),
            const SizedBox(width: 10),
            Expanded(
              child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Row(children: [
                  Flexible(
                    child: Text(d.name, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 15),
                        maxLines: 1, overflow: TextOverflow.ellipsis),
                  ),
                  const SizedBox(width: 6),
                  _tag(cloudTypeLabel[d.type] ?? d.type),
                  if (d.isDefault) _tag('默认', accent: true),
                ]),
                const SizedBox(height: 2),
                Text('根目录：${d.rootPath}', style: TextStyle(color: context.dimText, fontSize: 12), maxLines: 1, overflow: TextOverflow.ellipsis),
                if (_quotas[d.id] != null)
                  Text('容量：${_quotas[d.id]}', style: TextStyle(color: context.dimText, fontSize: 12)),
              ]),
            ),
          ]),
          const SizedBox(height: 10),
          Wrap(spacing: 8, runSpacing: 8, children: [
            OutlinedButton.icon(
              icon: _testing.contains(d.id)
                  ? const SizedBox(width: 14, height: 14, child: CircularProgressIndicator(strokeWidth: 2))
                  : const Icon(Icons.cable, size: 16),
              label: const Text('测试'),
              onPressed: _testing.contains(d.id) ? null : () => _test(d),
            ),
            OutlinedButton.icon(
              icon: const Icon(Icons.storage_outlined, size: 16),
              label: const Text('容量'),
              onPressed: () => _quota(d),
            ),
            OutlinedButton.icon(
              icon: const Icon(Icons.folder_open_outlined, size: 16),
              label: const Text('浏览'),
              onPressed: () => _browse(d),
            ),
            TextButton.icon(
              icon: const Icon(Icons.edit_outlined, size: 16),
              label: const Text('编辑'),
              onPressed: () => _openEditDrive(d),
            ),
            TextButton.icon(
              icon: Icon(Icons.delete_outline, size: 16, color: PTColors.danger),
              label: Text('删除', style: TextStyle(color: PTColors.danger)),
              onPressed: () => _deleteDrive(d),
            ),
          ]),
        ],
      ),
    );
  }

  Widget _rulesTab() {
    return Column(
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
          child: Row(children: [
            TextButton.icon(icon: const Icon(Icons.bolt_outlined, size: 16), label: const Text('生成全部'), onPressed: _generateAll),
            TextButton.icon(icon: const Icon(Icons.sync_outlined, size: 16), label: const Text('订阅追更'), onPressed: _syncSubs),
          ]),
        ),
        if (_rules.isEmpty)
          const Expanded(child: PTEmptyState(icon: Icons.rule_folder_outlined, text: '尚未配置 STRM 规则', description: '点右上「添加」创建规则'))
        else
          Expanded(
            child: ListView.builder(
              padding: const EdgeInsets.fromLTRB(12, 4, 12, 16),
              itemCount: _rules.length,
              itemBuilder: (_, i) => _ruleCard(_rules[i]),
            ),
          ),
      ],
    );
  }

  Widget _ruleCard(StrmRule r) {
    return Card(
      margin: const EdgeInsets.only(bottom: 8),
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Row(children: [
            Expanded(child: Text(r.name, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 14))),
            _tag(r.enabled ? '启用' : '停用', accent: r.enabled),
          ]),
          const SizedBox(height: 4),
          Text('网盘：${_driveName(r.driveId)} · ${r.cloudPath}', style: TextStyle(fontSize: 12, color: context.dimText), maxLines: 1, overflow: TextOverflow.ellipsis),
          Text('输出：${r.outputDir}', style: TextStyle(fontSize: 12, color: context.dimText), maxLines: 1, overflow: TextOverflow.ellipsis),
          if (r.mountRoot.isNotEmpty)
            Text('挂载：${r.mountRoot}', style: TextStyle(fontSize: 12, color: context.dimText), maxLines: 1, overflow: TextOverflow.ellipsis),
          const SizedBox(height: 8),
          Row(children: [
            TextButton.icon(icon: const Icon(Icons.play_arrow_outlined, size: 16), label: const Text('生成'), onPressed: () => _generate(r)),
            TextButton.icon(icon: const Icon(Icons.edit_outlined, size: 16), label: const Text('编辑'), onPressed: () => _openEditRule(r)),
            TextButton.icon(icon: Icon(Icons.delete_outline, size: 16, color: PTColors.danger), label: Text('删除', style: TextStyle(color: PTColors.danger)), onPressed: () => _deleteRule(r)),
          ]),
        ]),
      ),
    );
  }

  Widget _recordsTab() {
    if (_records.isEmpty) {
      return const PTEmptyState(icon: Icons.history_outlined, text: '暂无 STRM 记录', description: '生成 STRM 后此处显示已生成文件');
    }
    return ListView.separated(
      padding: const EdgeInsets.fromLTRB(12, 8, 12, 16),
      itemCount: _records.length,
      separatorBuilder: (_, _) => const Divider(height: 1),
      itemBuilder: (_, i) {
        final r = _records[i];
        return ListTile(
          leading: Icon(r.subscriptionId > 0 ? Icons.favorite : Icons.insert_drive_file_outlined, size: 22, color: context.dimText),
          title: Text(r.cloudPath, style: const TextStyle(fontSize: 13), maxLines: 1, overflow: TextOverflow.ellipsis),
          subtitle: Text('${_driveName(r.driveId)} · ${_fmtSize(r.size)}${r.subscriptionId > 0 ? ' · 追更' : ''}', style: TextStyle(fontSize: 11, color: context.dimText)),
          trailing: IconButton(
            tooltip: '删除该 STRM 记录',
            icon: Icon(Icons.delete_outline, size: 18, color: PTColors.danger),
            onPressed: () => _deleteRecord(r),
          ),
        );
      },
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
}

String _fmtSize(int n) {
  if (n <= 0) return '-';
  const g = 1 << 30;
  if (n >= g) return '${(n / g).toStringAsFixed(1)} GB';
  if (n >= 1 << 20) return '${(n / (1 << 20)).toStringAsFixed(1)} MB';
  return '${(n / (1 << 10)).toStringAsFixed(1)} KB';
}

/// 网盘目录浏览（可作纯浏览 or 选择目录回调）。
class _BrowseBody extends StatefulWidget {
  final CloudDrive drive;
  final CloudApi api;
  final void Function(String path)? onPick;
  const _BrowseBody({required this.drive, required this.api, this.onPick});
  @override
  State<_BrowseBody> createState() => _BrowseBodyState();
}

class _BrowseBodyState extends State<_BrowseBody> {
  String _path = '/';
  List<CloudFile> _files = [];
  bool _loading = false;

  @override
  void initState() {
    super.initState();
    _path = widget.drive.rootPath.isEmpty ? '/' : widget.drive.rootPath;
    _fetch();
  }

  Future<void> _fetch() async {
    setState(() => _loading = true);
    try {
      final files = await widget.api.browse(widget.drive.id, _path);
      if (mounted) setState(() => _files = files);
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('浏览失败：$e')));
        setState(() => _files = []);
      }
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  void _enter(CloudFile f) {
    if (!f.isDir) return;
    setState(() => _path = f.path);
    _fetch();
  }

  void _up() {
    if (_path == '/') return;
    final parts = _path.split('/').where((s) => s.isNotEmpty).toList()..removeLast();
    setState(() => _path = '/${parts.join('/')}');
    _fetch();
  }

  @override
  Widget build(BuildContext context) {
    final dirs = _files.where((f) => f.isDir).toList();
    final files = _files.where((f) => !f.isDir).toList();
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        Row(children: [
          TextButton.icon(icon: const Icon(Icons.arrow_upward, size: 16), label: const Text('上级'), onPressed: _path == '/' ? null : _up),
          Expanded(child: Text(_path, style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w600), maxLines: 1, overflow: TextOverflow.ellipsis)),
          if (widget.onPick != null)
            FilledButton(onPressed: () => widget.onPick!(_path), child: const Text('使用此目录')),
        ]),
        if (_loading) const Padding(padding: EdgeInsets.all(16), child: PTLoadingView()),
        if (!_loading)
          Flexible(
            child: ListView(shrinkWrap: true, children: [
              for (final d in dirs)
                ListTile(dense: true, leading: const Icon(Icons.folder, size: 20, color: Color(0xFF4CC9F0)), title: Text(d.name, style: const TextStyle(fontSize: 13)), onTap: () => _enter(d)),
              for (final f in files)
                ListTile(dense: true, leading: Icon(Icons.movie_outlined, size: 20, color: context.dimText), title: Text(f.name, style: const TextStyle(fontSize: 13), maxLines: 1, overflow: TextOverflow.ellipsis), trailing: Text(_fmtSize(f.size), style: TextStyle(fontSize: 11, color: context.dimText))),
              if (dirs.isEmpty && files.isEmpty)
                Padding(padding: const EdgeInsets.all(20), child: Center(child: Text('（空目录）', style: TextStyle(color: context.dimText)))),
            ]),
          ),
      ],
    );
  }
}

/// 网盘表单（添加/编辑）。
class _DriveForm extends StatefulWidget {
  final CloudDrive? initial;
  final Future<bool> Function(Map<String, dynamic>) onSubmit;
  const _DriveForm({this.initial, required this.onSubmit});
  @override
  State<_DriveForm> createState() => _DriveFormState();
}

class _DriveFormState extends State<_DriveForm> {
  late final _nameCtrl = TextEditingController(text: widget.initial?.name ?? '');
  late final _tokenCtrl = TextEditingController();
  late final _rootCtrl = TextEditingController(text: widget.initial?.rootPath ?? '/');
  late String _type = widget.initial?.type ?? '115';
  late bool _isDefault = widget.initial?.isDefault ?? false;
  bool _saving = false;

  @override
  void dispose() {
    _nameCtrl.dispose();
    _tokenCtrl.dispose();
    _rootCtrl.dispose();
    super.dispose();
  }

  void _toast(String msg) => ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));

  Future<void> _submit() async {
    if (_nameCtrl.text.trim().isEmpty) {
      _toast('请输入名称');
      return;
    }
    if (widget.initial == null && _tokenCtrl.text.trim().isEmpty) {
      _toast('新建需填写 Cookie / Token');
      return;
    }
    setState(() => _saving = true);
    final body = <String, dynamic>{
      'name': _nameCtrl.text.trim(),
      'type': _type,
      'root_path': _rootCtrl.text.trim(),
      'is_default': _isDefault,
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
        _field('名称', TextField(controller: _nameCtrl, decoration: const InputDecoration(isDense: true, hintText: '如：我的 115', border: OutlineInputBorder()))),
        _field('类型', DropdownButton<String>(
          value: _type,
          isExpanded: true,
          items: const [
            DropdownMenuItem(value: '115', child: Text('115网盘')),
            DropdownMenuItem(value: 'quark', child: Text('夸克网盘')),
            DropdownMenuItem(value: '123', child: Text('123云盘')),
          ],
          onChanged: (v) => setState(() => _type = v ?? '115'),
        )),
        _field('Cookie / Token', TextField(
          controller: _tokenCtrl,
          maxLines: 3,
          decoration: InputDecoration(
            isDense: true,
            hintText: widget.initial == null ? '115/夸克填整段 cookie，123 填 accessToken' : '留空=保留原凭据',
            border: const OutlineInputBorder(),
          ),
        )),
        _field('根目录', TextField(controller: _rootCtrl, decoration: const InputDecoration(isDense: true, hintText: '/（网盘根）', border: OutlineInputBorder()))),
        _switchRow('设为默认', _isDefault, (v) => setState(() => _isDefault = v)),
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

  Widget _field(String label, Widget child) => Padding(
        padding: const EdgeInsets.only(bottom: 12),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Text(label, style: TextStyle(fontSize: 12, fontWeight: FontWeight.w600, color: context.dimText)),
          const SizedBox(height: 6),
          child,
        ]),
      );

  Widget _switchRow(String label, bool value, ValueChanged<bool> onChanged) => Padding(
        padding: const EdgeInsets.only(bottom: 4),
        child: Row(children: [Expanded(child: Text(label, style: const TextStyle(fontSize: 13))), Switch(value: value, onChanged: onChanged)]),
      );
}

/// STRM 规则表单（添加/编辑）。
class _RuleForm extends StatefulWidget {
  final List<CloudDrive> drives;
  final StrmRule? initial;
  final Future<bool> Function(Map<String, dynamic>) onSubmit;
  final Future<String?> Function(CloudDrive) onBrowse;
  const _RuleForm({required this.drives, this.initial, required this.onSubmit, required this.onBrowse});
  @override
  State<_RuleForm> createState() => _RuleFormState();
}

class _RuleFormState extends State<_RuleForm> {
  late final _nameCtrl = TextEditingController(text: widget.initial?.name ?? '');
  late final _cloudCtrl = TextEditingController(text: widget.initial?.cloudPath ?? '/');
  late final _outCtrl = TextEditingController(text: widget.initial?.outputDir ?? '');
  late final _mountCtrl = TextEditingController(text: widget.initial?.mountRoot ?? '');
  late final _tplCtrl = TextEditingController(text: widget.initial?.template ?? '');
  late int _driveId = widget.initial?.driveId ?? (widget.drives.isNotEmpty ? widget.drives.first.id : 0);
  late bool _recursive = widget.initial?.recursive ?? true;
  late bool _keepExt = widget.initial?.keepExt ?? true;
  late bool _enabled = widget.initial?.enabled ?? true;
  bool _saving = false;

  @override
  void dispose() {
    _nameCtrl.dispose();
    _cloudCtrl.dispose();
    _outCtrl.dispose();
    _mountCtrl.dispose();
    _tplCtrl.dispose();
    super.dispose();
  }

  void _toast(String msg) => ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));

  Future<void> _browse() async {
    final d = widget.drives.firstWhere((x) => x.id == _driveId, orElse: () => widget.drives.first);
    final p = await widget.onBrowse(d);
    if (p != null && mounted) setState(() => _cloudCtrl.text = p);
  }

  Future<void> _submit() async {
    if (_nameCtrl.text.trim().isEmpty || _driveId == 0 || _outCtrl.text.trim().isEmpty) {
      _toast('名称/网盘/输出目录必填');
      return;
    }
    setState(() => _saving = true);
    final body = <String, dynamic>{
      'name': _nameCtrl.text.trim(),
      'drive_id': _driveId,
      'cloud_path': _cloudCtrl.text.trim(),
      'output_dir': _outCtrl.text.trim(),
      'mount_root': _mountCtrl.text.trim(),
      'template': _tplCtrl.text.trim(),
      'recursive': _recursive,
      'keep_ext': _keepExt,
      'enabled': _enabled,
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
          _field('名称', TextField(controller: _nameCtrl, decoration: const InputDecoration(isDense: true, hintText: '如：电影 STRM', border: OutlineInputBorder()))),
          _field('网盘', DropdownButton<int>(
            value: _driveId,
            isExpanded: true,
            items: [for (final d in widget.drives) DropdownMenuItem(value: d.id, child: Text(d.name))],
            onChanged: (v) => setState(() => _driveId = v ?? 0),
          )),
          _field('云盘目录', Row(children: [
            Expanded(child: TextField(controller: _cloudCtrl, decoration: const InputDecoration(isDense: true, hintText: '/电影', border: OutlineInputBorder()))),
            const SizedBox(width: 8),
            OutlinedButton(onPressed: _browse, child: const Text('浏览')),
          ])),
          _field('输出目录', TextField(controller: _outCtrl, decoration: const InputDecoration(isDense: true, hintText: '/strm/电影', border: OutlineInputBorder()))),
          _field('挂载根', TextField(controller: _mountCtrl, decoration: const InputDecoration(isDense: true, hintText: '/CloudDrive2/115', border: OutlineInputBorder()))),
          _field('路径模板', TextField(controller: _tplCtrl, decoration: const InputDecoration(isDense: true, hintText: '留空=默认 {mount}{cloudpath}', border: OutlineInputBorder()))),
          _switchRow('递归扫描', _recursive, (v) => setState(() => _recursive = v)),
          _switchRow('保留扩展名', _keepExt, (v) => setState(() => _keepExt = v)),
          _switchRow('启用', _enabled, (v) => setState(() => _enabled = v)),
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
          Text(label, style: TextStyle(fontSize: 12, fontWeight: FontWeight.w600, color: context.dimText)),
          const SizedBox(height: 6),
          child,
        ]),
      );

  Widget _switchRow(String label, bool value, ValueChanged<bool> onChanged) => Padding(
        padding: const EdgeInsets.only(bottom: 4),
        child: Row(children: [Expanded(child: Text(label, style: const TextStyle(fontSize: 13))), Switch(value: value, onChanged: onChanged)]),
      );
}
