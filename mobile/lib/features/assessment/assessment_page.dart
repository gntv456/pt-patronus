import 'package:flutter/material.dart';
import 'package:ptpatronus/api/assessment.dart';
import 'package:ptpatronus/theme/glass.dart';

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
          title: const Text('站点考核'),
          backgroundColor: Colors.transparent,
          elevation: 0,
          actions: [
            IconButton(icon: const Icon(Icons.refresh), onPressed: _loading ? null : () => _load(refresh: true)),
          ],
        ),
        body: RefreshIndicator(
          onRefresh: () => _load(refresh: true),
          child: _loading
              ? const Center(child: CircularProgressIndicator())
              : _error != null
                  ? _Empty(text: _error!, onRetry: _load)
                  : ListView(
                      padding: const EdgeInsets.all(12),
                      children: [
                        _filterBar(),
                        const SizedBox(height: 8),
                        ..._filtered.map(_card),
                      ],
                    ),
        ),
      ),
    );
  }

  Widget _filterBar() {
    final chips = [
      ('all', '全部 (${_cards.length})', const Color(0xFF6C5CE7)),
      ('safe', '安全 (${_count('safe')})', const Color(0xFF2ECC71)),
      ('progress', '进行 (${_count('progress')})', const Color(0xFF3498DB)),
      ('danger', '危险 (${_count('danger')})', const Color(0xFFF56C6C)),
    ];
    return SizedBox(
      height: 40,
      child: ListView(
        scrollDirection: Axis.horizontal,
        children: chips
            .map((c) => GestureDetector(
                  onTap: () => setState(() => _filter = c.$1),
                  child: Container(
                    margin: const EdgeInsets.only(right: 8),
                    padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 8),
                    decoration: BoxDecoration(
                      color: _filter == c.$1 ? c.$3.withValues(alpha: 0.16) : Colors.transparent,
                      border: Border.all(color: _filter == c.$1 ? c.$3.withValues(alpha: 0.4) : Colors.transparent),
                      borderRadius: BorderRadius.circular(999),
                    ),
                    child: Text(c.$2, style: TextStyle(color: _filter == c.$1 ? c.$3 : null, fontSize: 13)),
                  ),
                ))
            .toList(),
      ),
    );
  }

  Widget _card(AssessmentCard c) {
    final color = _statusColor(c.status);
    return Container(
      margin: const EdgeInsets.only(bottom: 8),
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
}

class _Empty extends StatelessWidget {
  final String text;
  final VoidCallback? onRetry;
  const _Empty({required this.text, this.onRetry});
  @override
  Widget build(BuildContext context) {
    return ListView(children: [
      const SizedBox(height: 120),
      Center(child: Padding(padding: const EdgeInsets.all(24), child: Text(text, textAlign: TextAlign.center, style: const TextStyle(color: Colors.grey)))),
      if (onRetry != null) Center(child: ElevatedButton(onPressed: onRetry, child: const Text('重试'))),
    ]);
  }
}
