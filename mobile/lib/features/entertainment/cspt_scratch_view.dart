import 'dart:async';
import 'ent_read_fail.dart';

import 'package:flutter/material.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/api/entertainment.dart';


// 财神刮刮乐（古风金玉）：统计（剩余刮卡/金元宝余额）+ 刮卡 + 单抽/自动 + 购卡档位。
class CsptScratchView extends StatefulWidget {
  final int siteId;
  const CsptScratchView({super.key, required this.siteId});
  @override
  State<CsptScratchView> createState() => _CsptScratchViewState();
}

class _CsptScratchViewState extends State<CsptScratchView> {
  final _api = EntertainmentApi();
  GameStats? _stats;
  List<BuyOption> _options = [];
  bool _playing = false;
  bool _buying = false;
  bool _auto = false;
  int _autoCount = 0;
  PlayResult? _reveal;
  Timer? _autoTimer;

  @override
  void initState() {
    super.initState();
    _loadAll();
  }

  @override
  void dispose() {
    _autoTimer?.cancel();
    super.dispose();
  }

  Future<void> _loadAll() async {
    try {
      final results = await Future.wait([
        _api.stats(widget.siteId, 'cspt_scratch').catchError((e) { if (mounted) entReadFailed(context, 'panel', e); return GameStats(balance: 0, cardNumber: 0, todaySpins: 0, maxDaily: 0, costPerPlay: 2000); }),
        _api.buyOptions(widget.siteId, 'cspt_scratch').catchError((e) { if (mounted) entReadFailed(context, 'buyOptions', e); return <BuyOption>[]; }),
      ]);
      _stats = results[0] as GameStats;
      _options = results[1] as List<BuyOption>;
    } catch (e) {
      if (mounted) entReadFailed(context, 'buyOptions', e);
    }
    if (mounted) setState(() {});
  }

  Future<void> _refreshStats() async {
    try {
      _stats = await _api.stats(widget.siteId, 'cspt_scratch');
    } catch (e) {
      if (mounted) entReadFailed(context, 'stats', e);
    }
    if (mounted) setState(() {});
  }

  Future<void> _play() async {
    if (_playing) return;
    setState(() {
      _playing = true;
      _reveal = null;
    });
    try {
      final res = await _api.play(widget.siteId, 'cspt_scratch');
      _reveal = res;
      _toast(res.success ? (res.prize.isNotEmpty ? '获得 ${res.prize}' : '刮卡成功') : (res.message.isNotEmpty ? res.message : '失败'));
      await _refreshStats();
    } catch (e) {
      _toast('请求失败：$e');
    } finally {
      if (mounted) setState(() => _playing = false);
    }
  }

  void _toggleAuto() {
    if (_auto) {
      _autoTimer?.cancel();
      setState(() => _auto = false);
    } else {
      setState(() {
        _auto = true;
        _autoCount = 0;
      });
      _autoTimer = Timer.periodic(const Duration(milliseconds: 800), (_) async {
        if (!mounted) {
          _autoTimer?.cancel();
          return;
        }
        // 卡用完 → 停止自动刮卡
        if ((_stats?.cardNumber ?? 0) <= 0) {
          _autoTimer?.cancel();
          setState(() => _auto = false);
          return;
        }
        // 上一张还在刮（网络未回）→ 跳过本次 tick 等下次，不可取消：
        // 旧逻辑把「在刮」当「该停」直接 cancel，导致自动模式只刮一张就停。
        if (_playing) return;
        await _play();
        _autoCount++;
        if (mounted) setState(() {});
      });
    }
  }

  Future<void> _buy(BuyOption opt) async {
    if (_buying) return;
    setState(() => _buying = true);
    try {
      final res = await _api.buy(widget.siteId, 'cspt_scratch', opt.type);
      _toast(res.success ? (res.message.isNotEmpty ? res.message : '购买成功') : (res.message.isNotEmpty ? res.message : '购买失败'));
      await _refreshStats();
    } catch (e) {
      _toast('购买失败：$e');
    } finally {
      if (mounted) setState(() => _buying = false);
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
        Row(children: [
          Expanded(child: _btn('刮一张', _playing, _play, const Color(0xFF388E3C))),
          const SizedBox(width: 8),
          Expanded(child: _btn(_auto ? '停止($_autoCount)' : '自动刮卡', false, _toggleAuto, _auto ? Colors.redAccent : const Color(0xFF1F6F4A))),
        ]),
        if (_options.isNotEmpty) ...[const SizedBox(height: 16), _buySection()],
      ],
    );
  }

  Widget _banner() => Container(
        padding: const EdgeInsets.symmetric(vertical: 14),
        decoration: BoxDecoration(
          borderRadius: BorderRadius.circular(14),
          gradient: const LinearGradient(colors: [Color(0xFF1F6F4A), Color(0xFF3A7D44)]),
          border: Border.all(color: const Color(0x80FFC850)),
        ),
        child: Column(children: [
          const Text('富豪刮刮乐', style: TextStyle(fontSize: 22, fontWeight: FontWeight.w800, color: Color(0xFFFFD76A), letterSpacing: 4)),
          const SizedBox(height: 3),
          Text('单卡 ${(_stats?.costPerPlay ?? 2000).round()} 金元宝', style: const TextStyle(fontSize: 12, color: Colors.white70)),
        ]),
      );

  Widget _statsBar() => Row(children: [
        _stat((_stats?.cardNumber ?? 0).toString(), '剩余刮刮卡'),
        const SizedBox(width: 8),
        _stat(_stats != null ? _fmt(_stats!.balance) : '--', '金元宝余额'),
      ]);

  Widget _stat(String v, String l) => Expanded(
        child: Container(
          padding: const EdgeInsets.symmetric(vertical: 10),
          decoration: BoxDecoration(color: Colors.white.withValues(alpha: 0.06), borderRadius: BorderRadius.circular(12), border: Border.all(color: const Color(0x38FFD76A))),
          child: Column(children: [
            Text(v, style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w800, color: Color(0xFFFFD76A))),
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
            gradient: const LinearGradient(colors: [Color(0xFFF4D67A), Color(0xFFE89B3C)]),
            border: Border.all(color: const Color(0xFFFFD76A), width: 2),
          ),
          child: _reveal == null
              ? Center(
                  child: Column(mainAxisSize: MainAxisSize.min, children: const [
                    Text('刮奖区', style: TextStyle(fontSize: 28, fontWeight: FontWeight.w800, color: Color(0xFF6B4A00), letterSpacing: 6)),
                    SizedBox(height: 4),
                    Text('点下方按钮刮开', style: TextStyle(fontSize: 11, color: Color(0x996B4A00))),
                  ]),
                )
              : Center(
                  child: Column(mainAxisSize: MainAxisSize.min, children: [
                    Text((_reveal!.amount > 0) ? '🎊' : '·', style: const TextStyle(fontSize: 30)),
                    Text(_reveal!.prize.isNotEmpty ? _reveal!.prize : (_reveal!.success ? '未中奖' : '失败'), style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w800, color: Color(0xFF4A2F00))),
                    if (_reveal!.amount > 0) Text('×${_fmt(_reveal!.amount)}', style: const TextStyle(fontWeight: FontWeight.w700, color: Color(0xFF4A2F00))),
                  ]),
                ),
        ),
      );

  Widget _btn(String label, bool busy, VoidCallback onTap, Color color) => GestureDetector(
        onTap: busy ? null : onTap,
        child: Container(
          height: 44,
          decoration: BoxDecoration(color: color, borderRadius: BorderRadius.circular(12)),
          child: Center(child: busy ? const SizedBox(width: 18, height: 18, child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white)) : Text(label, style: const TextStyle(color: Colors.white, fontWeight: FontWeight.w700))),
        ),
      );

  Widget _buySection() => Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        const Text('购买刮刮卡', style: TextStyle(fontSize: 13, fontWeight: FontWeight.w700, color: Color(0xFFFFD76A))),
        const SizedBox(height: 8),
        Row(children: _options.map((o) => Expanded(child: Padding(padding: const EdgeInsets.only(right: 6), child: _buyBtn(o)))).toList()),
      ]);

  Widget _buyBtn(BuyOption o) => GestureDetector(
        onTap: _buying ? null : () => _buy(o),
        child: Container(
          padding: const EdgeInsets.symmetric(vertical: 10),
          decoration: BoxDecoration(color: Colors.white.withValues(alpha: 0.06), borderRadius: BorderRadius.circular(10), border: Border.all(color: const Color(0x4DFFD76A))),
          child: Column(children: [
            Text(o.giftCount > 0 ? '买${o.buyCount}送${o.giftCount}' : '${o.buyCount}张', style: const TextStyle(fontWeight: FontWeight.w700, color: Color(0xFFFFD76A), fontSize: 12)),
            const SizedBox(height: 2),
            Text(_fmt(o.buyCount * o.unitPrice), style: TextStyle(fontSize: 11, color: context.dimText)),
          ]),
        ),
      );
}
