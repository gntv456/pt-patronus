import 'package:flutter/material.dart';
import 'package:ptpatronus/api/contract.dart';
import 'package:ptpatronus/api/site.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/glass_icon.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';

/// 保种契约页（对齐 Web ContractView）：每站一条契约（分享率/做种/上传下限），
/// 卡片列表 + 新建/编辑表单 + 手动检查 + 删除。复用后端 /contract/rules。
class ContractPage extends StatefulWidget {
  const ContractPage({super.key});
  @override
  State<ContractPage> createState() => _ContractPageState();
}

class _ContractPageState extends State<ContractPage> {
  final _api = ContractApi();
  final _siteApi = SiteApi();

  List<ContractRule> _list = [];
  bool _loading = true;
  String? _error;

  // 站点名查表缓存（拉取一次，卡片 meta + 表单下拉共用）。
  List<Site> _sites = [];

  // 检查进行中的规则 id，禁用对应按钮。
  final _busy = <int>{};

  @override
  void initState() {
    super.initState();
    _load();
    _ensureSites();
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

  Future<void> _ensureSites() async {
    if (_sites.isNotEmpty) return;
    try {
      final sites = await _siteApi.list();
      if (!mounted) return;
      setState(() => _sites = sites);
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

  String _fmtDate(DateTime? d) {
    if (d == null) return '-';
    String two(int v) => v.toString().padLeft(2, '0');
    return '${d.year}-${two(d.month)}-${two(d.day)} ${two(d.hour)}:${two(d.minute)}';
  }

  void _toast(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
  }

  // 切换启用：乐观更新 + 失败回滚。upsert 按 site_id 全量写回。
  Future<void> _toggleEnabled(ContractRule r, bool v) async {
    setState(() {
      _list = _list
          .map((e) => e.id == r.id ? _copyWithEnabled(e, v) : e)
          .toList();
    });
    try {
      await _api.upsert(_ruleBody(r, enabled: v));
    } catch (e) {
      _toast('更新失败：$e');
      await _load();
    }
  }

  ContractRule _copyWithEnabled(ContractRule r, bool v) => ContractRule(
        id: r.id,
        siteId: r.siteId,
        enabled: v,
        minRatio: r.minRatio,
        minSeeding: r.minSeeding,
        minUploadGb: r.minUploadGb,
        officialOnly: r.officialOnly,
        minSeedingSizeGb: r.minSeedingSizeGb,
        period: r.period,
        startTime: r.startTime,
        lastCheckedAt: r.lastCheckedAt,
        lastStatus: r.lastStatus,
        lastDetail: r.lastDetail,
        createdAt: r.createdAt,
        updatedAt: r.updatedAt,
      );

  /// 由现有规则组装 upsert body（[enabled] 可覆盖，供乐观切换）。
  Map<String, dynamic> _ruleBody(ContractRule r, {bool? enabled}) => {
        if (r.id != 0) 'id': r.id,
        'site_id': r.siteId,
        'enabled': enabled ?? r.enabled,
        'min_ratio': r.minRatio,
        'min_seeding': r.minSeeding,
        'min_upload_gb': r.minUploadGb,
        'official_only': r.officialOnly,
        'min_seeding_size_gb': r.minSeedingSizeGb,
        'period': r.period,
        if (r.startTime != null)
          'start_time': r.startTime!.toUtc().toIso8601String(),
      };

  Future<void> _check(ContractRule r) async {
    setState(() => _busy.add(r.id));
    try {
      final res = await _api.check(r.id);
      if (!mounted) return;
      _toast('${_statusLabel(res.status)}：${_siteName(res.siteId)}');
      await _load();
      if (!mounted) return;
      await _showCheckResult(res);
    } catch (e) {
      _toast('检查失败：$e');
    } finally {
      if (mounted) setState(() => _busy.remove(r.id));
    }
  }

  Future<void> _showCheckResult(ContractCheckResult res) {
    return showAdaptiveSheet(
      context,
      title: '检查结果 · ${res.siteName}',
      builder: (_) => Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        mainAxisSize: MainAxisSize.min,
        children: [
          Align(
            alignment: Alignment.centerLeft,
            child: _statusBadge(res.status),
          ),
          const SizedBox(height: 12),
          Row(
            children: [
              _statCell('分享率', res.ratio.toStringAsFixed(2)),
              _statCell('做种数', '${res.seeding}'),
              _statCell('上传', '${res.uploadGb.toStringAsFixed(1)} GiB'),
            ],
          ),
          const SizedBox(height: 8),
          Row(
            children: [
              if (res.seedingOfficial > 0)
                _statCell('官种', '${res.seedingOfficial}')
              else
                const Spacer(),
              _statCell('体积', '${res.seedingSizeGb.toStringAsFixed(1)} GiB'),
              if (res.seedingOfficial == 0) const Spacer(),
            ],
          ),
          if (res.unsupported.isNotEmpty) ...[
            const SizedBox(height: 8),
            Text(
              '⚠ ${res.unsupported}',
              style: const TextStyle(fontSize: 12, color: PTColors.warn),
            ),
          ],
          if (res.detail.isNotEmpty) ...[
            const SizedBox(height: 10),
            Text(res.detail, style: const TextStyle(fontSize: 13)),
          ],
          if (res.breaches.isNotEmpty) ...[
            const SizedBox(height: 10),
            const Text(
              '未达标项',
              style: TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.w600,
                color: Colors.grey,
              ),
            ),
            const SizedBox(height: 4),
            for (final b in res.breaches)
              Padding(
                padding: const EdgeInsets.only(top: 2, left: 4),
                child: Row(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    const Text(
                      '· ',
                      style: TextStyle(color: PTColors.warn, fontSize: 13),
                    ),
                    Expanded(
                      child: Text(b, style: const TextStyle(fontSize: 13)),
                    ),
                  ],
                ),
              ),
          ],
        ],
      ),
    );
  }

  Future<void> _delete(ContractRule r) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('删除保种契约'),
        content: Text('确认删除「${_siteName(r.siteId)}」的保种契约？此操作不可撤销。'),
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
      await _api.remove(r.id);
      _toast('已删除');
      await _load();
    } catch (e) {
      _toast('删除失败：$e');
    }
  }

  Future<void> _openCreate() async {
    await _ensureSites();
    if (!mounted) return;
    await showAdaptiveSheet(
      context,
      title: '新建保种契约',
      builder: (_) => _ContractForm(
        sites: _sites,
        onSubmit: (body) async {
          try {
            await _api.upsert(body);
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

  Future<void> _openEdit(ContractRule r) async {
    await _ensureSites();
    if (!mounted) return;
    await showAdaptiveSheet(
      context,
      title: '编辑保种契约',
      builder: (_) => _ContractForm(
        sites: _sites,
        initial: r,
        onSubmit: (body) async {
          try {
            await _api.upsert(body);
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

  Color _statusColor(String s) {
    switch (s) {
      case 'ok':
        return PTColors.success;
      case 'breach':
        return PTColors.warn;
      case 'error':
        return PTColors.danger;
      default:
        return Colors.grey;
    }
  }

  String _statusLabel(String s) {
    switch (s) {
      case 'ok':
        return '达标';
      case 'breach':
        return '未达标';
      case 'error':
        return '出错';
      default:
        return '未检查';
    }
  }

  Widget _statusBadge(String s) {
    final c = _statusColor(s);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
      decoration: BoxDecoration(
        color: c.withValues(alpha: 0.15),
        borderRadius: BorderRadius.circular(999),
        border: Border.all(color: c.withValues(alpha: 0.4)),
      ),
      child: Text(
        _statusLabel(s),
        style: TextStyle(fontSize: 12, fontWeight: FontWeight.w600, color: c),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return PTPageScaffold(
      title: '保种契约',
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
          if (_loading && _list.isNotEmpty)
            const LinearProgressIndicator(minHeight: 2),
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
        icon: Icons.assignment_outlined,
        text: '暂无保种契约',
        description: '点右上「新建」为站点设置分享率/做种/上传下限',
      );
    }
    return PTMasonry<ContractRule>(
      items: _list,
      minItemWidth: 300,
      padding: const EdgeInsets.fromLTRB(12, 8, 12, 16),
      itemBuilder: (_, r) => _ruleCard(r),
    );
  }

  Widget _ruleCard(ContractRule r) {
    final busy = _busy.contains(r.id);
    return GlassCard(
      padding: const EdgeInsets.all(14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // 头部：图标 + 站点名 + 下限 meta + 启用开关。
          Row(
            children: [
              const GlassIcon(
                glyph: 'contract',
                from: PTColors.primary,
                to: PTColors.primary2,
                size: 32,
              ),
              const SizedBox(width: 10),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      _siteName(r.siteId),
                      style: const TextStyle(
                        fontWeight: FontWeight.w700,
                        fontSize: 15,
                      ),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                    ),
                    const SizedBox(height: 2),
                    Text(
                      '分享率≥${r.minRatio} · 做种≥${r.minSeeding} · 上传≥${r.minUploadGb}GiB'
                      '${r.officialOnly ? " · 仅官种" : ""}'
                      '${r.minSeedingSizeGb > 0 ? " · 体积≥${r.minSeedingSizeGb}GiB" : ""}'
                      '${r.period == "weekly" ? " · 每周" : r.period == "monthly" ? " · 每月" : ""}',
                      style: const TextStyle(color: Colors.grey, fontSize: 12),
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                    ),
                  ],
                ),
              ),
              Switch(
                value: r.enabled,
                onChanged: (v) => _toggleEnabled(r, v),
                materialTapTargetSize: MaterialTapTargetSize.padded,
              ),
            ],
          ),
          const SizedBox(height: 10),
          // 状态徽章 + 上次检查时间。
          Row(
            children: [
              _statusBadge(r.lastStatus),
              const SizedBox(width: 8),
              Flexible(
                child: Text(
                  '检查于 ${_fmtDate(r.lastCheckedAt)}',
                  style: const TextStyle(color: Colors.grey, fontSize: 12),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
              ),
            ],
          ),
          if (r.lastDetail.isNotEmpty) ...[
            const SizedBox(height: 8),
            Text(
              r.lastDetail,
              style: const TextStyle(fontSize: 12),
              maxLines: 3,
              overflow: TextOverflow.ellipsis,
            ),
          ],
          const SizedBox(height: 10),
          // 操作行：检查 / 编辑 / 删除。
          Wrap(
            spacing: 6,
            runSpacing: 4,
            children: [
              FilledButton.tonal(
                onPressed: busy ? null : () => _check(r),
                child: const Text('检查', style: TextStyle(fontSize: 13)),
              ),
              TextButton(
                onPressed: () => _openEdit(r),
                child: const Text('编辑', style: TextStyle(fontSize: 13)),
              ),
              TextButton(
                onPressed: () => _delete(r),
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

/// 新建 / 编辑保种契约的表单（放进 showAdaptiveSheet）。
/// 自持下拉 / 开关 / 文本框状态；[onSubmit] 返回 true 时表单自关闭。
class _ContractForm extends StatefulWidget {
  final List<Site> sites;
  final ContractRule? initial;
  final Future<bool> Function(Map<String, dynamic> body) onSubmit;

  const _ContractForm({
    required this.sites,
    required this.onSubmit,
    this.initial,
  });

  @override
  State<_ContractForm> createState() => _ContractFormState();
}

class _ContractFormState extends State<_ContractForm> {
  late final _ratioCtrl = TextEditingController(
    text: widget.initial == null ? '' : '${widget.initial!.minRatio}',
  );
  late final _seedingCtrl = TextEditingController(
    text: widget.initial == null ? '' : '${widget.initial!.minSeeding}',
  );
  late final _uploadCtrl = TextEditingController(
    text: widget.initial == null ? '' : '${widget.initial!.minUploadGb}',
  );
  late final _sizeCtrl = TextEditingController(
    text: widget.initial == null ? '' : '${widget.initial!.minSeedingSizeGb}',
  );

  int? _siteId;
  bool _enabled = true;
  bool _officialOnly = false;
  String _period = '';
  DateTime? _startTime;
  bool _saving = false;

  @override
  void initState() {
    super.initState();
    final ini = widget.initial;
    if (ini != null) {
      _siteId =
          widget.sites.any((s) => s.id == ini.siteId) ? ini.siteId : null;
      _enabled = ini.enabled;
      _officialOnly = ini.officialOnly;
      _period = ini.period;
      _startTime = ini.startTime;
    } else {
      _siteId = widget.sites.isNotEmpty ? widget.sites.first.id : null;
    }
  }

  @override
  void dispose() {
    _ratioCtrl.dispose();
    _seedingCtrl.dispose();
    _uploadCtrl.dispose();
    _sizeCtrl.dispose();
    super.dispose();
  }

  double _parseDouble(String s, [double def = 0]) =>
      double.tryParse(s.trim()) ?? def;
  int _parseInt(String s, [int def = 0]) => int.tryParse(s.trim()) ?? def;

  Future<void> _pickStartTime() async {
    final now = DateTime.now();
    final d = await showDatePicker(
      context: context,
      initialDate: _startTime ?? now,
      firstDate: DateTime(now.year - 1),
      lastDate: DateTime(now.year + 5),
    );
    if (d == null || !mounted) return;
    final t = await showTimePicker(
      context: context,
      initialTime: TimeOfDay.fromDateTime(_startTime ?? now),
    );
    if (t == null || !mounted) return;
    setState(() {
      _startTime = DateTime(d.year, d.month, d.day, t.hour, t.minute);
    });
  }

  String _fmtStartTime(DateTime d) {
    String two(int v) => v.toString().padLeft(2, '0');
    return '${d.year}-${two(d.month)}-${two(d.day)} ${two(d.hour)}:${two(d.minute)}';
  }

  void _toast(String msg) {
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
  }

  Future<void> _submit() async {
    if (_siteId == null) {
      _toast('请选择站点');
      return;
    }
    setState(() => _saving = true);
    final body = <String, dynamic>{
      if (widget.initial != null) 'id': widget.initial!.id,
      'site_id': _siteId,
      'enabled': _enabled,
      'min_ratio': _parseDouble(_ratioCtrl.text),
      'min_seeding': _parseInt(_seedingCtrl.text),
      'min_upload_gb': _parseInt(_uploadCtrl.text),
      'official_only': _officialOnly,
      'min_seeding_size_gb': _parseInt(_sizeCtrl.text),
      'period': _period,
      if (_startTime != null)
        'start_time': _startTime!.toUtc().toIso8601String(),
    };
    final ok = await widget.onSubmit(body);
    if (!mounted) return;
    setState(() => _saving = false);
    if (ok) Navigator.of(context).pop();
  }

  @override
  Widget build(BuildContext context) {
    final editing = widget.initial != null;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      mainAxisSize: MainAxisSize.min,
      children: [
        _field(
          '站点',
          DropdownButton<int>(
            value: _siteId,
            isExpanded: true,
            hint: const Text('选择站点'),
            items: widget.sites
                .map((s) => DropdownMenuItem(value: s.id, child: Text(s.name)))
                .toList(),
            onChanged:
                editing ? null : (v) => setState(() => _siteId = v),
          ),
        ),
        _switchRow('启用', _enabled, (v) => setState(() => _enabled = v)),
        const Divider(height: 20),
        const Text(
          '保种下限',
          style: TextStyle(fontWeight: FontWeight.w600, fontSize: 13),
        ),
        const SizedBox(height: 8),
        _field(
          '最低分享率',
          TextField(
            controller: _ratioCtrl,
            keyboardType:
                const TextInputType.numberWithOptions(decimal: true),
            decoration: const InputDecoration(
              isDense: true,
              hintText: '如 1.0',
              border: OutlineInputBorder(),
            ),
          ),
        ),
        _field(
          '最少做种数',
          TextField(
            controller: _seedingCtrl,
            keyboardType: TextInputType.number,
            decoration: const InputDecoration(
              isDense: true,
              hintText: '如 5',
              border: OutlineInputBorder(),
            ),
          ),
        ),
        _field(
          '最低上传（GiB）',
          TextField(
            controller: _uploadCtrl,
            keyboardType: TextInputType.number,
            decoration: const InputDecoration(
              isDense: true,
              hintText: '如 100',
              border: OutlineInputBorder(),
            ),
          ),
        ),
        const Divider(height: 24),
        const Align(
          alignment: Alignment.centerLeft,
          child: Text(
            '扩展维度（官种 / 体积 / 周期 / 生效时间）',
            style: TextStyle(fontWeight: FontWeight.w600, fontSize: 13),
          ),
        ),
        const SizedBox(height: 8),
        _switchRow(
          '仅官种（做种数/体积只计官种）',
          _officialOnly,
          (v) => setState(() => _officialOnly = v),
        ),
        _field(
          '做种体积下限（GiB）',
          TextField(
            controller: _sizeCtrl,
            keyboardType: TextInputType.number,
            decoration: const InputDecoration(
              isDense: true,
              hintText: '如 500',
              border: OutlineInputBorder(),
            ),
          ),
        ),
        _field(
          '考核周期',
          DropdownButton<String>(
            value: _period,
            isExpanded: true,
            items: const [
              DropdownMenuItem(value: '', child: Text('每日')),
              DropdownMenuItem(value: 'weekly', child: Text('每周')),
              DropdownMenuItem(value: 'monthly', child: Text('每月')),
            ],
            onChanged: (v) => setState(() => _period = v ?? ''),
          ),
        ),
        _field(
          '生效时间',
          InkWell(
            onTap: _pickStartTime,
            child: InputDecorator(
              decoration: const InputDecoration(
                isDense: true,
                border: OutlineInputBorder(),
              ),
              child: Text(
                _startTime == null
                    ? '留空 = 立即生效'
                    : _fmtStartTime(_startTime!),
                style: TextStyle(
                  color: _startTime == null ? Colors.grey : null,
                ),
              ),
            ),
          ),
        ),
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
          label: Text(editing ? '保存修改' : '创建契约'),
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
          Switch(
            value: value,
            onChanged: onChanged,
            materialTapTargetSize: MaterialTapTargetSize.padded,
          ),
        ],
      ),
    );
  }
}
