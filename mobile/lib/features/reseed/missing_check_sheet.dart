// 从 reseed_page.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/features/reseed/reseed_page.dart';

/// 未辅种检查面板（showAdaptiveSheet 内容）：多选目标站 → 调 POST /keepseed/missing
/// → 渲染每站缺失数与示例种子。自持选中态 / 结果 / loading。
class _MissingCheckSheet extends StatefulWidget {
  final int downloaderId;
  final String downloaderName;
  const _MissingCheckSheet({
    required this.downloaderId,
    required this.downloaderName,
  });
  @override
  State<_MissingCheckSheet> createState() => _MissingCheckSheetState();
}

class _MissingCheckSheetState extends State<_MissingCheckSheet> {
  final _api = ReseedApi();
  final _siteApi = SiteApi();
  List<Site> _sites = [];
  final _selected = <int>{};
  MissingSummary? _result;
  bool _loading = false;
  bool _loadingSites = true;
  String? _error;

  @override
  void initState() {
    super.initState();
    _loadSites();
  }

  Future<void> _loadSites() async {
    try {
      final s = await _siteApi.list();
      if (!mounted) return;
      final active = s.where((e) => e.isActive).toList();
      setState(() {
        _sites = active;
        _selected.addAll(active.map((e) => e.id)); // 默认全选：典型用法是查全部启用站
        _loadingSites = false;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _error = '站点加载失败：$e';
        _loadingSites = false;
      });
    }
  }

  void _toast(String msg) {
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
  }

  Future<void> _run() async {
    if (_selected.isEmpty) {
      _toast('请至少选择一个目标站点');
      return;
    }
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final r = await _api.missing(widget.downloaderId, _selected.toList());
      if (!mounted) return;
      setState(() => _result = r);
    } catch (e) {
      if (!mounted) return;
      setState(() => _error = '检查失败：$e');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_loadingSites) {
      return const Padding(
        padding: EdgeInsets.all(32),
        child: PTLoadingView(),
      );
    }
    // 站点加载失败 / 无活跃站：compact 错误 + 重试（sheet 内不用 PTEmptyState 的 96px 留白）。
    if (_sites.isEmpty) {
      return Padding(
        padding: const EdgeInsets.symmetric(vertical: 24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              Icons.error_outline,
              size: 40,
              color: PTColors.danger.withValues(alpha: 0.6),
            ),
            const SizedBox(height: 8),
            Text(
              _error ?? '暂无可用站点',
              textAlign: TextAlign.center,
              style: TextStyle(fontSize: 13, color: context.dimText),
            ),
            if (_error != null) ...[
              const SizedBox(height: 12),
              OutlinedButton(
                onPressed: () {
                  setState(() {
                    _error = null;
                    _loadingSites = true;
                  });
                  _loadSites();
                },
                child: const Text('重试'),
              ),
            ],
          ],
        ),
      );
    }
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(
          '源下载器：${widget.downloaderName}',
          style: TextStyle(fontSize: 13, color: context.dimText),
        ),
        if (_result != null) ...[
          const SizedBox(height: 10),
          _summary(_result!),
        ],
        const SizedBox(height: 14),
        Row(
          children: [
            const Expanded(
              child: Text(
                '目标站点',
                style: TextStyle(fontWeight: FontWeight.w600, fontSize: 14),
              ),
            ),
            TextButton(
              onPressed: () => setState(() {
                if (_selected.length == _sites.length) {
                  _selected.clear();
                } else {
                  _selected.addAll(_sites.map((e) => e.id));
                }
              }),
              child: Text(
                _selected.length == _sites.length ? '清空' : '全选',
                style: const TextStyle(fontSize: 13),
              ),
            ),
          ],
        ),
        Wrap(
          spacing: 8,
          runSpacing: 8,
          children: _sites
              .map((s) {
                final on = _selected.contains(s.id);
                return FilterChip(
                  label: Text(s.name),
                  selected: on,
                  onSelected: (v) => setState(() {
                    if (v) {
                      _selected.add(s.id);
                    } else {
                      _selected.remove(s.id);
                    }
                  }),
                  materialTapTargetSize: MaterialTapTargetSize.padded,
                );
              })
              .toList(),
        ),
        const SizedBox(height: 14),
        SizedBox(
          width: double.infinity,
          child: FilledButton.icon(
            onPressed: _loading ? null : _run,
            icon: _loading
                ? const SizedBox(
                    width: 16,
                    height: 16,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : const Icon(Icons.search, size: 18),
            label: Text(_loading ? '检查中…' : '检查未辅种'),
          ),
        ),
        if (_error != null) ...[
          const SizedBox(height: 10),
          Container(
            padding: const EdgeInsets.all(10),
            decoration: BoxDecoration(
              color: PTColors.danger.withValues(alpha: 0.12),
              borderRadius: BorderRadius.circular(8),
            ),
            child: Row(
              children: [
                const Icon(Icons.error_outline, size: 18, color: PTColors.danger),
                const SizedBox(width: 6),
                Expanded(
                  child: Text(
                    _error!,
                    style: const TextStyle(fontSize: 13, color: PTColors.danger),
                  ),
                ),
              ],
            ),
          ),
        ],
        if (_result != null) ...[
          const SizedBox(height: 14),
          Row(
            children: [
              Text(
                '检查结果',
                style: TextStyle(fontWeight: FontWeight.w700, fontSize: 15),
              ),
              const SizedBox(width: 6),
              Text(
                '（${_result!.missing.length} 站）',
                style: TextStyle(fontSize: 12, color: context.dimText),
              ),
            ],
          ),
          const SizedBox(height: 8),
          if (_result!.missing.isEmpty)
            Padding(
              padding: EdgeInsets.symmetric(vertical: 12),
              child: Center(
                child: Text('暂无数据', style: TextStyle(color: context.dimText, fontSize: 13)),
              ),
            )
          else
            for (final m in _result!.missing)
              Padding(
                padding: const EdgeInsets.only(bottom: 8),
                child: _missingRow(m),
              ),
        ],
      ],
    );
  }

  // 顶部摘要：源做种 / 已提交 / 已映射 三指标。
  Widget _summary(MissingSummary r) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      decoration: BoxDecoration(
        color: PTColors.primary.withValues(alpha: 0.08),
        borderRadius: BorderRadius.circular(10),
      ),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceAround,
        children: [
          _stat('${r.total}', '源做种'),
          _stat('${r.checked}', '提交'),
          _stat('${r.targets}', '已映射'),
        ],
      ),
    );
  }

  Widget _stat(String value, String label) {
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          value,
          style: const TextStyle(
            fontSize: 18,
            fontWeight: FontWeight.w700,
            color: PTColors.primary,
          ),
        ),
        const SizedBox(height: 2),
        Text(label, style: TextStyle(fontSize: 12, color: context.dimText)),
      ],
    );
  }

  // 单站缺失行：count<0 无法判定(灰) / count==0 已全覆盖(绿) / count>0 N 个未辅种(橙) + 示例。
  Widget _missingRow(MissingSeed m) {
    final Color c;
    final String label;
    if (m.count < 0) {
      c = context.dimText;
      label = '无法判定';
    } else if (m.count == 0) {
      c = PTColors.success;
      label = '已全覆盖';
    } else {
      c = PTColors.warn;
      label = '${m.count} 个未辅种';
    }
    return GlassCard(
      padding: const EdgeInsets.all(12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Expanded(
                child: Text(
                  m.siteName,
                  style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 14),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
              ),
              const SizedBox(width: 8),
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
                decoration: BoxDecoration(
                  color: c.withValues(alpha: 0.16),
                  borderRadius: BorderRadius.circular(999),
                ),
                child: Text(
                  label,
                  style: TextStyle(
                    color: c,
                    fontSize: 12,
                    fontWeight: FontWeight.w600,
                  ),
                ),
              ),
            ],
          ),
          if (m.samples.isNotEmpty) ...[
            const SizedBox(height: 6),
            for (final s in m.samples.take(3))
              Padding(
                padding: const EdgeInsets.only(top: 2),
                child: Text(
                  '· ${s.name.isEmpty ? s.hash : s.name}',
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(fontSize: 12, color: context.dimText),
                ),
              ),
          ],
        ],
      ),
    );
  }
}
