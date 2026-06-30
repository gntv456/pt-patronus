import 'package:flutter/material.dart';
import 'package:ptpatronus/api/medal_wall.dart';
import 'package:ptpatronus/theme/glass.dart';

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
          title: const Text('勋章墙'),
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
                        if (_filtered.isEmpty)
                          const Padding(
                            padding: EdgeInsets.all(40),
                            child: Center(
                              child: Text(
                                '暂无勋章数据\nNexusPHP 站自动抓取；馒头无勋章页。\n若有勋章却为空，可能未登录或布局待校准',
                                textAlign: TextAlign.center,
                                style: TextStyle(color: Colors.grey, height: 1.6),
                              ),
                            ),
                          )
                        else
                          GridView.count(
                            crossAxisCount: 3,
                            shrinkWrap: true,
                            physics: const NeverScrollableScrollPhysics(),
                            mainAxisSpacing: 10,
                            crossAxisSpacing: 10,
                            childAspectRatio: 0.82,
                            children: _filtered.map(_gridCard).toList(),
                          ),
                      ],
                    ),
        ),
      ),
    );
  }

  Widget _filterBar() {
    final chips = [
      ('all', '全部 (${_siteFiltered.length})', const Color(0xFF6C5CE7)),
      ('owned', '已拥有 ($_ownedCount)', const Color(0xFF2ECC71)),
      ('available', '可购买 ($_availableCount)', const Color(0xFF3498DB)),
      ('out_of_stock', '缺货 ($_oosCount)', const Color(0xFFF56C6C)),
      ('wearing', '佩戴中 ($_wearingCount)', const Color(0xFFF39C12)),
    ];
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        // 站点筛选（横向 chips）
        SizedBox(
          height: 34,
          child: ListView(
            scrollDirection: Axis.horizontal,
            children: [
              _siteChip('all', '全部站点'),
              ..._sites.map((s) => _siteChip(s.value, s.value)),
            ],
          ),
        ),
        const SizedBox(height: 8),
        // 状态筛选
        SizedBox(
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
        ),
      ],
    );
  }

  Widget _siteChip(String value, String label) {
    const c = Color(0xFF6C5CE7);
    return GestureDetector(
      onTap: () => setState(() => _siteFilter = value),
      child: Container(
        margin: const EdgeInsets.only(right: 8),
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 7),
        decoration: BoxDecoration(
          color: _siteFilter == value ? c.withValues(alpha: 0.16) : Colors.transparent,
          border: Border.all(color: _siteFilter == value ? c.withValues(alpha: 0.4) : Colors.transparent),
          borderRadius: BorderRadius.circular(999),
        ),
        child: Text(label, style: TextStyle(color: _siteFilter == value ? c : null, fontSize: 12)),
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
            if (m.error.isNotEmpty) Text(m.error, style: const TextStyle(color: Color(0xFFF56C6C), fontSize: 10)),
          ],
        ),
      ),
    );
  }

  void _openDetail(Medal m) {
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      builder: (sheetCtx) => SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Center(
                child: CircleAvatar(
                  radius: 48,
                  backgroundColor: const Color(0xFF6C5CE7).withValues(alpha: 0.1),
                  backgroundImage: (m.originalImageUrl.isNotEmpty || m.imageSmall.isNotEmpty)
                      ? NetworkImage(m.originalImageUrl.isNotEmpty ? m.originalImageUrl : m.imageSmall)
                      : null,
                  onBackgroundImageError: m.imageSmall.isNotEmpty ? (_, _) {} : null,
                  child: (m.originalImageUrl.isEmpty && m.imageSmall.isEmpty)
                      ? Text(m.name.isNotEmpty ? m.name.substring(0, 1) : '?',
                          style: const TextStyle(color: Color(0xFF6C5CE7), fontSize: 32, fontWeight: FontWeight.bold))
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
                _row(
                  '价格',
                  m.originalPrice > 0 && m.originalPrice < m.price
                      ? '${m.originalPrice} → ${m.price} ${m.currency}（溢价）'
                      : '${m.price} ${m.currency}',
                ),
              if (m.bonusRate.isNotEmpty) _row('加成', m.bonusRate),
              if (m.validity.isNotEmpty) _row('有效期', m.validity),
              if (m.stock >= 0) _row('库存', m.stock == 0 ? '缺货' : '${m.stock}'),  // 注意
              if (m.group.isNotEmpty) _row('分组', m.group),
              _row('来源站', m.siteName),
              const SizedBox(height: 16),
              Row(mainAxisAlignment: MainAxisAlignment.end, children: [
                if (!m.isOwned && m.isAvailable)
                  FilledButton.icon(
                    onPressed: () => _doAction('buy', m),
                    icon: const Icon(Icons.shopping_cart_outlined),
                    label: const Text('购买'),
                  ),
                if (m.isOwned && !m.wearStatus)
                  Padding(
                    padding: const EdgeInsets.only(left: 8),
                    child: FilledButton.icon(
                      onPressed: () => _doAction('wear', m),
                      icon: const Icon(Icons.star_border),
                      label: const Text('佩戴'),
                    ),
                  ),
                if (m.wearStatus)
                  Padding(
                    padding: const EdgeInsets.only(left: 8),
                    child: FilledButton.icon(
                      onPressed: () => _doAction('takeoff', m),
                      icon: const Icon(Icons.star),
                      label: const Text('摘下'),
                    ),
                  ),
              ]),
            ],
          ),
        ),
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
