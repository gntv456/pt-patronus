import 'package:flutter/material.dart';
import 'package:ptpatronus/api/entertainment.dart';
import 'package:ptpatronus/theme/glass.dart';

// LongPT 每日抽奖：今日奖品卡片网格 + 立即参与 + 参与人数/开奖时间 + 获奖记录。
// 每天一次，后端权威防重（join 返回 code/msg）。
class LongPTDailyView extends StatefulWidget {
  final int siteId;
  const LongPTDailyView({super.key, required this.siteId});
  @override
  State<LongPTDailyView> createState() => _LongPTDailyViewState();
}

class _LongPTDailyViewState extends State<LongPTDailyView> {
  static const _game = 'longpt_daily';

  final _api = EntertainmentApi();
  List<PrizeDef> _prizes = [];
  GameStats? _stats;
  List<GameRecord> _records = [];
  bool _joining = false;
  PlayResult? _joined;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    try {
      _prizes = await _api.prizes(widget.siteId, _game);
    } catch (_) {}
    await _refresh();
    if (mounted) setState(() {});
  }

  Future<void> _refresh() async {
    try {
      _stats = await _api.stats(widget.siteId, _game);
    } catch (_) {}
    try {
      _records = await _api.history(widget.siteId, _game);
    } catch (_) {}
  }

  // 每日奖品图标按名称关键词推断（奖品名来自站点 desc 字段）
  String _emoji(String name) {
    if (RegExp(r'魔力|火花').hasMatch(name)) return '🪙';
    if (name.contains('上传')) return '📤';
    if (RegExp(r'VIP|会员').hasMatch(name)) return '👑';
    if (name.contains('邀请')) return '✉️';
    if (name.contains('勋章')) return '🎖️';
    if (name.contains('彩虹')) return '🌈';
    return '🎁';
  }

  Future<void> _join() async {
    if (_joining) return;
    setState(() => _joining = true);
    try {
      final res = await _api.play(widget.siteId, _game);
      _joined = res;
      _toast(res.success ? (res.message.isNotEmpty ? res.message : '参与成功，等待开奖') : (res.message.isNotEmpty ? res.message : '参与失败'));
      await _refresh();
    } catch (e) {
      _toast('请求失败：$e');
    } finally {
      if (mounted) setState(() => _joining = false);
    }
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.fromLTRB(16, 4, 16, 24),
      children: [
        _hero(),
        const SizedBox(height: 14),
        Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Expanded(child: _prizesSection()),
          const SizedBox(width: 12),
          Expanded(child: _joinSection()),
        ]),
        if (_joined != null) ...[const SizedBox(height: 12), _resultView()],
        if (_records.isNotEmpty) ...[const SizedBox(height: 14), _recordsView()],
      ],
    );
  }

  Widget _hero() => Container(
        padding: const EdgeInsets.symmetric(vertical: 16),
        decoration: BoxDecoration(
          borderRadius: BorderRadius.circular(14),
          gradient: const LinearGradient(colors: [Color(0xFF1E3A8A), Color(0xFF3B5BDB)]),
        ),
        child: Column(children: [
          const Text('🎊 每日抽奖', style: TextStyle(fontSize: 22, fontWeight: FontWeight.w800, color: Colors.white, letterSpacing: 2)),
          const SizedBox(height: 4),
          Text('每人每天仅限参与一次${_stats != null && _stats!.openTime.isNotEmpty ? '，${_stats!.openTime} 统一开奖' : ''}',
              style: const TextStyle(fontSize: 12, color: Colors.white70)),
        ]),
      );

  Widget _sectionTitle(String t) => Padding(
        padding: const EdgeInsets.only(bottom: 10),
        child: Text(t, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
      );

  Widget _prizesSection() => Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        _sectionTitle('今日奖品'),
        if (_prizes.isEmpty)
          const Padding(padding: EdgeInsets.all(16), child: Text('暂无奖品配置', style: TextStyle(color: Colors.grey, fontSize: 13)))
        else
          GridView.count(
            shrinkWrap: true,
            physics: const NeverScrollableScrollPhysics(),
            crossAxisCount: 3,
            mainAxisSpacing: 8,
            crossAxisSpacing: 8,
            childAspectRatio: 0.85,
            children: _prizes.map((p) => Container(
                  decoration: BoxDecoration(color: Colors.white.withValues(alpha: 0.06), borderRadius: BorderRadius.circular(12), border: Border.all(color: Colors.white.withValues(alpha: 0.08))),
                  child: Column(mainAxisAlignment: MainAxisAlignment.center, children: [
                    Text(_emoji(p.name), style: const TextStyle(fontSize: 24)),
                    const SizedBox(height: 4),
                    Text(p.name, textAlign: TextAlign.center, style: const TextStyle(fontSize: 11, fontWeight: FontWeight.w600), maxLines: 2, overflow: TextOverflow.ellipsis),
                  ]),
                )).toList(),
          ),
      ]);

  Widget _joinSection() => Column(children: [
        _sectionTitle('立即参与'),
        GestureDetector(
          onTap: _joining ? null : _join,
          child: Container(
            width: 120,
            height: 120,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              gradient: const LinearGradient(colors: [Color(0xFF22C55E), Color(0xFF16A34A)]),
              boxShadow: const [BoxShadow(color: Color(0x7322C55E), blurRadius: 16, offset: Offset(0, 6))],
            ),
            child: Center(
              child: _joining
                  ? const SizedBox(width: 28, height: 28, child: CircularProgressIndicator(color: Colors.white, strokeWidth: 3))
                  : Column(mainAxisSize: MainAxisSize.min, children: [
                      const Text('🎁', style: TextStyle(fontSize: 30)),
                      const SizedBox(height: 4),
                      Text(_joined != null && _joined!.success ? '已参与' : '立即参与', style: const TextStyle(color: Colors.white, fontWeight: FontWeight.w700, fontSize: 13)),
                    ]),
            ),
          ),
        ),
        const SizedBox(height: 14),
        if (_stats != null) ...[
          if (_stats!.openTime.isNotEmpty) _metaRow('🔔', '开奖时间 ${_stats!.openTime}'),
          if (_stats!.startTime.isNotEmpty) _metaRow('🕒', '参与 ${_stats!.startTime}-${_stats!.endTime}'),
          if (_stats!.participants > 0) _metaRow('👥', '已参与 ${_stats!.participants} 人'),
        ],
      ]);

  Widget _metaRow(String icon, String text) => Container(
        margin: const EdgeInsets.only(bottom: 6),
        padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
        decoration: BoxDecoration(color: const Color(0xFF1E3A8A).withValues(alpha: 0.12), borderRadius: BorderRadius.circular(10)),
        child: Row(mainAxisSize: MainAxisSize.min, children: [
          Text(icon, style: const TextStyle(fontSize: 13)),
          const SizedBox(width: 6),
          Flexible(child: Text(text, style: const TextStyle(fontSize: 11))),
        ]),
      );

  Widget _resultView() => Container(
        padding: const EdgeInsets.all(10),
        decoration: BoxDecoration(
          color: _joined!.success ? PTColors.success.withValues(alpha: 0.12) : Colors.redAccent.withValues(alpha: 0.10),
          borderRadius: BorderRadius.circular(10),
        ),
        child: Row(children: [
          Text(_joined!.success ? '✓ 已参与' : '✗ 失败', style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 13)),
          const SizedBox(width: 8),
          Expanded(child: Text(_joined!.message, style: const TextStyle(fontSize: 13))),
        ]),
      );

  Widget _recordsView() => GlassCard(
        padding: const EdgeInsets.all(14),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          const Text('获奖记录', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
          const SizedBox(height: 8),
          ..._records.take(10).map((r) => Padding(
                padding: const EdgeInsets.symmetric(vertical: 4),
                child: Row(children: [
                  Text(r.prizeName, style: TextStyle(fontWeight: FontWeight.w600, fontSize: 13, color: r.prizeName == '未中奖' ? Colors.grey : null)),
                  if (r.amount > 0) Padding(padding: const EdgeInsets.only(left: 6), child: Text('×${r.amount.toStringAsFixed(0)}', style: const TextStyle(color: Colors.orange, fontSize: 13))),
                  const Spacer(),
                  Text(r.createdAt, style: const TextStyle(fontSize: 11, color: Colors.grey)),
                ]),
              )),
        ]),
      );
}
