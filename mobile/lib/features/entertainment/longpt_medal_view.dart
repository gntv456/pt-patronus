// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter/material.dart';
import 'package:ptpatronus/api/entertainment.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'ent_read_fail.dart';

// LongPT 六十四卦勋章：爻碎片合成 + 金字塔升级（太极→两仪→四象→八卦→六十四卦）。
class LongPTMedalView extends StatefulWidget {
  final int siteId;
  const LongPTMedalView({super.key, required this.siteId});
  @override
  State<LongPTMedalView> createState() => _LongPTMedalViewState();
}

class _LongPTMedalViewState extends State<LongPTMedalView> {
  static const _game = 'longpt_medal';
  static const _levels = [
    {'type': 'taiji', 'name': '太极'},
    {'type': 'liangyi', 'name': '两仪'},
    {'type': 'sixiang', 'name': '四象'},
    {'type': 'bagua', 'name': '八卦'},
    {'type': 'liushisi', 'name': '六十四卦'},
  ];

  final _api = EntertainmentApi();
  List<MedalNode> _tree = [];
  MedalShard? _shards;
  bool _loading = true;
  String _composing = ''; // 正在合成的 medalId
  int _converting = 0; // 正在转换方向
  String _fetchError = ''; // 勋章列表加载失败原因（诊断用）

  @override
  void initState() {
    super.initState();
    _refresh();
  }

  Future<void> _refresh() async {
    setState(() {
      _loading = true;
      _fetchError = '';
    });
    try {
      _tree = await _api.medalTree(widget.siteId, _game);
    } catch (e) {
      _fetchError = '$e';
    }
    try {
      _shards = await _api.medalShards(widget.siteId, _game);
    } catch (e) {
      if (mounted) entReadFailed(context, 'medalShards', e);
    }
    if (mounted) setState(() => _loading = false);
  }

  bool _isBig(String type) => type == 'bagua' || type == 'liushisi';

  List<MedalNode> _childrenOf(MedalNode n) => _tree.where((c) => c.parentId == n.medalId).toList();

  bool _canComposeBig(MedalNode n) {
    final kids = _childrenOf(n);
    return kids.isNotEmpty && kids.every((c) => c.owned);
  }

  // 碎片合成是否够（阳爻/阴爻余额 ≥ 所需）
  bool _canComposeShard(MedalNode n) {
    if (_shards == null) return false;
    return _shards!.yangYao >= n.needYang && _shards!.yinYao >= n.needYin;
  }

  // 综合：该勋章当前是否可合成
  bool _canCompose(MedalNode n) => _isBig(n.type) ? _canComposeBig(n) : _canComposeShard(n);

  String _costText(MedalNode n) =>
      _isBig(n.type) ? '消耗 ${n.bigCost} 魔力+子勋章' : '消耗 ${n.needYang} 阳+${n.needYin} 阴';

  Future<void> _confirmAndCompose(MedalNode n) async {
    if (_isBig(n.type) && !_canComposeBig(n)) {
      _toast('需要拥有全部子勋章才能合成');
      return;
    }
    final ok = await _confirm('合成确认', '是否${_costText(n)}合成【${n.name}】？');
    if (ok != true) return;
    setState(() => _composing = n.medalId);
    try {
      final res = _isBig(n.type)
          ? await _api.composeBigMedal(widget.siteId, _game, n.medalId)
          : await _api.composeMedal(widget.siteId, _game, n.medalId);
      _toast(res.success ? (res.message.isNotEmpty ? res.message : '合成成功') : (res.message.isNotEmpty ? res.message : '合成失败'));
      await _refresh();
    } catch (e) {
      _toast('请求失败：$e');
    } finally {
      if (mounted) setState(() => _composing = '');
    }
  }

  Future<void> _confirmAndConvert(int yiYang) async {
    final ok = await _confirm('阴阳转换', '是否花费 10000 魔力将 1 个${yiYang < 0 ? '阳爻' : '阴爻'}转换为${yiYang < 0 ? '阴爻' : '阳爻'}？');
    if (ok != true) return;
    setState(() => _converting = yiYang);
    try {
      final res = await _api.convertShard(widget.siteId, _game, yiYang);
      _toast(res.success ? (res.message.isNotEmpty ? res.message : '转换成功') : (res.message.isNotEmpty ? res.message : '转换失败'));
      await _refresh();
    } catch (e) {
      _toast('请求失败：$e');
    } finally {
      if (mounted) setState(() => _converting = 0);
    }
  }

  Future<bool?> _confirm(String title, String content) => showDialog<bool>(
        context: context,
        builder: (ctx) => AlertDialog(
          title: Text(title),
          content: Text(content),
          actions: [
            TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('取消')),
            TextButton(onPressed: () => Navigator.pop(ctx, true), child: const Text('确认')),
          ],
        ),
      );

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) return const PTLoadingView();
    return ListView(
      padding: const EdgeInsets.fromLTRB(16, 4, 16, 24),
      children: [
        _banner(),
        const SizedBox(height: 12),
        _shardBar(),
        const SizedBox(height: 14),
        ..._levels.map((l) {
          final nodes = _tree.where((n) => n.type == l['type']).toList();
          if (nodes.isEmpty) return const SizedBox.shrink();
          return Padding(padding: const EdgeInsets.only(bottom: 16), child: _level(l['name']!, nodes));
        }),
        if (_tree.isEmpty)
          Padding(
            padding: const EdgeInsets.all(24),
            child: Text(
              _fetchError.isNotEmpty ? '加载失败：$_fetchError' : '暂无勋章数据',
              style: TextStyle(color: _fetchError.isNotEmpty ? Colors.redAccent : context.dimText, fontSize: 12),
              textAlign: TextAlign.center,
            ),
          ),
      ],
    );
  }

  Widget _banner() => Container(
        padding: const EdgeInsets.symmetric(vertical: 14),
        decoration: BoxDecoration(borderRadius: BorderRadius.circular(14), gradient: const LinearGradient(colors: [Color(0xFF2D3436), Color(0xFF636E72)])),
        child: const Column(children: [
          Text('☯ 六十四卦勋章', style: TextStyle(fontSize: 22, fontWeight: FontWeight.w800, color: Color(0xFFFFD76A), letterSpacing: 2)),
          SizedBox(height: 3),
          Text('收集阳爻/阴爻碎片，合成太极→两仪→四象→八卦→六十四卦', style: TextStyle(fontSize: 11, color: Colors.white70), textAlign: TextAlign.center),
        ]),
      );

  Widget _shardBar() => GlassCard(
        padding: const EdgeInsets.all(14),
        child: Column(children: [
          Row(children: [
            _shardTile('⚊', _shards?.yangYao, '阳爻'),
            const Spacer(),
            _convertBtn(-1, '阳→阴', _shards?.yangYao ?? 0),
            const SizedBox(width: 20),
            _shardTile('⚋', _shards?.yinYao, '阴爻'),
            const Spacer(),
            _convertBtn(1, '阴→阳', _shards?.yinYao ?? 0),
          ]),
          const SizedBox(height: 8),
          Text('阴阳互转：10000 魔力 / 个', style: TextStyle(fontSize: 11, color: context.dimText)),
        ]),
      );

  Widget _shardTile(String icon, int? val, String label) => Row(mainAxisSize: MainAxisSize.min, children: [
        Text(icon, style: const TextStyle(fontSize: 26, color: Color(0xFF6C5CE7))),
        const SizedBox(width: 6),
        Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Text('${val ?? '--'}', style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w800)),
          Text(label, style: TextStyle(fontSize: 11, color: context.dimText)),
        ]),
      ]);

  Widget _convertBtn(int yiYang, String label, int cur) => SizedBox(
        width: 64,
        child: FilledButton.tonal(
          style: FilledButton.styleFrom(padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 4), textStyle: const TextStyle(fontSize: 11)),
          onPressed: (cur < 1 || _converting == yiYang) ? null : () => _confirmAndConvert(yiYang),
          child: _converting == yiYang ? const SizedBox(width: 14, height: 14, child: CircularProgressIndicator(strokeWidth: 2)) : Text(label),
        ),
      );

  Widget _level(String name, List<MedalNode> nodes) {
    final owned = nodes.where((n) => n.owned).length;
    return Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
      Padding(padding: const EdgeInsets.only(bottom: 8), child: Text('$name勋章  $owned/${nodes.length}', style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 15))),
      GridView.count(
        shrinkWrap: true,
        physics: const NeverScrollableScrollPhysics(),
        crossAxisCount: 3,
        mainAxisSpacing: 10,
        crossAxisSpacing: 10,
        childAspectRatio: 0.72,
        children: nodes.map(_nodeCard).toList(),
      ),
    ]);
  }

  Widget _nodeCard(MedalNode n) {
    final big = _isBig(n.type);
    final ok = _canCompose(n);
    return Container(
      padding: const EdgeInsets.all(8),
      decoration: BoxDecoration(
        color: Colors.white.withValues(alpha: 0.06),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: n.owned ? const Color(0x662ECC71) : Colors.white.withValues(alpha: 0.08)),
      ),
      child: Column(children: [
        SizedBox(
          width: 48,
          height: 48,
          child: Stack(clipBehavior: Clip.none, children: [
            Center(
              child: n.imageUrl.isNotEmpty
                  // 未拥有滤镜用 dimText（Colors.grey 固定色在暗色下对比度差）
                  ? Image.network(n.imageUrl, width: 48, height: 48, color: n.owned ? null : context.dimText.withValues(alpha: 0.6), errorBuilder: (_, _, _) => Text('☯', style: TextStyle(fontSize: 32, color: context.dimText)))
                  : Text('☯', style: TextStyle(fontSize: 32, color: context.dimText)),
            ),
            if (n.owned)
              const Positioned(top: -4, right: -4, child: CircleAvatar(radius: 9, backgroundColor: Color(0xFF2ECC71), child: Icon(Icons.check, size: 12, color: Colors.white))),
          ]),
        ),
        const SizedBox(height: 4),
        Text(n.name, textAlign: TextAlign.center, style: const TextStyle(fontSize: 11, fontWeight: FontWeight.w600), maxLines: 1, overflow: TextOverflow.ellipsis),
        const SizedBox(height: 2),
        Text(_costText(n), textAlign: TextAlign.center, style: TextStyle(fontSize: 9, color: context.dimText), maxLines: 2),
        const Spacer(),
        if (n.owned)
          const Text('已拥有', style: TextStyle(fontSize: 11, color: Color(0xFF2ECC71), fontWeight: FontWeight.w600))
        else
          SizedBox(
            width: double.infinity,
            height: 24,
            child: FilledButton(
              style: FilledButton.styleFrom(padding: EdgeInsets.zero, textStyle: const TextStyle(fontSize: 11), backgroundColor: ok ? null : Colors.grey),
              onPressed: (_composing.isNotEmpty || !ok) ? null : () => _confirmAndCompose(n),
              child: _composing == n.medalId
                  ? const SizedBox(width: 12, height: 12, child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white))
                  : Text(ok ? (big ? '大合成' : '合成') : (big ? '子勋章不足' : '碎片不足')),
            ),
          ),
      ]),
    );
  }
}
