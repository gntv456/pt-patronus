import 'package:flutter/material.dart';
import 'package:ptpatronus/features/home/home_menu_button.dart';
import 'package:ptpatronus/api/medal_wall.dart';
import 'package:ptpatronus/core/responsive.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';

class MedalWallPage extends StatefulWidget {
  const MedalWallPage({super.key});
  @override
  State<MedalWallPage> createState() => _MedalWallPageState();
}

class _MedalWallPageState extends State<MedalWallPage> {
  final _api = MedalWallApi();
  List<Medal> _medals = [];
  bool _loading = true;
  String? _error;
  String _filter = 'all'; // all/owned/available/out_of_stock/wearing
  String _siteFilter = 'all'; // 'all' 或 site_name

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
      _medals = await _api.all(refresh: refresh);
    } catch (e) {
      _error = e.toString();
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  List<MapEntry<int, String>> get _sites {
    final seen = <int, String>{};
    for (final m in _medals) {
      seen.putIfAbsent(m.siteId, () => m.siteName);
    }
    return seen.entries.toList();
  }

  List<Medal> get _siteFiltered {
    if (_siteFilter == 'all') return _medals;
    return _medals.where((m) => m.siteName == _siteFilter).toList();
  }

  List<Medal> get _filtered {
    final list = _siteFiltered;
    if (_filter == 'all') return list;
    if (_filter == 'wearing') return list.where((m) => m.wearStatus).toList();
    return list.where((m) => m.purchaseStatus.toLowerCase() == _filter).toList();
  }

  int get _ownedCount => _siteFiltered.where((m) => m.isOwned).length;
  int get _availableCount => _siteFiltered.where((m) => m.isAvailable).length;
  int get _oosCount => _siteFiltered.where((m) => m.isOutOfStock).length;
  int get _wearingCount => _siteFiltered.where((m) => m.wearStatus).length;

  Color _statusColor(String s) => switch (s) {
        'OWNED' => const Color(0xFF2ECC71),
        'AVAILABLE' => const Color(0xFF3498DB),
        'OUT_OF_STOCK' => const Color(0xFFF56C6C),
        'SYNTHESIS' => const Color(0xFF9B59B6),
        _ => const Color(0xFF95A5A6), // NOT_ON_SALE/GRANT_ONLY/DISCONTINUED/UNKNOWN
      };

  String _statusLabel(String s) => switch (s) {
        'OWNED' => '已拥有',
        'AVAILABLE' => '可购买',
        'OUT_OF_STOCK' => '缺货',
        'NOT_ON_SALE' => '未开售',
        'GRANT_ONLY' => '仅授予',
        'SYNTHESIS' => '仅合成',
        'DISCONTINUED' => '已下架',
        _ => '未知',
      };

  @override
  Widget build(BuildContext context) {
    return GlassBackground(
      child: Scaffold(
        backgroundColor: Colors.transparent,
        appBar: AppBar(
          leading: const HomeMenuButton(),
          title: const Text('勋章墙'),
          backgroundColor: Colors.transparent,
          elevation: 0,
          actions: [
            PTIconAction(icon: Icons.refresh, tooltip: '刷新', loading: _loading, onPressed: _loading ? null : () => _load(refresh: true)),
          ],
        ),
        body: RefreshIndicator(
          onRefresh: () => _load(refresh: true),
          child: _loading
              ? const Center(child: CircularProgressIndicator())
              : _error != null
                  ? PTEmptyState(text: _error!, onRetry: _load)
                  : ListView(
                      padding: const EdgeInsets.all(12),
                      children: [
                        _filterBar(),
                        const SizedBox(height: 8),
                        if (_filtered.isEmpty)
                          PTEmptyState(
                            icon: Icons.military_tech_outlined,
                            text: '暂无勋章数据',
                            description: 'NexusPHP 站自动抓取；馒头无勋章页。若有勋章却为空，可能未登录或布局待校准',
                          )
                        else
                          LayoutBuilder(
                            builder: (context, c) {
                              final cols = ptAutoColumns(c.maxWidth, 150, minColumns: 3);
                              return GridView.count(
                                crossAxisCount: cols,
                                shrinkWrap: true,
                                physics: const NeverScrollableScrollPhysics(),
                                mainAxisSpacing: 10,
                                crossAxisSpacing: 10,
                                childAspectRatio: 0.82,
                                children: _filtered.map(_gridCard).toList(),
                              );
                            },
                          ),
                      ],
                    ),
        ),
      ),
    );
  }

  Widget _filterBar() {
    String statusLabel(String k) => switch (k) {
          'all' => '全部',
          'owned' => '已拥有',
          'available' => '可购买',
          'out_of_stock' => '缺货',
          'wearing' => '佩戴中',
          _ => k,
        };
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 6, 12, 8),
      child: Wrap(
        spacing: 8,
        runSpacing: 6,
        children: [
          PTFilterPill(
            label: '站点',
            value: _siteFilter == 'all' ? '全部站点' : _siteFilter,
            active: _siteFilter != 'all',
            onTap: () => showFilterSheet<String>(
              context,
              title: '筛选站点',
              selected: _siteFilter,
              onSelected: (v) => setState(() => _siteFilter = v),
              options: [
                const PTFilterOption(value: 'all', label: '全部站点'),
                ..._sites.map((s) => PTFilterOption(value: s.value, label: s.value)),
              ],
            ),
          ),
          PTFilterPill(
            label: '状态',
            value: statusLabel(_filter),
            active: _filter != 'all',
            onTap: () => showFilterSheet<String>(
              context,
              title: '筛选状态',
              selected: _filter,
              onSelected: (v) => setState(() => _filter = v),
              options: [
                PTFilterOption(value: 'all', label: '全部 (${_siteFiltered.length})'),
                PTFilterOption(value: 'owned', label: '已拥有 ($_ownedCount)'),
                PTFilterOption(value: 'available', label: '可购买 ($_availableCount)'),
                PTFilterOption(value: 'out_of_stock', label: '缺货 ($_oosCount)'),
                PTFilterOption(value: 'wearing', label: '佩戴中 ($_wearingCount)'),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _gridCard(Medal m) {
    return GestureDetector(
      onTap: () => _openDetail(m),
      child: GlassCard(
        padding: const EdgeInsets.symmetric(vertical: 10),
        child: Column(
          children: [
            Stack(children: [
              CircleAvatar(
                radius: 26,
                backgroundColor: const Color(0xFF6C5CE7).withValues(alpha: 0.1),
                backgroundImage: m.imageSmall.isNotEmpty ? NetworkImage(m.imageSmall) : null,
                onBackgroundImageError: m.imageSmall.isNotEmpty ? (_, _) {} : null,
                child: m.imageSmall.isEmpty
                    ? Text(m.name.isNotEmpty ? m.name.substring(0, 1) : '?',
                        style: const TextStyle(color: Color(0xFF6C5CE7), fontWeight: FontWeight.bold))
                    : null,
              ),
              if (m.isOwned || m.isOutOfStock)
                Positioned(
                  right: -2,
                  bottom: -2,
                  child: Container(
                    width: 20,
                    height: 20,
                    decoration: BoxDecoration(
                      color: m.isOwned ? const Color(0xFF2ECC71) : const Color(0xFFF56C6C),
                      shape: BoxShape.circle,
                    ),
                    child: Icon(m.isOwned ? Icons.check : Icons.block, color: Colors.white, size: 12),
                  ),
                ),
            ]),
            const SizedBox(height: 6),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 4),
              child: Text(m.name.isNotEmpty ? m.name : '未命名',
                  maxLines: 1, overflow: TextOverflow.ellipsis, style: const TextStyle(fontSize: 12, fontWeight: FontWeight.w600)),
            ),
            if (m.price > 0)
              RichText(
                text: TextSpan(
                  children: [
                    if (m.originalPrice > 0 && m.originalPrice < m.price)
                      TextSpan(text: '${m.originalPrice} ', style: const TextStyle(color: Color(0xFF999999), fontSize: 11, decoration: TextDecoration.lineThrough)),
                    TextSpan(
                      text: '${m.price} ${m.currency}',
                      style: TextStyle(
                        color: (m.originalPrice > 0 && m.originalPrice < m.price) ? const Color(0xFFF56C6C) : const Color(0xFFF59E0B),
                        fontSize: 11,
                      ),
                    ),
                  ],
                ),
              ),
            if (m.bonusRate.isNotEmpty)
              Text(m.bonusRate, style: const TextStyle(color: Color(0xFF2ECC71), fontSize: 11)),
            if (m.error.isNotEmpty) Text(m.error, style: const TextStyle(color: Color(0xFFF56C6C), fontSize: 11)),
          ],
        ),
      ),
    );
  }

  String _fmtUnix(int? t) {
    if (t == null || t == 0) return '';
    final d = DateTime.fromMillisecondsSinceEpoch(t * 1000);
    return '${d.year}-${d.month.toString().padLeft(2, '0')}-${d.day.toString().padLeft(2, '0')}';
  }

  void _openDetail(Medal m) {
    showAdaptiveSheet(
      context,
      title: m.name,
      wideWidth: 480,
      builder: (_) => Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Center(
            child: CircleAvatar(
              radius: 48,
              backgroundColor: PTColors.primary.withValues(alpha: 0.1),
              backgroundImage: (m.originalImageUrl.isNotEmpty || m.imageSmall.isNotEmpty)
                  ? NetworkImage(m.originalImageUrl.isNotEmpty ? m.originalImageUrl : m.imageSmall)
                  : null,
              onBackgroundImageError: m.imageSmall.isNotEmpty ? (_, _) {} : null,
              child: (m.originalImageUrl.isEmpty && m.imageSmall.isEmpty)
                  ? Text(m.name.isNotEmpty ? m.name.substring(0, 1) : '?', style: const TextStyle(color: PTColors.primary, fontSize: 32, fontWeight: FontWeight.bold))
                  : null,
            ),
          ),
          const SizedBox(height: 10),
          Center(child: Text(m.name, style: const TextStyle(fontSize: 18, fontWeight: FontWeight.bold))),
          Center(
            child: Text(
              _statusLabel(m.purchaseStatus) + (m.wearStatus ? ' · 佩戴中' : ''),
              style: TextStyle(color: _statusColor(m.purchaseStatus), fontWeight: FontWeight.w600),
            ),
          ),
          if (m.description.isNotEmpty)
            Padding(padding: const EdgeInsets.only(top: 10), child: Text(m.description, style: const TextStyle(color: Colors.grey, height: 1.4))),
          const SizedBox(height: 12),
          if (m.price > 0)
            _row('价格', m.originalPrice > 0 && m.originalPrice < m.price ? '${m.originalPrice} → ${m.price} ${m.currency}（溢价）' : '${m.price} ${m.currency}'),
          if (m.bonusRate.isNotEmpty) _row('加成', m.bonusRate),
          if (m.validity.isNotEmpty) _row('有效期', m.validity),
          if (_fmtUnix(m.saleBeginTime).isNotEmpty) _row('开售', _fmtUnix(m.saleBeginTime)),
          if (_fmtUnix(m.saleEndTime).isNotEmpty) _row('截止', _fmtUnix(m.saleEndTime)),
          if (m.stock >= 0) _row('库存', m.stock == 0 ? '缺货' : '${m.stock}'),
          if (m.group.isNotEmpty) _row('分组', m.group),
          _row('来源站', m.siteName),
          const SizedBox(height: 16),
          Row(mainAxisAlignment: MainAxisAlignment.end, children: [
            if (!m.isOwned && m.isAvailable)
              FilledButton.icon(onPressed: () => _doAction('buy', m), icon: const Icon(Icons.shopping_cart_outlined), label: const Text('购买')),
            if (m.isOwned && !m.wearStatus)
              Padding(padding: const EdgeInsets.only(left: 8), child: FilledButton.icon(onPressed: () => _doAction('wear', m), icon: const Icon(Icons.star_border), label: const Text('佩戴'))),
            if (m.wearStatus)
              Padding(padding: const EdgeInsets.only(left: 8), child: FilledButton.icon(onPressed: () => _doAction('takeoff', m), icon: const Icon(Icons.star), label: const Text('摘下'))),
          ]),
        ],
      ),
    );
  }

  Widget _row(String label, String value) => Padding(
        padding: const EdgeInsets.only(top: 6),
        child: Row(mainAxisAlignment: MainAxisAlignment.spaceBetween, children: [
          Text(label, style: const TextStyle(color: Colors.grey)),
          Text(value),
        ]),
      );

  Future<void> _doAction(String action, Medal m) async {
    // 购买扣魔力不可逆 → 二次确认；佩戴/摘下可逆不确认。
    if (action == 'buy') {
      final ok = await showDialog<bool>(
        context: context,
        builder: (_) => AlertDialog(
          title: const Text('购买勋章'),
          content: Text('将消耗 ${m.price} ${m.currency} 购买「${m.name}」，确认？'),
          actions: [
            TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('取消')),
            FilledButton(onPressed: () => Navigator.pop(context, true), child: const Text('购买')),
          ],
        ),
      );
      if (ok != true) return;
    }
    try {
      ActionResult r;
      if (action == 'buy') {
        r = await _api.buy(m.siteId, m.medalId);
      } else if (action == 'wear') {
        r = await _api.wear(m.siteId, m.medalId);
      } else {
        r = await _api.takeOff(m.siteId, m.medalId);
      }
      if (!mounted) return;
      if (r.success) {
        Navigator.of(context).pop();
        _load();
      }
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(r.message.isEmpty ? '完成' : r.message)));
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('$e')));
    }
  }
}
