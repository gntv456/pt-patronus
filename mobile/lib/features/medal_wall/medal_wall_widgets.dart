// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// MedalWallPage 的展示层扩展（无 setState 段；从 medal_wall_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/medal_wall/medal_wall_page.dart';

extension on _MedalWallPageState {
  Widget _gridCard(Medal m) {
    return GestureDetector(
      onTap: () => _openDetail(m),
      child: GlassCard(
        padding: const EdgeInsets.symmetric(vertical: 10),
        child: Column(
          children: [
            Stack(
              children: [
                CircleAvatar(
                  radius: 26,
                  backgroundColor: const Color(
                    0xFF6C5CE7,
                  ).withValues(alpha: 0.1),
                  backgroundImage: m.imageSmall.isNotEmpty
                      ? NetworkImage(m.imageSmall)
                      : null,
                  onBackgroundImageError: m.imageSmall.isNotEmpty
                      ? (_, _) {}
                      : null,
                  child: m.imageSmall.isEmpty
                      ? Text(
                          m.name.isNotEmpty ? m.name.substring(0, 1) : '?',
                          style: const TextStyle(
                            color: Color(0xFF6C5CE7),
                            fontWeight: FontWeight.bold,
                          ),
                        )
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
                        color: m.isOwned
                            ? const Color(0xFF2ECC71)
                            : const Color(0xFFF56C6C),
                        shape: BoxShape.circle,
                      ),
                      child: Icon(
                        m.isOwned ? Icons.check : Icons.block,
                        color: Colors.white,
                        size: 12,
                      ),
                    ),
                  ),
              ],
            ),
            const SizedBox(height: 6),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 4),
              child: Text(
                m.name.isNotEmpty ? m.name : '未命名',
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: const TextStyle(
                  fontSize: 12,
                  fontWeight: FontWeight.w600,
                ),
              ),
            ),
            if (m.price > 0)
              RichText(
                text: TextSpan(
                  children: [
                    if (m.originalPrice > 0 && m.originalPrice < m.price)
                      TextSpan(
                        text: '${m.originalPrice} ',
                        style: const TextStyle(
                          color: Color(0xFF999999),
                          fontSize: 11,
                          decoration: TextDecoration.lineThrough,
                        ),
                      ),
                    TextSpan(
                      text: '${m.price} ${m.currency}',
                      style: TextStyle(
                        color:
                            (m.originalPrice > 0 && m.originalPrice < m.price)
                            ? const Color(0xFFF56C6C)
                            : const Color(0xFFF59E0B),
                        fontSize: 11,
                      ),
                    ),
                  ],
                ),
              ),
            if (m.bonusRate.isNotEmpty)
              Text(
                m.bonusRate,
                style: const TextStyle(color: Color(0xFF2ECC71), fontSize: 11),
              ),
            if (m.error.isNotEmpty)
              Text(
                m.error,
                style: const TextStyle(color: Color(0xFFF56C6C), fontSize: 11),
              ),
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
              backgroundImage:
                  (m.originalImageUrl.isNotEmpty || m.imageSmall.isNotEmpty)
                  ? NetworkImage(
                      m.originalImageUrl.isNotEmpty
                          ? m.originalImageUrl
                          : m.imageSmall,
                    )
                  : null,
              onBackgroundImageError: m.imageSmall.isNotEmpty
                  ? (_, _) {}
                  : null,
              child: (m.originalImageUrl.isEmpty && m.imageSmall.isEmpty)
                  ? Text(
                      m.name.isNotEmpty ? m.name.substring(0, 1) : '?',
                      style: TextStyle(
                        color: PTColors.primary,
                        fontSize: 32,
                        fontWeight: FontWeight.bold,
                      ),
                    )
                  : null,
            ),
          ),
          const SizedBox(height: 10),
          Center(
            child: Text(
              m.name,
              style: const TextStyle(fontSize: 18, fontWeight: FontWeight.bold),
            ),
          ),
          Center(
            child: Text(
              _statusLabel(m.purchaseStatus) + (m.wearStatus ? ' · 佩戴中' : ''),
              style: TextStyle(
                color: _statusColor(m.purchaseStatus),
                fontWeight: FontWeight.w600,
              ),
            ),
          ),
          if (m.description.isNotEmpty)
            Padding(
              padding: const EdgeInsets.only(top: 10),
              child: Text(
                m.description,
                style: TextStyle(color: context.dimText, height: 1.4),
              ),
            ),
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
          if (_fmtUnix(m.saleBeginTime).isNotEmpty)
            _row('开售', _fmtUnix(m.saleBeginTime)),
          if (_fmtUnix(m.saleEndTime).isNotEmpty)
            _row('截止', _fmtUnix(m.saleEndTime)),
          if (m.stock >= 0) _row('库存', m.stock == 0 ? '缺货' : '${m.stock}'),
          if (m.group.isNotEmpty) _row('分组', m.group),
          _row('来源站', m.siteName),
          const SizedBox(height: 16),
          Row(
            mainAxisAlignment: MainAxisAlignment.end,
            children: [
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
            ],
          ),
        ],
      ),
    );
  }

  Widget _row(String label, String value) => Padding(
    padding: const EdgeInsets.only(top: 6),
    child: Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: [
        Text(label, style: TextStyle(color: context.dimText)),
        Text(value),
      ],
    ),
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
            TextButton(
              onPressed: () => Navigator.pop(context, false),
              child: const Text('取消'),
            ),
            FilledButton(
              onPressed: () => Navigator.pop(context, true),
              child: const Text('购买'),
            ),
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
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(r.message.isEmpty ? '完成' : r.message)),
        );
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text('$e')));
      }
    }
  }
}
