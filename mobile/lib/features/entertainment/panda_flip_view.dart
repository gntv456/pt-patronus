// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:ptpatronus/api/entertainment.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'package:web_socket_channel/web_socket_channel.dart';

// PandaPT 翻面夺宝：10×10 卡片网格 + 奖项/规则侧栏 + 本局记录。
// 点格子→bet(selection=index, extra.round_number)→刷新 FlipState。
// 他人翻格实时同步：后端桥接站点 wss://host/wss，前端连 /flip-ws 收 tileFlip 事件（不做轮询）。
class PandaFlipView extends StatefulWidget {
  final int siteId;
  const PandaFlipView({super.key, required this.siteId});
  @override
  State<PandaFlipView> createState() => _PandaFlipViewState();
}

class _PandaFlipViewState extends State<PandaFlipView> {
  final _api = EntertainmentApi();
  FlipState? _state;
  bool _loading = true;
  bool _refreshing = false;
  int? _flipping; // 正在翻的格子 index
  int _liveFlips = 0; // websocket 实时收到的他人翻格数（叠加到 flippedCount 显示）
  WebSocketChannel? _flipCh;
  StreamSubscription? _flipSub;
  bool _wsStopped = false;
  static const _game = 'panda_flip';

  // 兜底配色（站点 tier.color 缺失时按 tier 顺序）
  static const _palette = [
    Color(0xFFFACC15), Color(0xFF9CA3AF), Color(0xFFB45309), Color(0xFF3B82F6),
    Color(0xFF22C55E), Color(0xFFA855F7), Color(0xFFEC4899),
  ];

  static const _rules = [
    '点击任意卡片翻面',
    '每次翻面消耗对应魔力',
    '翻中对应奖项即获奖励',
    '特等奖/全翻完开启新一轮',
    '奖项随机分布',
    '翻面过快可能卡顿（非 bug）',
    '最终解释权归站点所有',
  ];

  @override
  void initState() {
    super.initState();
    _load();
    _connectFlipWs();
  }

  @override
  void dispose() {
    _wsStopped = true;
    _flipSub?.cancel();
    _flipCh?.sink.close();
    super.dispose();
  }

  Future<void> _refresh() async {
    setState(() => _refreshing = true);
    await _load(silent: true);
    if (mounted) setState(() => _refreshing = false);
  }

  // ── websocket 桥接：收站点 tileFlip 推送，实时标记他人翻的格 ──
  Future<void> _connectFlipWs() async {
    if (_wsStopped) return;
    final api = ApiClient();
    if (api.token == null || api.token!.isEmpty) return;
    String ticket;
    try {
      ticket = (await api.post('/auth/ws-ticket', {}))['ticket'] as String? ?? '';
    } catch (_) {
      return;
    }
    if (ticket.isEmpty || _wsStopped) return;
    final base = api.baseUrl.replaceFirst(RegExp(r'^http'), 'ws');
    final uri = Uri.parse('$base/api/v1/entertainment/${widget.siteId}/games/$_game/flip-ws?ticket=${Uri.encodeQueryComponent(ticket)}');
    try {
      _flipCh = WebSocketChannel.connect(uri);
      _flipSub = _flipCh!.stream.listen(
        (raw) {
          try {
            final ev = jsonDecode(raw as String) as Map<String, dynamic>;
            if (ev['type'] == 'tileFlip' && ev['tile_index'] != null) {
              _applyTileFlip((ev['tile_index'] as num).toInt(), ev['tier_id']?.toString() ?? '', ev['tier_name']?.toString() ?? '', ev['player']?.toString() ?? '');
            } else if (ev['type'] == 'special') {
              _toast('🎉 特等奖出现，新局即将开始');
            }
          } catch (_) {}
        },
        onDone: () { if (!_wsStopped) Future.delayed(const Duration(seconds: 3), _connectFlipWs); },
        onError: (_) { if (!_wsStopped) Future.delayed(const Duration(seconds: 3), _connectFlipWs); },
      );
    } catch (_) {}
  }

  // 收到他人翻格事件：标记该格已翻 + 记入本局记录（去重）
  void _applyTileFlip(int index, String tierId, String tierName, String player) {
    final s = _state;
    if (s == null || s.tiles.any((t) => t.index == index)) return;
    setState(() {
      s.tiles.add(FlipTile(index: index, flipped: true, tierId: tierId, byUser: player));
      final label = '${player.isEmpty ? '?' : player} 翻了 #$index${tierName.isNotEmpty ? ' · $tierName' : ''}';
      s.history.insert(0, GameRecord(prizeName: label, prizeType: tierId, amount: 0, cost: 0, createdAt: ''));
      if (s.history.length > 50) s.history.removeLast();
      _liveFlips++;
    });
  }

  Future<void> _load({bool silent = false}) async {
    try {
      _state = await _api.flipState(widget.siteId, _game);
    } catch (e) {
      if (!silent && mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('加载翻面夺宝失败：$e')));
      }
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Color _tierColor(String tierId) {
    final idx = _state!.tiers.indexWhere((t) => t.id == tierId);
    if (idx < 0) return const Color(0xFF6366F1);
    final t = _state!.tiers[idx];
    if (t.color.isNotEmpty) {
      final hex = t.color.replaceAll('#', '');
      return Color(int.tryParse('FF$hex', radix: 16) ?? 0xFF6366F1);
    }
    return _palette[idx % _palette.length];
  }

  String _tierName(String tierId) {
    final i = _state?.tiers.indexWhere((t) => t.id == tierId) ?? -1;
    return i >= 0 ? _state!.tiers[i].name : '';
  }

  Future<void> _flip(int index) async {
    if (_flipping != null || _state == null) return;
    final byIndex = {for (var t in _state!.tiles) t.index: t};
    if (byIndex[index]?.flipped ?? false) return;
    setState(() => _flipping = index);
    try {
      final res = await _api.bet(widget.siteId, _game, BetRequest(
        selection: index.toString(),
        amount: 0,
        extra: {'round_number': (_state!.roundNumber > 0 ? _state!.roundNumber : 1).toString()},
      ));
      if (res.success) {
        final win = res.prize.isNotEmpty && !res.prize.contains('未中奖');
        _toast(win ? '🎉 ${res.prize}' : (res.message.isNotEmpty ? res.message : '未中奖，继续加油'));
      } else {
        _toast(res.message.isNotEmpty ? res.message : '翻牌失败');
      }
      await _load();
    } catch (e) {
      _toast('请求失败：$e');
    } finally {
      if (mounted) setState(() => _flipping = null);
    }
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) return const PTLoadingView();
    final s = _state;
    return ListView(
      padding: const EdgeInsets.fromLTRB(16, 4, 16, 24),
      children: [
        _hero(s),
        const SizedBox(height: 12),
        _statBar(s),
        const SizedBox(height: 12),
        _grid(s),
        const SizedBox(height: 12),
        _tiersPanel(s),
        const SizedBox(height: 12),
        _rulesPanel(),
        if (s != null && s.history.isNotEmpty) ...[const SizedBox(height: 12), _historyPanel(s)],
      ],
    );
  }

  Widget _hero(FlipState? s) => Container(
        padding: const EdgeInsets.symmetric(vertical: 16),
        decoration: BoxDecoration(
          borderRadius: BorderRadius.circular(14),
          gradient: const LinearGradient(colors: [Color(0xFFDB2777), Color(0xFFF59E0B)]),
        ),
        child: Column(children: [
          const Text('🃏 翻面夺宝', style: TextStyle(fontSize: 22, fontWeight: FontWeight.w800, color: Colors.white)),
          const SizedBox(height: 4),
          Text('余额 ${s != null ? s.balance.toStringAsFixed(0) : '—'}', style: const TextStyle(fontSize: 13, color: Colors.white)),
        ]),
      );

  Widget _statBar(FlipState? s) => Row(children: [
        _stat('${(s?.flippedCount ?? 0) + _liveFlips}/${s?.totalTiles ?? 100}', '游戏状态', const Color(0xFF6C5CE7)),
        const SizedBox(width: 10),
        _stat(s != null && s.costPerFlip > 0 ? s.costPerFlip.toStringAsFixed(0) : '—', '翻牌成本', const Color(0xFFF59E0B)),
        const SizedBox(width: 10),
        _stat('第 ${s?.roundNumber ?? '?'} 局', '当前轮次', const Color(0xFF22C55E)),
        const SizedBox(width: 10),
        _refreshBtn(),
      ]);

  Widget _refreshBtn() => SizedBox(
        width: 48,
        child: Tooltip(
          message: '刷新他人翻的格子',
          child: TextButton(
            onPressed: _refreshing ? null : _refresh,
            style: TextButton.styleFrom(
              padding: EdgeInsets.zero,
              shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
              backgroundColor: Colors.white.withValues(alpha: 0.06),
            ),
            child: _refreshing
                ? const SizedBox(width: 18, height: 18, child: CircularProgressIndicator(strokeWidth: 2))
                : const Icon(Icons.refresh, size: 22, color: Color(0xFF6C5CE7)),
          ),
        ),
      );

  Widget _stat(String val, String lbl, Color c) => Expanded(
        child: Container(
          padding: const EdgeInsets.symmetric(vertical: 12),
          decoration: BoxDecoration(
            color: Colors.white.withValues(alpha: 0.06),
            borderRadius: BorderRadius.circular(12),
            border: Border.all(color: Colors.white.withValues(alpha: 0.08)),
          ),
          child: Column(children: [
            Text(val, style: TextStyle(fontSize: 16, fontWeight: FontWeight.w800, color: c)),
            const SizedBox(height: 2),
            Text(lbl, style: TextStyle(fontSize: 11, color: context.dimText)),
          ]),
        ),
      );

  Widget _grid(FlipState? s) {
    final total = s?.totalTiles ?? 100;
    final byIndex = {for (var t in (s?.tiles ?? <FlipTile>[])) t.index: t};
    return GlassCard(
      padding: const EdgeInsets.all(10),
      child: GridView.builder(
        shrinkWrap: true,
        physics: const NeverScrollableScrollPhysics(),
        itemCount: total,
        gridDelegate: const SliverGridDelegateWithFixedCrossAxisCount(crossAxisCount: 10, mainAxisSpacing: 4, crossAxisSpacing: 4),
        itemBuilder: (context, i) {
          final idx = i + 1;
          final t = byIndex[idx];
          final flipped = t?.flipped ?? false;
          final tierId = t?.tierId ?? '';
          final isFlipping = _flipping == idx;
          return GestureDetector(
            onTap: (_flipping != null || flipped) ? null : () => _flip(idx),
            child: AnimatedContainer(
              duration: const Duration(milliseconds: 200),
              alignment: Alignment.center,
              decoration: BoxDecoration(
                borderRadius: BorderRadius.circular(7),
                gradient: flipped && tierId.isNotEmpty
                    ? null
                    : (flipped ? null : const LinearGradient(colors: [Color(0xFF7C3AED), Color(0xFF6366F1)])),
                color: flipped && tierId.isNotEmpty ? _tierColor(tierId) : (flipped ? Colors.white.withValues(alpha: 0.1) : null),
              ),
              child: isFlipping
                  ? const SizedBox(width: 14, height: 14, child: CircularProgressIndicator(color: Colors.white, strokeWidth: 2))
                  : flipped && tierId.isNotEmpty
                      ? Text(_tierName(tierId), style: const TextStyle(color: Colors.white, fontSize: 9, fontWeight: FontWeight.w700),
                          textAlign: TextAlign.center, maxLines: 2, overflow: TextOverflow.ellipsis)
                      : Text(flipped ? '$idx' : '?', style: TextStyle(color: Colors.white, fontSize: flipped ? 9 : 18, fontWeight: FontWeight.w800)),
            ),
          );
        },
      ),
    );
  }

  Widget _tiersPanel(FlipState? s) => GlassCard(
        padding: const EdgeInsets.all(14),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          const Text('奖项详情', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
          const SizedBox(height: 8),
          if (s == null || s.tiers.isEmpty)
            Text('加载中…', style: TextStyle(color: context.dimText, fontSize: 13))
          else
            ...s.tiers.asMap().entries.map((e) {
              final t = e.value;
              final c = t.color.isNotEmpty
                  ? _colorFromHex(t.color)
                  : _palette[e.key % _palette.length];
              return Padding(
                padding: const EdgeInsets.symmetric(vertical: 4),
                child: Row(children: [
                  Container(width: 6, height: 18, decoration: BoxDecoration(color: c, borderRadius: BorderRadius.circular(3))),
                  const SizedBox(width: 8),
                  Text(t.name, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 12)),
                  const Spacer(),
                  Text('${t.reward.toStringAsFixed(0)} 魔力 × ${t.count} 张', style: TextStyle(color: context.dimText, fontSize: 11)),
                ]),
              );
            }),
        ]),
      );

  Widget _rulesPanel() => GlassCard(
        padding: const EdgeInsets.all(14),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          const Text('游戏规则', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
          const SizedBox(height: 6),
          ..._rules.asMap().entries.map((e) => Padding(
                padding: const EdgeInsets.symmetric(vertical: 2),
                child: Text('${e.key + 1}. ${e.value}', style: TextStyle(fontSize: 12, color: context.dimText, height: 1.5)),
              )),
        ]),
      );

  Widget _historyPanel(FlipState s) => GlassCard(
        padding: const EdgeInsets.all(14),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          const Text('本局翻牌记录', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
          const SizedBox(height: 8),
          ...s.history.take(12).map((r) => Padding(
                padding: const EdgeInsets.symmetric(vertical: 4),
                child: Row(children: [
                  Expanded(child: Text(r.prizeName, style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w600))),
                  if (r.amount > 0) Padding(padding: const EdgeInsets.only(left: 6), child: Text('+${r.amount.toStringAsFixed(0)}', style: const TextStyle(color: Color(0xFF22C55E), fontSize: 13))),
                  if (r.createdAt.isNotEmpty) Text(r.createdAt, style: TextStyle(fontSize: 11, color: context.dimText)),
                ]),
              )),
        ]),
      );

  Color _colorFromHex(String hex) {
    final h = hex.replaceAll('#', '');
    return Color(int.tryParse('FF$h', radix: 16) ?? 0xFF6366F1);
  }
}
