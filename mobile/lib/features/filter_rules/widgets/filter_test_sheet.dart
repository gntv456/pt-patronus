import 'package:flutter/material.dart';
import 'package:ptpatronus/api/filter_rules.dart';
import 'package:ptpatronus/theme/glass.dart';

/// 规则测试面板（POST /filter/test）：粘贴 rule_string + 一批种子标题（每行一条）→
/// 展示命中（带 pri_order，越大越优）/ 被剔除 / 命中的层级明细。
class FilterTestSheet extends StatefulWidget {
  final FilterRuleApi api;
  final String initialRule; // 默认填入的 rule_string（如来自某规则组）

  const FilterTestSheet({super.key, required this.api, this.initialRule = ''});

  @override
  State<FilterTestSheet> createState() => _FilterTestSheetState();
}

class _FilterTestSheetState extends State<FilterTestSheet> {
  late final TextEditingController _ruleCtrl;
  late final TextEditingController _titlesCtrl;
  bool _running = false;
  FilterTestResult? _result;
  String? _error;

  @override
  void initState() {
    super.initState();
    _ruleCtrl = TextEditingController(text: widget.initialRule);
    _titlesCtrl = TextEditingController();
  }

  @override
  void dispose() {
    _ruleCtrl.dispose();
    _titlesCtrl.dispose();
    super.dispose();
  }

  Future<void> _run() async {
    final rule = _ruleCtrl.text.trim();
    if (rule.isEmpty) {
      _toast('请填写 rule_string');
      return;
    }
    final titles = _titlesCtrl.text
        .split('\n')
        .map((s) => s.trim())
        .where((s) => s.isNotEmpty)
        .toList();
    if (titles.isEmpty) {
      _toast('请输入至少一条种子标题');
      return;
    }
    setState(() {
      _running = true;
      _error = null;
      _result = null;
    });
    try {
      final torrents = titles.map((t) => FilterTestTorrent(title: t)).toList();
      final r = await widget.api.test(rule, torrents);
      if (mounted) setState(() => _result = r);
    } catch (e) {
      if (mounted) setState(() => _error = '$e');
    } finally {
      if (mounted) setState(() => _running = false);
    }
  }

  void _toast(String msg) {
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(content: Text(msg), duration: const Duration(seconds: 2)),
    );
  }

  @override
  Widget build(BuildContext context) {
    final r = _result;
    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      TextField(
        controller: _ruleCtrl,
        maxLines: 2,
        style: const TextStyle(fontFamily: 'monospace', fontSize: 13),
        decoration: const InputDecoration(
          labelText: 'rule_string',
          alignLabelWithHint: true,
          isDense: true,
          helperText: '> 分优先级层；! & | () 连接规则 id',
          hintText: '例：4K & !BLU > 1080P',
        ),
      ),
      const SizedBox(height: 8),
      TextField(
        controller: _titlesCtrl,
        maxLines: 5,
        decoration: const InputDecoration(
          labelText: '种子标题（每行一条）',
          alignLabelWithHint: true,
          isDense: true,
          hintText: '粘贴一批种子标题进行测试',
        ),
      ),
      const SizedBox(height: 12),
      FilledButton.icon(
        onPressed: _running ? null : _run,
        icon: _running
            ? const SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2))
            : const Icon(Icons.play_arrow, size: 18),
        label: const Text('测试'),
      ),
      if (_error != null) ...[
        const SizedBox(height: 12),
        Text(_error!, style: const TextStyle(color: PTColors.danger, fontSize: 12)),
      ],
      if (r != null) ...[
        const SizedBox(height: 16),
        _resultView(r),
      ],
    ]);
  }

  Widget _resultView(FilterTestResult r) {
    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      if (r.levels.isNotEmpty)
        Wrap(
          spacing: 6,
          runSpacing: 6,
          children: [
            const Text('命中层级：', style: TextStyle(fontSize: 12, fontWeight: FontWeight.w600)),
            for (final lv in r.levels)
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
                decoration: BoxDecoration(
                  color: PTColors.primary.withValues(alpha: 0.12),
                  borderRadius: BorderRadius.circular(6),
                ),
                child: Text(lv, style: const TextStyle(fontSize: 11, fontFamily: 'monospace')),
              ),
          ],
        ),
      const SizedBox(height: 10),
      _sectionHeader('命中 (${r.matched.length})', PTColors.success),
      const SizedBox(height: 4),
      if (r.matched.isEmpty)
        const Padding(padding: EdgeInsets.only(top: 2), child: Text('无', style: TextStyle(fontSize: 12, color: Colors.grey)))
      else
        ...r.matched.map((m) => ListTile(
              dense: true,
              contentPadding: EdgeInsets.zero,
              leading: CircleAvatar(
                radius: 14,
                backgroundColor: PTColors.success.withValues(alpha: 0.16),
                child: Text('${m.priOrder}', style: const TextStyle(fontSize: 11, color: PTColors.success, fontWeight: FontWeight.w700)),
              ),
              title: Text(m.torrent.title, maxLines: 2, overflow: TextOverflow.ellipsis, style: const TextStyle(fontSize: 13)),
              subtitle: const Text('pri_order 越大越优', style: TextStyle(fontSize: 10, color: Colors.grey)),
            )),
      const SizedBox(height: 8),
      _sectionHeader('剔除 (${r.dropped.length})', PTColors.danger),
      const SizedBox(height: 4),
      if (r.dropped.isEmpty)
        const Padding(padding: EdgeInsets.only(top: 2), child: Text('无', style: TextStyle(fontSize: 12, color: Colors.grey)))
      else
        ...r.dropped.map((t) => ListTile(
              dense: true,
              contentPadding: EdgeInsets.zero,
              leading: const Icon(Icons.remove_circle_outline, size: 18, color: PTColors.danger),
              title: Text(t.title, maxLines: 2, overflow: TextOverflow.ellipsis, style: const TextStyle(fontSize: 13)),
            )),
    ]);
  }

  Widget _sectionHeader(String text, Color color) {
    return Text(text, style: TextStyle(fontSize: 12, fontWeight: FontWeight.w700, color: color));
  }
}
