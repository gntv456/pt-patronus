import 'package:flutter/material.dart';
import 'package:ptpatronus/api/downloader.dart';
import 'package:ptpatronus/api/reseed.dart';
import 'package:ptpatronus/theme/glass.dart';

/// 辅种页（IYUU 精确匹配）：配置 token → 选下载器开始辅种 → 候选任务审阅 → 添加辅种/跳过。
/// 复用后端 /keepseed/* API，对齐 Web Reseed.vue。
class ReseedPage extends StatefulWidget {
  const ReseedPage({super.key});
  @override
  State<ReseedPage> createState() => _ReseedPageState();
}

class _ReseedPageState extends State<ReseedPage> {
  final _api = ReseedApi();
  final _dlApi = DownloaderApi();
  final _tokenCtrl = TextEditingController();

  bool _configured = false;
  String _masked = '';
  bool _savingToken = false;

  List<Downloader> _downloaders = [];
  int? _selectedDl;
  bool _reseeding = false;

  // 当前选中下载器的辅种设置（自动辅种 / 自动添加）
  ReseedSetting? _setting;
  bool _loadingSetting = false;

  List<ReseedTask> _tasks = [];
  bool _loading = true;
  String _status = 'pending'; // '' = 全部
  final _resolving = <int>{};

  @override
  void dispose() {
    _tokenCtrl.dispose();
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
      setState(() => _setting = s);
    } catch (_) {
      // 保留空设置（默认全关）
    } finally {
      if (mounted) setState(() => _loadingSetting = false);
    }
  }

  Future<void> _toggleAuto({bool? autoReseed, bool? autoReseedAdd}) async {
    if (_selectedDl == null || _setting == null) return;
    final next = _setting!.copyWith(autoReseed: autoReseed, autoReseedAdd: autoReseedAdd);
    setState(() => _setting = next);
    try {
      await _api.saveSetting(next);
    } catch (_) {
      if (mounted) _toast('设置保存失败');
      await _loadSetting(); // 回滚
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
    return Scaffold(
      body: GlassBackground(
        child: SafeArea(
          child: Padding(
            padding: const EdgeInsets.all(16),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Row(children: [
                  const Text('辅种', style: TextStyle(fontSize: 24, fontWeight: FontWeight.w700)),
                  const Spacer(),
                  IconButton(
                    icon: const Icon(Icons.refresh),
                    onPressed: _loading ? null : _loadTasks,
                    tooltip: '刷新',
                  ),
                ]),
                const SizedBox(height: 12),
                Expanded(
                  child: ListView(
                    children: [
                      _configCard(),
                      const SizedBox(height: 14),
                      _reseedCard(),
                      const SizedBox(height: 14),
                      _filterChips(),
                      const SizedBox(height: 8),
                      if (_loading)
                        const Padding(
                          padding: EdgeInsets.symmetric(vertical: 24),
                          child: Center(child: CircularProgressIndicator()),
                        )
                      else if (_tasks.isEmpty)
                        const Padding(
                          padding: EdgeInsets.symmetric(vertical: 24),
                          child: Center(
                              child: Text('暂无候选，点「开始辅种」',
                                  style: TextStyle(color: Colors.grey))),
                        )
                      else
                        ..._tasks.map(_taskCard),
                    ],
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  Widget _configCard() {
    return GlassCard(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text('IYUU 配置', style: TextStyle(fontWeight: FontWeight.w600, fontSize: 15)),
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
          const Text('开始辅种', style: TextStyle(fontWeight: FontWeight.w600, fontSize: 15)),
          const SizedBox(height: 8),
          Row(children: [
            Expanded(
              child: DropdownButton<int>(
                value: _selectedDl,
                isExpanded: true,
                hint: const Text('选择下载器'),
                items: _downloaders
                    .map((d) => DropdownMenuItem(value: d.id, child: Text(d.name)))
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
                  ? const SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2))
                  : const Icon(Icons.eco_outlined, size: 18),
              label: const Text('开始辅种'),
            ),
          ]),
          const SizedBox(height: 4),
          _buildAutoRow(),
        ],
      ),
    );
  }

  // 该下载器的定时自动辅种 / 自动添加开关（随选中下载器加载、toggle 即保存）。
  Widget _buildAutoRow() {
    if (_selectedDl == null) return const SizedBox.shrink();
    final autoReseed = _setting?.autoReseed ?? false;
    final autoReseedAdd = _setting?.autoReseedAdd ?? false;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Divider(height: 24),
        Row(children: [
          const Expanded(child: Text('定时自动辅种', style: TextStyle(fontSize: 13))),
          Switch(
            value: autoReseed,
            onChanged: _loadingSetting ? null : (v) => _toggleAuto(autoReseed: v),
          ),
        ]),
        Row(children: [
          const Expanded(
            child: Text('自动添加新候选', style: TextStyle(fontSize: 13, color: Colors.grey)),
          ),
          Switch(
            value: autoReseedAdd,
            onChanged: autoReseed ? ((v) => _toggleAuto(autoReseedAdd: v)) : null,
          ),
        ]),
        const Text('每天 3 点扫描（KEEPSEED_CRON 可改）',
            style: TextStyle(color: Colors.grey, fontSize: 11)),
      ],
    );
  }

  Widget _filterChips() {
    const items = [
      ('pending', '待处理'),
      ('added', '已添加'),
      ('skipped', '已跳过'),
      ('failed', '失败'),
      ('', '全部'),
    ];
    return Wrap(
      spacing: 6,
      children: items
          .map((e) => ChoiceChip(
                label: Text(e.$2),
                selected: _status == e.$1,
                onSelected: (_) {
                  setState(() => _status = e.$1);
                  _loadTasks();
                },
              ))
          .toList(),
    );
  }

  Widget _taskCard(ReseedTask t) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 10),
      child: GlassCard(
        padding: const EdgeInsets.fromLTRB(14, 12, 14, 12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(children: [
              Expanded(
                child: Text(
                  t.sourceName.isEmpty ? t.infoHash : t.sourceName,
                  style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 14),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
              ),
              const SizedBox(width: 8),
              Text(_statusLabel(t.status), style: TextStyle(color: _statusColor(t.status), fontSize: 12)),
            ]),
            const SizedBox(height: 6),
            Text('候选站点：${t.candidateSite}  ·  保存：${t.savePath.isEmpty ? '-' : t.savePath}',
                style: const TextStyle(color: Colors.grey, fontSize: 12),
                maxLines: 1, overflow: TextOverflow.ellipsis),
            if (t.status == 'pending') ...[
              const SizedBox(height: 8),
              Row(children: [
                FilledButton.tonal(
                  onPressed: _resolving.contains(t.id) ? null : () => _resolve(t, 'add'),
                  child: const Text('添加辅种', style: TextStyle(fontSize: 13)),
                ),
                const SizedBox(width: 8),
                TextButton(
                  onPressed: _resolving.contains(t.id) ? null : () => _resolve(t, 'skip'),
                  child: const Text('跳过', style: TextStyle(fontSize: 13)),
                ),
              ]),
            ],
          ],
        ),
      ),
    );
  }
}
