import 'dart:async';

import 'package:flutter/material.dart';
import 'package:ptpatronus/api/entertainment.dart';
import 'package:ptpatronus/theme/glass.dart';

// 好学九宫格：3×3（外圈 8 格顺时针 + 中心按钮），中奖后高亮循环减速落到中奖格。
// 连抽 10/20/50/100 走站点 /jgg/magic_grid_bulk.php，一次返回 N 份结果（无单一中奖格，仅装饰性流转）。
class JggGameView extends StatefulWidget {
  final int siteId;
  const JggGameView({super.key, required this.siteId});
  @override
  State<JggGameView> createState() => _JggGameViewState();
}

class _JggGameViewState extends State<JggGameView> {
  final _api = EntertainmentApi();
  final _prizes = <_Prize>[
    _Prize(1, '勋章', '🎊', '#4CAF50'),
    _Prize(2, '临时邀请码', '⏱️', '#2196F3'),
    _Prize(3, '1GB上传', '💾', '#9C27B0'),
    _Prize(4, '10GB上传', '💾', '#FF9800'),
    _Prize(5, '10GB下载', '📥', '#00BCD4'),
    _Prize(6, 'VIP(1天)', '👑', '#FF5722'),
    _Prize(7, '邀请码', '✉️', '#795548'),
    _Prize(8, '谢谢参与', '⭐', '#607D8B'),
  ];
  final _clockwise = <int>[]; // id 1-8 顺时针位置 0-7
  // 网格行主序：0..7 为顺时针索引，-1 为中心
  final _layout = [
    [0, 1, 2],
    [7, -1, 3],
    [6, 5, 4],
  ];
  // 连抽档位（站点仅接受 10/20/50/100）
  final _bulkOptions = [
    _BulkOption(10, '🎀'),
    _BulkOption(20, '💖'),
    _BulkOption(50, '✨'),
    _BulkOption(100, '🌸'),
  ];

  GameStats? _stats;
  bool _spinning = false;
  int _highlight = -1; // 顺时针索引；-1 无
  PlayResult? _result;

  bool _bulkSpinning = false;
  PlayResult? _bulk;
  int _bulkCount = 0;
  List<GameRecord> _records = const [];

  @override
  void initState() {
    super.initState();
    _clockwise.addAll(_prizes.map((p) => p.id));
    _refreshStats();
    _refreshHistory();
  }

  int _mapIdToPos(int id) {
    if (id >= 1 && id <= 8) return id - 1;
    if (id == 9) return 5;
    return 7;
  }

  Future<void> _refreshStats() async {
    try {
      _stats = await _api.stats(widget.siteId, 'jgg');
    } catch (_) {}
    if (mounted) setState(() {});
  }

  Future<void> _refreshHistory() async {
    try {
      _records = await _api.history(widget.siteId, 'jgg');
    } catch (_) {}
    if (mounted) setState(() {});
  }

  Future<void> _spin() async {
    if (_spinning || _bulkSpinning) return;
    setState(() {
      _spinning = true;
      _highlight = -1;
      _result = null;
      _bulk = null;
    });
    try {
      final res = await _api.play(widget.siteId, 'jgg');
      if (res.success) {
        final target = _mapIdToPos(res.prizeId);
        final total = 3 * 8 + target;
        var delay = 70;
        for (var i = 0; i <= total; i++) {
          if (!mounted) break; // 页面销毁中断动画循环（审计 P2）
          _highlight = i % 8;
          setState(() {});
          await Future.delayed(Duration(milliseconds: delay));
          delay = (delay + 8).clamp(70, 280);
        }
        if (!mounted) return; // 销毁则跳过结算与循环末尾的无用刷新请求
        _result = res;
        _toast(res.prize.isNotEmpty && res.prize != '谢谢参与' ? '🎉 获得 ${res.prize}' : (res.message.isNotEmpty ? res.message : '谢谢参与'));
      } else {
        _result = res;
        _toast(res.message.isNotEmpty ? res.message : '抽奖失败');
      }
      await _refreshStats();
      await _refreshHistory();
    } catch (e) {
      _toast('请求失败：$e');
    } finally {
      if (mounted) setState(() => _spinning = false);
    }
  }

  // 连抽：一次返回 N 份结果，无单一中奖格，仅做装饰性流转反馈。
  Future<void> _drawMulti(int count) async {
    if (_spinning || _bulkSpinning) return;
    setState(() {
      _bulkSpinning = true;
      _highlight = -1;
      _bulk = null;
      _result = null;
    });
    try {
      var delay = 55;
      for (var i = 0; i < 18; i++) {
        if (!mounted) break;
        _highlight = i % 8;
        setState(() {});
        await Future.delayed(Duration(milliseconds: delay));
      }
      if (!mounted) return;
      _highlight = -1;
      final res = await _api.playMulti(widget.siteId, 'jgg', count);
      if (res.success) {
        _bulk = res;
        _bulkCount = count;
        final wins = res.prizes.where((p) => p.isWin).length;
        _toast('🎉 $count连抽完成，中奖 $wins/$count');
      } else {
        _toast(res.message.isNotEmpty ? res.message : '连抽失败');
      }
      await _refreshStats();
      await _refreshHistory();
    } catch (e) {
      _toast('请求失败：$e');
    } finally {
      if (mounted) setState(() => _bulkSpinning = false);
    }
  }

  // 连抽结果按奖品名分组统计（等价于站点 prize_stats）。
  List<MapEntry<String, int>> _bulkStats() {
    final m = <String, int>{};
    for (final p in _bulk?.prizes ?? const <PrizeOutcome>[]) {
      m[p.prizeName] = (m[p.prizeName] ?? 0) + 1;
    }
    return m.entries.toList()..sort((a, b) => b.value.compareTo(a.value));
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  @override
  Widget build(BuildContext context) {
    final busy = _spinning || _bulkSpinning;
    return ListView(
      padding: const EdgeInsets.fromLTRB(16, 4, 16, 24),
      children: [
        _banner(),
        const SizedBox(height: 12),
        _statsBar(),
        const SizedBox(height: 14),
        _grid(busy),
        if (_result != null) ...[const SizedBox(height: 12), _resultView(_result!)],
        const SizedBox(height: 12),
        _bulkActions(busy),
        if (_bulk != null) ...[const SizedBox(height: 12), _bulkView()],
        if (_records.isNotEmpty) ...[const SizedBox(height: 12), _historyView()],
      ],
    );
  }

  Widget _banner() => Container(
        padding: const EdgeInsets.symmetric(vertical: 14),
        decoration: BoxDecoration(
          borderRadius: BorderRadius.circular(14),
          gradient: const LinearGradient(colors: [Color(0xFF6C5CE7), Color(0xFF8E44AD)]),
        ),
        child: Column(children: [
          const Text('九宫格抽奖', style: TextStyle(fontSize: 22, fontWeight: FontWeight.w800, color: Color(0xFFFFD76A), letterSpacing: 3)),
          const SizedBox(height: 3),
          Text('单次消耗 ${(_stats?.costPerPlay ?? 3000).round()} 火花', style: const TextStyle(fontSize: 12, color: Colors.white70)),
        ]),
      );

  Widget _statsBar() => Row(children: [
        _statTile(_stats != null ? '${_stats!.todaySpins}/${_stats!.maxDaily > 0 ? _stats!.maxDaily : '∞'}' : '--', '今日次数'),
        const SizedBox(width: 8),
        _statTile(_result != null || _bulk != null ? (_bulk?.balance ?? _result!.balance).toStringAsFixed(0) : (_stats != null ? '—' : '--'),
            _result != null || _bulk != null ? '当前火花' : '火花余额'),
      ]);

  Widget _statTile(String value, String label) => Expanded(
        child: Container(
          padding: const EdgeInsets.symmetric(vertical: 10),
          decoration: BoxDecoration(color: Colors.white.withValues(alpha: 0.06), borderRadius: BorderRadius.circular(12), border: Border.all(color: const Color(0x38FFD76A))),
          child: Column(children: [
            Text(value, style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w800, color: Color(0xFFFFD76A))),
            const SizedBox(height: 2),
            Text(label, style: const TextStyle(fontSize: 11, color: Colors.grey)),
          ]),
        ),
      );

  Widget _grid(bool busy) => Column(
        children: _layout.map((row) {
          return Padding(
            padding: const EdgeInsets.only(bottom: 8),
            child: Row(
              children: row.map((pos) {
                if (pos == -1) {
                  return Expanded(child: AspectRatio(aspectRatio: 1, child: _centerBtn(busy)));
                }
                return Expanded(child: AspectRatio(aspectRatio: 1, child: Padding(padding: const EdgeInsets.only(right: 8), child: _cell(pos))));
              }).toList(),
            ),
          );
        }).toList(),
      );

  Widget _centerBtn(bool busy) => GestureDetector(
        onTap: busy ? null : _spin,
        child: Container(
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(12),
            gradient: const LinearGradient(colors: [Color(0xFF6C5CE7), Color(0xFF4285F4)]),
            boxShadow: const [BoxShadow(color: Color(0x666C5CE7), blurRadius: 12, offset: Offset(0, 4))],
          ),
          child: Center(
            child: busy
                ? const SizedBox(width: 28, height: 28, child: CircularProgressIndicator(color: Colors.white, strokeWidth: 3))
                : Column(mainAxisSize: MainAxisSize.min, children: const [
                    Text('⚡', style: TextStyle(fontSize: 26)),
                    Text('开始\n抽奖', textAlign: TextAlign.center, style: TextStyle(color: Colors.white, fontWeight: FontWeight.w800, fontSize: 12, height: 1.2)),
                  ]),
          ),
        ),
      );

  Widget _cell(int pos) {
    final prize = _prizes[pos];
    final lit = _highlight == pos;
    final win = _result != null && lit && !_spinning;
    return AnimatedContainer(
      duration: const Duration(milliseconds: 120),
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(12),
        color: lit ? _color(prize.color) : _color(prize.color).withValues(alpha: 0.32),
        border: lit ? Border.all(color: const Color(0xFFFFD76A), width: 2) : null,
        boxShadow: lit ? [BoxShadow(color: _color(prize.color), blurRadius: 16)] : null,
      ),
      child: Column(mainAxisAlignment: MainAxisAlignment.center, children: [
        Text(prize.icon, style: TextStyle(fontSize: 24, color: lit && win ? null : Colors.white)),
        const SizedBox(height: 2),
        Text(prize.name, textAlign: TextAlign.center, style: TextStyle(fontSize: 11, fontWeight: lit ? FontWeight.bold : FontWeight.normal, color: Colors.white), maxLines: 2),
      ]),
    );
  }

  // 连抽按钮：2×2，emoji 档位。
  Widget _bulkActions(bool busy) => Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text('连抽', style: TextStyle(fontSize: 13, fontWeight: FontWeight.bold)),
          const SizedBox(height: 8),
          GridView.count(
            crossAxisCount: 2,
            shrinkWrap: true,
            physics: const NeverScrollableScrollPhysics(),
            mainAxisSpacing: 8,
            crossAxisSpacing: 8,
            childAspectRatio: 3.2,
            children: _bulkOptions
                .map((o) => GestureDetector(
                      onTap: busy ? null : () => _drawMulti(o.count),
                      child: Container(
                        decoration: BoxDecoration(
                          color: Colors.white.withValues(alpha: 0.06),
                          borderRadius: BorderRadius.circular(12),
                          border: Border.all(color: const Color(0x386C5CE7)),
                        ),
                        child: Center(
                          child: Text('${o.icon} ${o.count}连抽', style: TextStyle(fontSize: 14, fontWeight: FontWeight.bold, color: busy ? Colors.grey : Colors.white)),
                        ),
                      ),
                    ))
                .toList(),
          ),
        ],
      );

  // 连抽结果：分组统计 + 消耗/余额。
  Widget _bulkView() {
    final stats = _bulkStats();
    final wins = _bulk!.prizes.where((p) => p.isWin).length;
    final cost = _bulkCount * (_stats?.costPerPlay ?? 3000).round();
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: PTColors.success.withValues(alpha: 0.1),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: PTColors.success.withValues(alpha: 0.3)),
      ),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Text('🎉 $_bulkCount 连抽完成 · 中奖 $wins/$_bulkCount', style: const TextStyle(fontSize: 13, fontWeight: FontWeight.bold, color: PTColors.success)),
        const SizedBox(height: 6),
        for (final e in stats)
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 2),
            child: Row(children: [
              Text(e.key, style: TextStyle(fontSize: 13, color: e.key == '谢谢参与' ? Colors.grey : null)),
              const Spacer(),
              Text('×${e.value}', style: const TextStyle(fontSize: 13, fontWeight: FontWeight.bold, color: Color(0xFFFFD76A))),
            ]),
          ),
        const SizedBox(height: 6),
        Text('消耗 $cost 火花 · 余额 ${_bulk!.balance.toStringAsFixed(0)}', style: const TextStyle(fontSize: 11, color: Colors.grey)),
      ]),
    );
  }

  // 抽奖历史。
  Widget _historyView() => Container(
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(
          color: Colors.white.withValues(alpha: 0.04),
          borderRadius: BorderRadius.circular(12),
          border: Border.all(color: Colors.white.withValues(alpha: 0.08)),
        ),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          const Text('📜 抽奖历史', style: TextStyle(fontSize: 13, fontWeight: FontWeight.bold)),
          const SizedBox(height: 6),
          for (final r in _records.take(20))
            Padding(
              padding: const EdgeInsets.symmetric(vertical: 3),
              child: Row(children: [
                Expanded(child: Text(r.prizeName, style: TextStyle(fontSize: 12, color: r.prizeName == '谢谢参与' ? Colors.grey : null))),
                Text('-${r.cost.toStringAsFixed(0)}', style: const TextStyle(fontSize: 12, color: Colors.redAccent, fontWeight: FontWeight.bold)),
                const SizedBox(width: 8),
                Text(r.createdAt, style: const TextStyle(fontSize: 11, color: Colors.grey)),
              ]),
            ),
        ]),
      );

  Color _color(String hex) {
    final h = hex.replaceAll('#', '');
    return Color(int.tryParse('FF$h', radix: 16) ?? 0xFF6366F1);
  }

  Widget _resultView(PlayResult r) => Container(
        padding: const EdgeInsets.all(10),
        decoration: BoxDecoration(
          color: r.success ? PTColors.success.withValues(alpha: 0.12) : Colors.redAccent.withValues(alpha: 0.10),
          borderRadius: BorderRadius.circular(10),
        ),
        child: Wrap(crossAxisAlignment: WrapCrossAlignment.center, spacing: 8, children: [
          Text(r.success ? '🎉 中奖' : '✗ 失败', style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 13)),
          if (r.prize.isNotEmpty) Text(r.prize, style: const TextStyle(color: Color(0xFFFFD76A), fontWeight: FontWeight.w700, fontSize: 13)),
          if (r.amount > 0) Text('×${r.amount.toStringAsFixed(0)}', style: const TextStyle(color: Colors.orange, fontWeight: FontWeight.w700, fontSize: 13)),
        ]),
      );
}

class _Prize {
  final int id;
  final String name;
  final String icon;
  final String color;
  _Prize(this.id, this.name, this.icon, this.color);
}

class _BulkOption {
  final int count;
  final String icon;
  _BulkOption(this.count, this.icon);
}
