import 'package:flutter/material.dart';
import 'package:ptpatronus/features/home/home_menu_button.dart';
import 'package:ptpatronus/api/assessment.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';

class AssessmentPage extends StatefulWidget {
  const AssessmentPage({super.key});
  @override
  State<AssessmentPage> createState() => _AssessmentPageState();
}

class _AssessmentPageState extends State<AssessmentPage> {
  final _api = AssessmentApi();
  List<AssessmentCard> _cards = [];
  bool _loading = true;
  String? _error;
  String _filter = 'all';

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load({bool refresh = false}) async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      _cards = await _api.all(refresh: refresh);
    } catch (e) {
      _error = e.toString();
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  List<AssessmentCard> get _filtered =>
      _filter == 'all' ? _cards : _cards.where((c) => c.status.toLowerCase() == _filter).toList();

  Color _statusColor(String s) => switch (s) {
        'SAFE' => const Color(0xFF2ECC71),
        'PROGRESS' => const Color(0xFF3498DB),
        'DANGER' => const Color(0xFFF56C6C),
        _ => const Color(0xFF95A5A6),
      };

  int _count(String s) => _cards.where((c) => c.status.toLowerCase() == s).length;

  @override
  Widget build(BuildContext context) {
    return GlassBackground(
      child: Scaffold(
        backgroundColor: Colors.transparent,
        appBar: AppBar(
          leading: const HomeMenuButton(),
          title: const Text('站点考核'),
          backgroundColor: Colors.transparent,
          elevation: 0,
          actions: [
            IconButton(icon: const Icon(Icons.refresh), onPressed: _loading ? null : () => _load(refresh: true)),
          ],
        ),
        body: Column(
          children: [
            Padding(padding: const EdgeInsets.fromLTRB(12, 8, 12, 0), child: _filterBar()),
            Expanded(
              child: _loading
                  ? const Center(child: CircularProgressIndicator())
                  : _error != null
                      ? PTEmptyState(text: _error!, onRetry: _load)
                      : _filtered.isEmpty
                          ? PTEmptyState(text: '该筛选下无站点')
                          : RefreshIndicator(
                              onRefresh: () => _load(refresh: true),
                              child: PTMasonry<AssessmentCard>(
                                items: _filtered,
                                minItemWidth: 320,
                                itemBuilder: (_, c) => _card(c),
                              ),
                            ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _filterBar() {
    final cur = _filter;
    String labelOf(String k) => switch (k) {
          'all' => '全部',
          'safe' => '安全',
          'progress' => '进行',
          'danger' => '危险',
          'exempt' => '免考',
          _ => k,
        };
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 6, 12, 8),
      child: Align(
        alignment: Alignment.centerLeft,
        child: PTFilterPill(
          label: '状态',
          value: labelOf(cur),
          active: cur != 'all',
          onTap: () => showFilterSheet<String>(
            context,
            title: '筛选状态',
            selected: cur,
            onSelected: (v) => setState(() => _filter = v),
            options: [
              PTFilterOption(value: 'all', label: '全部', count: _cards.length),
              PTFilterOption(value: 'safe', label: '安全', count: _count('safe')),
              PTFilterOption(value: 'progress', label: '进行', count: _count('progress')),
              PTFilterOption(value: 'danger', label: '危险', count: _count('danger')),
              PTFilterOption(value: 'exempt', label: '免考', count: _count('exempt')),
            ],
          ),
        ),
      ),
    );
  }

  Widget _card(AssessmentCard c) {
    final color = _statusColor(c.status);
    return Container(
      decoration: BoxDecoration(border: Border(left: BorderSide(color: color, width: 3))),
      child: GlassCard(
        padding: const EdgeInsets.all(14),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(children: [
              Expanded(child: Text(c.siteName, style: const TextStyle(fontWeight: FontWeight.w600))),
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
                decoration: BoxDecoration(color: color.withValues(alpha: 0.18), borderRadius: BorderRadius.circular(999)),
                child: Text(c.status, style: TextStyle(color: color, fontSize: 11, fontWeight: FontWeight.w600)),
              ),
            ]),
            if (c.error.isNotEmpty)
              Padding(padding: const EdgeInsets.only(top: 6), child: Text(c.error, style: const TextStyle(color: Color(0xFFF56C6C), fontSize: 13)))
            else ...[
              const SizedBox(height: 10),
              ClipRRect(
                borderRadius: BorderRadius.circular(4),
                child: LinearProgressIndicator(value: c.progressPercent / 100, minHeight: 8, backgroundColor: Colors.black12, valueColor: AlwaysStoppedAnimation(color)),
              ),
              const SizedBox(height: 4),
              Row(children: [
                Text('${c.progressPercent.toStringAsFixed(0)}%', style: const TextStyle(color: Colors.grey, fontSize: 12)),
                const Spacer(),
                if (c.endTime != null) ...[
                  Text('截止 ${_fmtDate(c.endTime)}', style: const TextStyle(color: Colors.grey, fontSize: 12)),
                  const SizedBox(width: 8),
                ],
                if (c.remainingDays >= 0) Text('剩 ${c.remainingDays} 天', style: const TextStyle(color: Colors.grey, fontSize: 12)),
              ]),
              if (c.assessmentType.isNotEmpty)
                Padding(padding: const EdgeInsets.only(top: 4), child: Text(c.assessmentType, style: const TextStyle(color: Color(0xFF6C5CE7), fontSize: 12))),
              if (c.description.isNotEmpty)
                Padding(padding: const EdgeInsets.only(top: 4), child: Text(c.description, style: const TextStyle(color: Colors.grey, fontSize: 12, height: 1.4))),
              if (c.metrics.isNotEmpty)
                Padding(
                  padding: const EdgeInsets.only(top: 8),
                  child: Column(
                    children: c.metrics
                        .map((m) => Padding(
                              padding: const EdgeInsets.only(top: 4),
                              child: Row(children: [
                                Expanded(child: Text(m.label, style: const TextStyle(fontSize: 13))),
                                Text('${m.current.toStringAsFixed(1)}/${m.required.toStringAsFixed(1)} ${m.unit}', style: const TextStyle(color: Colors.grey, fontSize: 12)),
                                const SizedBox(width: 8),
                                Text(m.isCompleted ? '✓' : '✗', style: TextStyle(color: m.isCompleted ? const Color(0xFF2ECC71) : const Color(0xFFF56C6C))),
                              ]),
                            ))
                        .toList(),
                  ),
                ),
            ],
          ],
        ),
      ),
    );
  }

  String _fmtDate(String? t) {
    if (t == null || t.isEmpty) return '—';
    final s = t.replaceAll('T', ' ');
    return s.length > 10 ? s.substring(0, 10) : s;
  }
}
