import 'package:flutter/material.dart';
import 'package:ptpatronus/api/entertainment.dart';
import 'package:ptpatronus/theme/glass.dart';

// NovaHD 魔力农场：余额 + 一键收获 + 农作物/动物网格 + 仓库 + 菜市场。
// 后端 magic_fram.php（GET + 整页 HTML），配置写死。
class MagicFarmView extends StatefulWidget {
  final int siteId;
  const MagicFarmView({super.key, required this.siteId});
  @override
  State<MagicFarmView> createState() => _MagicFarmViewState();
}

class _MagicFarmViewState extends State<MagicFarmView> {
  static const _game = 'magic_farm';
  final _api = EntertainmentApi();
  FarmState? _farm;
  bool _busy = false;
  PlayResult? _last;

  @override
  void initState() {
    super.initState();
    _refresh();
  }

  Future<void> _refresh() async {
    try {
      _farm = await _api.farm(widget.siteId, _game);
    } catch (_) {}
    if (mounted) setState(() {});
  }

  String _fmt(double v) => v % 1 == 0 ? v.toStringAsFixed(0) : v.toStringAsFixed(1);

  String _emoji(String name) {
    if (name.contains('鸡')) return '🐔';
    if (name.contains('猪')) return '🐷';
    if (name.contains('羊')) return '🐑';
    if (name.contains('牛')) return '🐮';
    if (name.contains('麦')) return '🌾';
    if (name.contains('玉米')) return '🌽';
    if (name.contains('花生')) return '🥜';
    if (name.contains('土豆')) return '🥔';
    return '🌱';
  }

  String _statusText(String s) {
    switch (s) {
      case 'ready':
        return '可收获';
      case 'growing':
        return '生长中';
      default:
        return '未种植';
    }
  }

  Future<void> _doAct(Future<PlayResult> Function() fn) async {
    if (_busy) return;
    setState(() => _busy = true);
    try {
      final res = await fn();
      _last = res;
      _toast(res.success ? (res.message.isNotEmpty ? res.message : '完成') : (res.message.isNotEmpty ? res.message : '操作失败'));
      await _refresh();
    } catch (e) {
      _toast('请求失败：$e');
    } finally {
      if (mounted) setState(() => _busy = false);
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
        const SizedBox(height: 12),
        if (_last != null) ...[_resultView(), const SizedBox(height: 12)],
        if (_farm != null) _gridSection('🌾 农作物种植区', '有效期 2 天', _farm!.crops),
        if (_farm != null) ...[const SizedBox(height: 12), _gridSection('🐔 动物养殖区', '有效期 5 天', _farm!.animals)],
        if (_farm?.warehouse.isNotEmpty == true) ...[const SizedBox(height: 12), _warehouseSection()],
        if (_farm?.market.isNotEmpty == true) ...[const SizedBox(height: 12), _marketSection()],
      ],
    );
  }

  Widget _hero() => Container(
        padding: const EdgeInsets.symmetric(vertical: 16, horizontal: 14),
        decoration: BoxDecoration(
          borderRadius: BorderRadius.circular(14),
          gradient: const LinearGradient(colors: [Color(0xFF166534), Color(0xFF16A34A)]),
        ),
        child: Column(children: [
          Text('🚜 ${_farm?.farmName.isNotEmpty == true ? _farm!.farmName : '魔力农场'}',
              style: const TextStyle(fontSize: 20, fontWeight: FontWeight.w800, color: Colors.white)),
          const SizedBox(height: 4),
          Text('当前魔力值: ${_farm != null ? _fmt(_farm!.balance) : '—'}',
              style: const TextStyle(fontSize: 13, color: Colors.white)),
          const SizedBox(height: 10),
          FilledButton.icon(
            onPressed: _busy ? null : () => _doAct(() => _api.harvestAll(widget.siteId, _game)),
            icon: const Text('🧺', style: TextStyle(fontSize: 16)),
            label: Text(_busy ? '处理中…' : '一键收获'),
            style: FilledButton.styleFrom(backgroundColor: Colors.white, foregroundColor: const Color(0xFF166534)),
          ),
        ]),
      );

  Widget _gridSection(String title, String sub, List<FarmItem> items) => GlassCard(
        padding: const EdgeInsets.all(14),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Row(children: [
            Text(title, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
            const SizedBox(width: 6),
            Text(sub, style: TextStyle(fontSize: 12, color: context.dimText)),
          ]),
          const SizedBox(height: 10),
          GridView.count(
            shrinkWrap: true,
            physics: const NeverScrollableScrollPhysics(),
            crossAxisCount: 2,
            mainAxisSpacing: 8,
            crossAxisSpacing: 8,
            childAspectRatio: 0.82,
            children: items.map(_itemCard).toList(),
          ),
        ]),
      );

  Widget _itemCard(FarmItem it) {
    final ready = it.status == 'ready';
    final growing = it.status == 'growing';
    return Container(
      padding: const EdgeInsets.symmetric(vertical: 8),
      decoration: BoxDecoration(color: Colors.white.withValues(alpha: 0.06), borderRadius: BorderRadius.circular(12)),
      child: Column(mainAxisAlignment: MainAxisAlignment.center, children: [
        Text(_emoji(it.name), style: const TextStyle(fontSize: 28)),
        const SizedBox(height: 2),
        Text(it.name, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 14)),
        Text('成长 ${it.growHours}h · 买 ${_fmt(it.price)}', style: TextStyle(fontSize: 11, color: context.dimText)),
        const SizedBox(height: 4),
        Container(
          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
          decoration: BoxDecoration(
            color: ready ? const Color(0x2922C55E) : growing ? const Color(0x29F59E0B) : const Color(0x2994A3B8),
            borderRadius: BorderRadius.circular(8),
          ),
          child: Text(_statusText(it.status),
              style: TextStyle(fontSize: 11, color: ready ? const Color(0xFF22C55E) : growing ? const Color(0xFFF59E0B) : context.dimText)),
        ),
        const SizedBox(height: 6),
        SizedBox(
          height: 28,
          child: FilledButton(
            onPressed: (_busy || growing)
                ? null
                : () => _doAct(ready
                    ? () => _api.harvest(widget.siteId, _game, it.section, it.id)
                    : () => _api.plant(widget.siteId, _game, it.section, it.id)),
            style: FilledButton.styleFrom(padding: const EdgeInsets.symmetric(horizontal: 12), backgroundColor: const Color(0xFF16A34A)),
            child: Text(ready ? '收获' : (growing ? '生长中' : (it.section == 'animal' ? '养殖' : '种植')), style: const TextStyle(fontSize: 12)),
          ),
        ),
      ]),
    );
  }

  Widget _warehouseSection() => GlassCard(
        padding: const EdgeInsets.all(14),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          const Text('📦 仓库', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
          const SizedBox(height: 8),
          ..._farm!.warehouse.map((w) => Padding(
                padding: const EdgeInsets.symmetric(vertical: 4),
                child: Row(children: [
                  Text('${w.name} ×${w.count}', style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 13)),
                  if (w.marketPrice > 0) Padding(padding: const EdgeInsets.only(left: 6), child: Text('市价 ${_fmt(w.marketPrice)}', style: TextStyle(fontSize: 11, color: context.dimText))),
                  const Spacer(),
                  SizedBox(
                    height: 26,
                    child: FilledButton(
                      onPressed: _busy ? null : () => _doAct(() => _api.sell(widget.siteId, _game, w.key)),
                      style: FilledButton.styleFrom(padding: const EdgeInsets.symmetric(horizontal: 10), backgroundColor: const Color(0xFF3B82F6)),
                      child: const Text('出售', style: TextStyle(fontSize: 11)),
                    ),
                  ),
                ]),
              )),
        ]),
      );

  Widget _marketSection() => GlassCard(
        padding: const EdgeInsets.all(14),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Row(children: [
            const Text('🛒 菜市场', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
            const SizedBox(width: 6),
            Text('每 24h 更新，±20%', style: TextStyle(fontSize: 12, color: context.dimText)),
          ]),
          const SizedBox(height: 8),
          ..._farm!.market.map((m) => Padding(
                padding: const EdgeInsets.symmetric(vertical: 3),
                child: Row(children: [
                  Text(m.name, style: const TextStyle(fontSize: 13)),
                  Padding(padding: const EdgeInsets.only(left: 6), child: Text(m.section == 'crop' ? '农作物' : '动物', style: TextStyle(fontSize: 11, color: context.dimText))),
                  const Spacer(),
                  Text(_fmt(m.marketPrice), style: const TextStyle(color: Color(0xFFF59E0B), fontWeight: FontWeight.w700, fontSize: 13)),
                ]),
              )),
        ]),
      );

  Widget _resultView() => Container(
        padding: const EdgeInsets.all(10),
        decoration: BoxDecoration(
          color: _last!.success ? PTColors.success.withValues(alpha: 0.12) : Colors.redAccent.withValues(alpha: 0.10),
          borderRadius: BorderRadius.circular(10),
        ),
        child: Row(children: [
          Text(_last!.success ? '✓' : '✗', style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 13)),
          const SizedBox(width: 8),
          Expanded(child: Text(_last!.message, style: const TextStyle(fontSize: 13))),
        ]),
      );
}
