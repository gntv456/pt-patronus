// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter/material.dart';
import 'package:ptpatronus/api/entertainment.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'ent_read_fail.dart';

// 魔力刮刮乐（火花/琥珀）：统计（火花余额/今日次数）+ 刮卡 + 刮一张。
class MagicScratchView extends StatefulWidget {
  final int siteId;
  const MagicScratchView({super.key, required this.siteId});
  @override
  State<MagicScratchView> createState() => _MagicScratchViewState();
}

class _MagicScratchViewState extends State<MagicScratchView> {
  final _api = EntertainmentApi();
  GameStats? _stats;
  bool _playing = false;
  PlayResult? _reveal;

  @override
  void initState() {
    super.initState();
    _refreshStats();
  }

  Future<void> _refreshStats() async {
    try {
      _stats = await _api.stats(widget.siteId, 'magic_scratch');
    } catch (e) {
      if (mounted) entReadFailed(context, 'stats', e);
    }
    if (mounted) setState(() {});
  }

  Future<void> _scratch() async {
    if (_playing) return;
    setState(() {
      _playing = true;
      _reveal = null;
    });
    try {
      final res = await _api.play(widget.siteId, 'magic_scratch');
      _reveal = res;
      _toast(res.success
          ? (res.amount > 0 ? '获得 ${res.prize} ${res.amount.toStringAsFixed(0)}' : (res.message.isNotEmpty ? res.message : '未中奖'))
          : (res.message.isNotEmpty ? res.message : '失败'));
      await _refreshStats();
    } catch (e) {
      _toast('请求失败：$e');
    } finally {
      if (mounted) setState(() => _playing = false);
    }
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  String _fmt(double n) => n.round().toString();

  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.fromLTRB(16, 4, 16, 24),
      children: [
        _banner(),
        const SizedBox(height: 12),
        _statsBar(),
        const SizedBox(height: 14),
        _scratchCard(),
        const SizedBox(height: 14),
        GestureDetector(
          onTap: _playing ? null : _scratch,
          child: Container(
            height: 46,
            decoration: BoxDecoration(borderRadius: BorderRadius.circular(12), gradient: const LinearGradient(colors: [Color(0xFFF7971E), Color(0xFFFF5858)])),
            child: Center(child: _playing ? const SizedBox(width: 18, height: 18, child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white)) : const Text('刮一张', style: TextStyle(color: Colors.white, fontWeight: FontWeight.w700, fontSize: 15))),
          ),
        ),
      ],
    );
  }

  Widget _banner() => Container(
        padding: const EdgeInsets.symmetric(vertical: 14),
        decoration: BoxDecoration(borderRadius: BorderRadius.circular(14), gradient: const LinearGradient(colors: [Color(0xFFF7971E), Color(0xFFFF5858)])),
        child: Column(children: [
          const Text('魔力刮刮乐', style: TextStyle(fontSize: 22, fontWeight: FontWeight.w800, color: Colors.white, letterSpacing: 3)),
          const SizedBox(height: 3),
          Text('消耗 ${(_stats?.costPerPlay ?? 5000).round()} 火花', style: const TextStyle(fontSize: 12, color: Colors.white70)),
        ]),
      );

  Widget _statsBar() => Row(children: [
        _stat(_stats != null ? _fmt(_stats!.balance) : '--', '火花余额'),
        const SizedBox(width: 8),
        _stat(_stats != null ? '${_stats!.todaySpins}/${_stats!.maxDaily > 0 ? _stats!.maxDaily : '∞'}' : '--', '今日次数'),
      ]);

  Widget _stat(String v, String l) => Expanded(
        child: Container(
          padding: const EdgeInsets.symmetric(vertical: 10),
          decoration: BoxDecoration(color: Colors.white.withValues(alpha: 0.06), borderRadius: BorderRadius.circular(12), border: Border.all(color: const Color(0x40FFB74D))),
          child: Column(children: [
            Text(v, style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w800, color: Color(0xFFFFB74D))),
            const SizedBox(height: 2),
            Text(l, style: TextStyle(fontSize: 11, color: context.dimText)),
          ]),
        ),
      );

  Widget _scratchCard() => Center(
        child: Container(
          width: 240,
          height: 150,
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(14),
            gradient: const LinearGradient(colors: [Color(0xFFFFD180), Color(0xFFFF8A3D)]),
            border: Border.all(color: const Color(0xFFFFB74D), width: 2),
          ),
          child: _reveal == null
              ? Center(child: Column(mainAxisSize: MainAxisSize.min, children: const [
                  Text('刮奖区', style: TextStyle(fontSize: 28, fontWeight: FontWeight.w800, color: Color(0xFF4A2F00), letterSpacing: 6)),
                  SizedBox(height: 4),
                  Text('点下方按钮刮开', style: TextStyle(fontSize: 11, color: Color(0x994A2F00))),
                ]))
              : Center(child: Column(mainAxisSize: MainAxisSize.min, children: [
                  Text(_reveal!.amount > 0 ? '✨' : '·', style: const TextStyle(fontSize: 30)),
                  Text(_reveal!.prize.isNotEmpty ? _reveal!.prize : (_reveal!.success ? '未中奖' : '失败'), style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w800, color: Color(0xFF4A2F00))),
                  if (_reveal!.amount > 0) Text('+${_fmt(_reveal!.amount)} 火花', style: const TextStyle(fontWeight: FontWeight.w700, color: Color(0xFF4A2F00))),
                ])),
        ),
      );
}
