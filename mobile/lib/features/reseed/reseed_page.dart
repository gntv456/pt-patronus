import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:ptpatronus/api/downloader.dart';
import 'package:ptpatronus/api/reseed.dart';
import 'package:ptpatronus/core/responsive.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_filter_sheet.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';
import 'package:url_launcher/url_launcher.dart';

/// 辅种页（IYUU 精确匹配，对齐 Web Reseed.vue）：配置 token → 选下载器开始辅种 →
/// 候选任务审阅（宽屏表格 / 窄屏卡片）→ 添加辅种/跳过。复用后端 /keepseed/*。
class ReseedPage extends StatefulWidget {
  const ReseedPage({super.key});
  @override
  State<ReseedPage> createState() => _ReseedPageState();
}

class _ReseedPageState extends State<ReseedPage> {
  final _api = ReseedApi();
  final _dlApi = DownloaderApi();
  final _tokenCtrl = TextEditingController();
  final _savePathCtrl = TextEditingController();

  bool _configured = false;
  String _masked = '';
  bool _savingToken = false;

  List<Downloader> _downloaders = [];
  int? _selectedDl;
  bool _reseeding = false;

  ReseedSetting? _setting;
  bool _loadingSetting = false;

  List<ReseedTask> _tasks = [];
  bool _loading = true;
  String _status = 'pending';
  final _resolving = <int>{};

  @override
  void dispose() {
    _tokenCtrl.dispose();
    _savePathCtrl.dispose();
    super.dispose();
  }

  @override
  void initState() {
    super.initState();
    _loadAll();
  }

  Future<void> _loadAll() async {
    await Future.wait([_loadConfig(), _loadDownloaders(), _loadTasks()]);
  }

  Future<void> _loadConfig() async {
    try {
      final c = await _api.getConfig();
      if (!mounted) return;
      setState(() {
        _configured = c['configured'] == true;
        _masked = (c['iyuu_token_masked'] as String?) ?? '';
      });
    } catch (_) {}
  }

  Future<void> _saveToken() async {
    final t = _tokenCtrl.text.trim();
    if (t.isEmpty) {
      _toast('请输入 IYUU token');
      return;
    }
    setState(() => _savingToken = true);
    try {
      await _api.setToken(t);
      _tokenCtrl.clear();
      _toast('IYUU token 已保存');
      await _loadConfig();
    } catch (_) {
      _toast('保存失败');
    } finally {
      if (mounted) setState(() => _savingToken = false);
    }
  }

  Future<void> _loadDownloaders() async {
    try {
      final ls = await _dlApi.list();
      if (!mounted) return;
      setState(() {
        _downloaders = ls;
        _selectedDl ??= ls.isNotEmpty ? ls.first.id : null;
      });
      if (_selectedDl != null) _loadSetting();
    } catch (_) {}
  }

  Future<void> _loadSetting() async {
    if (_selectedDl == null) return;
    setState(() => _loadingSetting = true);
    try {
      final s = await _api.setting(_selectedDl!);
      if (!mounted) return;
      setState(() {
        _setting = s;
        _savePathCtrl.text = s.savePath;
      });
    } catch (_) {
      if (mounted) {
        setState(() {
          _setting = ReseedSetting(downloaderId: _selectedDl!);
          _savePathCtrl.text = '';
        });
      }
    } finally {
      if (mounted) setState(() => _loadingSetting = false);
    }
  }

  Future<void> _toggleAuto({bool? autoReseed, bool? autoReseedAdd}) async {
    if (_selectedDl == null || _setting == null) return;
    final next = _setting!.copyWith(
      autoReseed: autoReseed,
      autoReseedAdd: autoReseedAdd,
    );
    setState(() => _setting = next);
    try {
      await _api.saveSetting(next);
    } catch (_) {
      if (mounted) _toast('设置保存失败');
      await _loadSetting();
    }
  }

  // 保存保存路径 + 暂停添加（这两个字段不在开关即时保存，单独按钮提交）。
  Future<void> _savePathAndPause() async {
    if (_selectedDl == null || _setting == null) return;
    final next = _setting!.copyWith(
      savePath: _savePathCtrl.text.trim(),
      pauseAfterAdd: _setting!.pauseAfterAdd,
    );
    setState(() => _setting = next);
    try {
      await _api.saveSetting(next);
      _toast('已保存');
    } catch (_) {
      _toast('保存失败');
    }
  }

  Future<void> _togglePause(bool v) async {
    if (_selectedDl == null || _setting == null) return;
    final next = _setting!.copyWith(
      pauseAfterAdd: v,
      savePath: _savePathCtrl.text.trim(),
    );
    setState(() => _setting = next);
    try {
      await _api.saveSetting(next);
    } catch (_) {
      if (mounted) _toast('保存失败');
      await _loadSetting();
    }
  }

  Future<void> _loadTasks() async {
    setState(() => _loading = true);
    try {
      final ts = await _api.tasks(_status);
      if (!mounted) return;
      setState(() => _tasks = ts);
    } catch (_) {
      if (mounted) _toast('任务加载失败');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _startReseed() async {
    if (_selectedDl == null) {
      _toast('请选择下载器');
      return;
    }
    if (!_configured) {
      _toast('请先保存 IYUU token');
      return;
    }
    setState(() => _reseeding = true);
    try {
      final created = await _api.reseed(_selectedDl!);
      _toast('辅种完成，新增 ${created.length} 个候选');
      if (!mounted) return;
      setState(() => _status = 'pending');
      await _loadTasks();
    } catch (e) {
      _toast('辅种失败：$e');
    } finally {
      if (mounted) setState(() => _reseeding = false);
    }
  }

  Future<void> _resolve(ReseedTask t, String action) async {
    // 添加辅种推送下载器（不可逆）→ 二次确认；跳过可逆不确认。
    if (action == 'add') {
      final ok = await showDialog<bool>(
        context: context,
        builder: (_) => AlertDialog(
          title: const Text('添加辅种'),
          content: const Text('将向下载器添加该辅种任务，确认？'),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(context, false),
              child: const Text('取消'),
            ),
            FilledButton(
              onPressed: () => Navigator.pop(context, true),
              child: const Text('添加'),
            ),
          ],
        ),
      );
      if (ok != true) return;
    }
    setState(() => _resolving.add(t.id));
    try {
      await _api.resolve(t.id, action);
      _toast(action == 'add' ? '已添加辅种' : '已跳过');
      await _loadTasks();
    } catch (e) {
      _toast('操作失败：$e');
    } finally {
      if (mounted) setState(() => _resolving.remove(t.id));
    }
  }

  Future<void> _copyLink(String url) async {
    if (url.isEmpty) return;
    await Clipboard.setData(ClipboardData(text: url));
    _toast('已复制下载链接');
  }

  void _openIyuu() async {
    const url = 'https://iyuu.cn';
    if (!await launchUrl(
      Uri.parse(url),
      mode: LaunchMode.externalApplication,
    )) {
      _toast(url);
    }
  }

  void _toast(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
  }

  Color _statusColor(String s) => switch (s) {
    'added' => PTColors.success,
    'failed' => PTColors.danger,
    'pending' => PTColors.warn,
    _ => Colors.grey,
  };

  String _statusLabel(String s) => switch (s) {
    'added' => '已添加',
    'failed' => '失败',
    'pending' => '待处理',
    'skipped' => '已跳过',
    _ => s,
  };

  @override
  Widget build(BuildContext context) {
    return PTPageScaffold(
      title: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          const Text(
            '辅种',
            style: TextStyle(fontSize: 17, fontWeight: FontWeight.w700),
          ),
          const SizedBox(width: 8),
          _iyuuBadge(),
        ],
      ),
      actions: [
        PTBarAction(
          icon: Icons.refresh,
          label: '刷新',
          loading: _loading,
          onPressed: _loading ? null : _loadTasks,
        ),
        PTBarAction(
          icon: Icons.open_in_new,
          label: 'iyuu.cn',
          onPressed: _openIyuu,
          overflow: true,
        ),
      ],
      filterDimensions: _filterDimensions(),
      filterOnClearAll: () {
        setState(() => _status = '');
        _loadTasks();
      },
      body: Padding(
        padding: const EdgeInsets.fromLTRB(14, 4, 14, 2),
        child: isWideScreen(context) ? _wideBody() : _narrowBody(),
      ),
    );
  }

  // IYUU 配置状态徽标（原 header 内联，现并入标题区）。
  Widget _iyuuBadge() {
    final on = _configured;
    final c = on ? PTColors.success : PTColors.warn;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
      decoration: BoxDecoration(
        color: c.withValues(alpha: 0.16),
        borderRadius: BorderRadius.circular(999),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            width: 6,
            height: 6,
            decoration: BoxDecoration(color: c, shape: BoxShape.circle),
          ),
          const SizedBox(width: 5),
          Text(
            on ? 'IYUU 已配置' : 'IYUU 未配置',
            style: TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w600,
              color: c,
            ),
          ),
        ],
      ),
    );
  }

  // 窄屏：纵向卡片列表（配置 + 辅种 + 任务卡片）。
  Widget _narrowBody() {
    return ListView(
      children: [
        _configCard(),
        const SizedBox(height: 14),
        _reseedCard(),
        const SizedBox(height: 14),
        _tasksArea(narrow: true),
      ],
    );
  }

  // 宽屏：配置/辅种两栏并排 + 任务表格。
  Widget _wideBody() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Expanded(child: _configCard()),
            const SizedBox(width: 14),
            Expanded(child: _reseedCard()),
          ],
        ),
        const SizedBox(height: 14),
        Expanded(child: _tasksArea(narrow: false)),
      ],
    );
  }

  // 任务区：宽屏表格 / 窄屏卡片，由 PTAdaptiveTable 统一收敛。
  Widget _tasksArea({required bool narrow}) {
    if (_loading) {
      return const Center(child: CircularProgressIndicator());
    }
    if (_tasks.isEmpty) {
      return PTEmptyState(
        icon: Icons.eco_outlined,
        text: '暂无候选',
        description: '点「开始辅种」扫描跨站可辅助种子',
      );
    }
    return PTAdaptiveTable<ReseedTask>(
      items: _tasks,
      columns: [
        PTColumn<ReseedTask>(
          label: '源种子',
          flex: 3,
          cell: (_, t) => Text(
            t.sourceName.isEmpty ? t.infoHash : t.sourceName,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(fontSize: 13),
          ),
        ),
        PTColumn<ReseedTask>(
          label: '候选站点',
          flex: 2,
          cell: (_, t) =>
              Text(t.candidateSite, style: const TextStyle(fontSize: 13)),
        ),
        PTColumn<ReseedTask>(
          label: '下载链接',
          flex: 2,
          cell: (_, t) => t.candidateUrl.isEmpty
              ? const Text(
                  '—',
                  style: TextStyle(color: Colors.grey, fontSize: 12),
                )
              : InkWell(
                  onTap: () => _copyLink(t.candidateUrl),
                  child: const Text(
                    '复制链接',
                    style: TextStyle(
                      fontSize: 12,
                      color: PTColors.primary2,
                      decoration: TextDecoration.underline,
                    ),
                  ),
                ),
        ),
        PTColumn<ReseedTask>(
          label: '保存路径',
          flex: 2,
          cell: (_, t) => Text(
            t.savePath.isEmpty ? '-' : t.savePath,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(fontSize: 12, color: Colors.grey),
          ),
        ),
        PTColumn<ReseedTask>(
          label: '状态',
          flex: 1,
          cell: (_, t) => Container(
            padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 2),
            decoration: BoxDecoration(
              color: _statusColor(t.status).withValues(alpha: 0.16),
              borderRadius: BorderRadius.circular(999),
            ),
            child: Text(
              _statusLabel(t.status),
              style: TextStyle(
                color: _statusColor(t.status),
                fontSize: 11,
                fontWeight: FontWeight.w600,
              ),
            ),
          ),
        ),
        PTColumn<ReseedTask>(
          label: '操作',
          flex: 2,
          cell: (_, t) => t.status == 'pending'
              ? Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    FilledButton.tonal(
                      onPressed: _resolving.contains(t.id)
                          ? null
                          : () => _resolve(t, 'add'),
                      child: const Text('添加', style: TextStyle(fontSize: 12)),
                    ),
                    const SizedBox(width: 6),
                    TextButton(
                      onPressed: _resolving.contains(t.id)
                          ? null
                          : () => _resolve(t, 'skip'),
                      child: const Text('跳过', style: TextStyle(fontSize: 12)),
                    ),
                  ],
                )
              : const Text(
                  '—',
                  style: TextStyle(color: Colors.grey, fontSize: 12),
                ),
        ),
      ],
      narrowCard: (_, t) => _taskCard(t),
      emptyText: '暂无候选',
      embedded: true, // 窄屏嵌在 _narrowBody ListView：shrinkWrap 避免塌缩（宽屏分支不受影响）
    );
  }

  Widget _configCard() {
    return GlassCard(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text(
            'IYUU 配置',
            style: TextStyle(fontWeight: FontWeight.w600, fontSize: 15),
          ),
          const SizedBox(height: 8),
          TextField(
            controller: _tokenCtrl,
            obscureText: true,
            decoration: InputDecoration(
              isDense: true,
              hintText: _configured ? '已配置：$_masked（输入新值覆盖）' : '粘贴 IYUU token',
              border: const OutlineInputBorder(),
            ),
          ),
          const SizedBox(height: 8),
          FilledButton.icon(
            onPressed: _savingToken ? null : _saveToken,
            icon: const Icon(Icons.save_outlined, size: 18),
            label: const Text('保存 token'),
          ),
        ],
      ),
    );
  }

  Widget _reseedCard() {
    return GlassCard(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text(
            '开始辅种',
            style: TextStyle(fontWeight: FontWeight.w600, fontSize: 15),
          ),
          const SizedBox(height: 8),
          Row(
            children: [
              Expanded(
                child: DropdownButton<int>(
                  value: _selectedDl,
                  isExpanded: true,
                  hint: const Text('选择下载器'),
                  items: _downloaders
                      .map(
                        (d) =>
                            DropdownMenuItem(value: d.id, child: Text(d.name)),
                      )
                      .toList(),
                  onChanged: (v) {
                    setState(() => _selectedDl = v);
                    if (v != null) _loadSetting();
                  },
                ),
              ),
              const SizedBox(width: 10),
              FilledButton.icon(
                onPressed: _reseeding ? null : _startReseed,
                icon: _reseeding
                    ? const SizedBox(
                        width: 16,
                        height: 16,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : const Icon(Icons.eco_outlined, size: 18),
                label: const Text('开始辅种'),
              ),
            ],
          ),
          const SizedBox(height: 4),
          _buildAutoRow(),
        ],
      ),
    );
  }

  // 该下载器的辅种设置：自动辅种 / 自动添加 / 保存路径 / 添加后暂停。
  Widget _buildAutoRow() {
    if (_selectedDl == null) return const SizedBox.shrink();
    final autoReseed = _setting?.autoReseed ?? false;
    final autoReseedAdd = _setting?.autoReseedAdd ?? false;
    final pauseAfterAdd = _setting?.pauseAfterAdd ?? true;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Divider(height: 24),
        Row(
          children: [
            const Expanded(
              child: Text('定时自动辅种', style: TextStyle(fontSize: 13)),
            ),
            Switch(
              value: autoReseed,
              onChanged: _loadingSetting
                  ? null
                  : (v) => _toggleAuto(autoReseed: v),
            ),
          ],
        ),
        Row(
          children: [
            const Expanded(
              child: Text(
                '自动添加新候选',
                style: TextStyle(fontSize: 13, color: Colors.grey),
              ),
            ),
            Switch(
              value: autoReseedAdd,
              onChanged: autoReseed
                  ? ((v) => _toggleAuto(autoReseedAdd: v))
                  : null,
            ),
          ],
        ),
        Row(
          children: [
            const Expanded(
              child: Text(
                '添加后暂停',
                style: TextStyle(fontSize: 13, color: Colors.grey),
              ),
            ),
            Switch(
              value: pauseAfterAdd,
              onChanged: _loadingSetting ? null : _togglePause,
            ),
          ],
        ),
        const SizedBox(height: 6),
        Row(
          children: [
            Expanded(
              child: TextField(
                controller: _savePathCtrl,
                decoration: const InputDecoration(
                  hintText: '保存路径（留空=下载器默认）',
                  border: OutlineInputBorder(),
                  isDense: true,
                ),
              ),
            ),
            const SizedBox(width: 8),
            FilledButton.tonal(
              onPressed: _savePathAndPause,
              child: const Text('保存路径'),
            ),
          ],
        ),
        const Text(
          '每天 3 点扫描（KEEPSEED_CRON 可改）',
          style: TextStyle(color: Colors.grey, fontSize: 11),
        ),
      ],
    );
  }

  // 状态筛选维度（收进顶栏筛选按钮）：选状态 → 重新拉该状态任务。
  List<PTFilterDimension> _filterDimensions() {
    return [
      PTFilterDimension.choice(
        label: '状态',
        selected: _status,
        isDefault: _status.isEmpty,
        onChanged: (v) {
          setState(() => _status = (v as String?) ?? '');
          _loadTasks();
        },
        options: const [
          PTFilterOption<Object?>(value: '', label: '全部'),
          PTFilterOption<Object?>(value: 'pending', label: '待处理'),
          PTFilterOption<Object?>(value: 'added', label: '已添加'),
          PTFilterOption<Object?>(value: 'skipped', label: '已跳过'),
          PTFilterOption<Object?>(value: 'failed', label: '失败'),
        ],
      ),
    ];
  }

  Widget _taskCard(ReseedTask t) {
    return GlassCard(
      padding: const EdgeInsets.fromLTRB(14, 12, 14, 12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Expanded(
                child: Text(
                  t.sourceName.isEmpty ? t.infoHash : t.sourceName,
                  style: const TextStyle(
                    fontWeight: FontWeight.w600,
                    fontSize: 14,
                  ),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
              ),
              const SizedBox(width: 8),
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 2),
                decoration: BoxDecoration(
                  color: _statusColor(t.status).withValues(alpha: 0.16),
                  borderRadius: BorderRadius.circular(999),
                ),
                child: Text(
                  _statusLabel(t.status),
                  style: TextStyle(
                    color: _statusColor(t.status),
                    fontSize: 11,
                    fontWeight: FontWeight.w600,
                  ),
                ),
              ),
            ],
          ),
          const SizedBox(height: 6),
          Text(
            '候选站点：${t.candidateSite}  ·  保存：${t.savePath.isEmpty ? '-' : t.savePath}',
            style: const TextStyle(color: Colors.grey, fontSize: 12),
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
          ),
          if (t.candidateUrl.isNotEmpty)
            Padding(
              padding: const EdgeInsets.only(top: 4),
              child: GestureDetector(
                onTap: () => _copyLink(t.candidateUrl),
                child: const Text(
                  '复制下载链接',
                  style: TextStyle(
                    fontSize: 12,
                    color: PTColors.primary2,
                    decoration: TextDecoration.underline,
                  ),
                ),
              ),
            ),
          if (t.status == 'pending') ...[
            const SizedBox(height: 8),
            Row(
              children: [
                FilledButton.tonal(
                  onPressed: _resolving.contains(t.id)
                      ? null
                      : () => _resolve(t, 'add'),
                  child: const Text('添加辅种', style: TextStyle(fontSize: 13)),
                ),
                const SizedBox(width: 8),
                TextButton(
                  onPressed: _resolving.contains(t.id)
                      ? null
                      : () => _resolve(t, 'skip'),
                  child: const Text('跳过', style: TextStyle(fontSize: 13)),
                ),
              ],
            ),
          ],
        ],
      ),
    );
  }
}
