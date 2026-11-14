// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 从 savept_page.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/features/savept/savept_page.dart';

/// 单站点卡片：今天站庆时叠加 confetti 彩带；死亡站灰度图标 + 状态色卡底。
class _SiteCard extends StatefulWidget {
  final SaveptSiteCard card;
  final String Function(String) siteTypeLabel;
  final String Function(SiteHealthStatus) statusLabel;
  final String Function(MatchType) matchLabel;
  final Color Function(SiteHealthStatus) healthColor;
  final bool Function(String) isTodayAnniv;
  final String Function(String) annivDisplay;
  const _SiteCard(
    this.card,
    this.siteTypeLabel,
    this.statusLabel,
    this.matchLabel,
    this.healthColor,
    this.isTodayAnniv,
    this.annivDisplay,
  );

  @override
  State<_SiteCard> createState() => _SiteCardState();
}

class _SiteCardState extends State<_SiteCard> {
  late ConfettiController _confetti;
  bool get _isToday => widget.isTodayAnniv(widget.card.anniversaryText);

  @override
  void initState() {
    super.initState();
    _confetti = ConfettiController(duration: const Duration(seconds: 4));
    if (_isToday) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (mounted) _confetti.play();
      });
    }
  }

  @override
  void dispose() {
    _confetti.dispose();
    super.dispose();
  }

  Future<void> _launchUrl(String url) async {
    try {
      await launchUrl(Uri.parse(url), mode: LaunchMode.externalApplication);
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text('打开站点失败：$e')));
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final c = widget.card;
    final color = widget.healthColor(c.status);
    final matchColor = switch (c.matchType) {
      MatchType.urlExact || MatchType.domainExact => const Color(0xFF16A34A),
      MatchType.nameExact || MatchType.nameFuzzy => const Color(0xFF2563EB),
      MatchType.none => const Color(0xFF64748B),
    };
    final iconWidget = Image.network(
      c.icon!,
      width: 34,
      height: 34,
      fit: BoxFit.contain,
      errorBuilder: (_, _, _) => const SizedBox(width: 34, height: 34),
    );
    return Container(
      margin: EdgeInsets.zero,
      child: Stack(
        children: [
          GlassCard(
            onTap: c.url.isNotEmpty ? () => _launchUrl(c.url) : null,
            tint: color, // 健康状态淡色底（对齐 Web 卡片 healthy/critical/closed 着色）
            padding: const EdgeInsets.all(12),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                // 标签行：纯彩色文字 + 「·」分隔（无底色框，最省地方；一行排不下才自然换行）。
                Builder(builder: (_) {
                  final spans = <TextSpan>[];
                  void add(String text, Color color) {
                    if (spans.isNotEmpty) spans.add(const TextSpan(text: ' · '));
                    spans.add(TextSpan(
                      text: text,
                      style: TextStyle(color: color, fontSize: 11, fontWeight: FontWeight.w500),
                    ));
                  }
                  add(widget.siteTypeLabel(c.siteType), context.dimText);
                  if (c.durationText.isNotEmpty) add(c.durationText, PTColors.primary);
                  if (c.anniversaryText.isNotEmpty) {
                    add(widget.annivDisplay(c.anniversaryText), const Color(0xFFF59E0B));
                  }
                  if (c.faultDuration.isNotEmpty) {
                    add('抢救 ${c.faultDuration}', const Color(0xFFEF4444));
                  }
                  return Text.rich(
                    TextSpan(children: spans, style: const TextStyle(fontSize: 11, height: 1.3)),
                  );
                }),
                const SizedBox(height: 6),
                Row(
                  children: [
                    if (c.icon != null && c.icon!.isNotEmpty)
                      Padding(
                        padding: const EdgeInsets.only(right: 10),
                        child: ClipRRect(
                          borderRadius: BorderRadius.circular(8),
                          // 死亡站图标灰度（对齐 Web closed 卡 filter: grayscale(1)）
                          child: c.status == SiteHealthStatus.closed
                              ? ColorFiltered(
                                  colorFilter:
                                      const ColorFilter.matrix(<double>[
                                        0.2126,
                                        0.7152,
                                        0.0722,
                                        0,
                                        0,
                                        0.2126,
                                        0.7152,
                                        0.0722,
                                        0,
                                        0,
                                        0.2126,
                                        0.7152,
                                        0.0722,
                                        0,
                                        0,
                                        0,
                                        0,
                                        0,
                                        1,
                                        0,
                                      ]),
                                  child: iconWidget,
                                )
                              : iconWidget,
                        ),
                      ),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Row(
                            children: [
                              CircleAvatar(radius: 4, backgroundColor: color),
                              const SizedBox(width: 6),
                              Expanded(
                                child: Text(
                                  c.name,
                                  style: const TextStyle(
                                    fontWeight: FontWeight.w600,
                                  ),
                                  overflow: TextOverflow.ellipsis,
                                ),
                              ),
                              Icon(
                                _statusIcon(c.status),
                                color: color,
                                size: 16,
                              ),
                            ],
                          ),
                          const SizedBox(height: 4),
                          Text(
                            '开站 ${c.openTime.isNotEmpty ? c.openTime : '未知'}${c.closeTime.isNotEmpty && c.closeTime != '未知' ? ' · 关站 ${c.closeTime}' : ''}',
                            style: TextStyle(
                              fontSize: 11,
                              color: context.dimText,
                            ),
                          ),
                          if (c.localSiteName.isNotEmpty)
                            Padding(
                              padding: const EdgeInsets.only(top: 2),
                              child: Text(
                                c.localSiteName,
                                style: TextStyle(
                                  fontSize: 11,
                                  color: PTColors.primary,
                                ),
                                overflow: TextOverflow.ellipsis,
                              ),
                            ),
                        ],
                      ),
                    ),
                  ],
                ),
                const SizedBox(height: 8),
                Row(
                  children: [
                    // MP 状态徽章带前缀图标（已拥有=check / 可添加=plus / 未收录=help）
                    Container(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 8,
                        vertical: 2,
                      ),
                      decoration: BoxDecoration(
                        color: matchColor.withValues(alpha: 0.18),
                        borderRadius: BorderRadius.circular(999),
                      ),
                      child: Row(
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          Icon(
                            _mpIcon(c.matchType),
                            size: 12,
                            color: matchColor,
                          ),
                          const SizedBox(width: 3),
                          Text(
                            widget.matchLabel(c.matchType),
                            style: TextStyle(color: matchColor, fontSize: 11),
                          ),
                        ],
                      ),
                    ),
                    if (c.domain.isNotEmpty) ...[
                      const SizedBox(width: 8),
                      Expanded(
                        child: Text(
                          c.domain,
                          style: TextStyle(
                            color: context.dimText,
                            fontSize: 11,
                          ),
                          overflow: TextOverflow.ellipsis,
                        ),
                      ),
                    ],
                  ],
                ),
              ],
            ),
          ),
          if (_isToday)
            Positioned(
              top: 0,
              left: 0,
              right: 0,
              child: ConfettiWidget(
                confettiController: _confetti,
                blastDirectionality: BlastDirectionality.explosive,
                shouldLoop: true,
                colors: const [
                  Color(0xFFF59E0B),
                  Color(0xFFFACC15),
                  Color(0xFFFB7185),
                  Color(0xFF22C55E),
                  Color(0xFF38BDF8),
                ],
                maxBlastForce: 6,
                minBlastForce: 3,
                emissionFrequency: 0.4,
                numberOfParticles: 6,
                blastDirection: math.pi / 2,
                gravity: 0.25,
              ),
            ),
        ],
      ),
    );
  }

  IconData _statusIcon(SiteHealthStatus s) => switch (s) {
    SiteHealthStatus.healthy => Icons.favorite,
    SiteHealthStatus.critical => Icons.error_outline,
    SiteHealthStatus.closed => Icons.sentiment_very_dissatisfied,
    SiteHealthStatus.unknown => Icons.help_outline,
  };

  IconData _mpIcon(MatchType m) => switch (m) {
    MatchType.urlExact || MatchType.domainExact => Icons.check_circle_outline,
    MatchType.nameExact || MatchType.nameFuzzy => Icons.add_circle_outline,
    MatchType.none => Icons.help_outline,
  };
}

class _Empty extends StatelessWidget {
  final String text;
  final VoidCallback? onRetry;
  const _Empty({required this.text, this.onRetry});
  @override
  Widget build(BuildContext context) {
    return ListView(
      children: [
        const SizedBox(height: 120),
        Center(
          child: Padding(
            padding: const EdgeInsets.all(24),
            child: Text(
              text,
              textAlign: TextAlign.center,
              style: TextStyle(color: context.dimText),
            ),
          ),
        ),
        if (onRetry != null)
          Center(
            child: ElevatedButton(onPressed: onRetry, child: const Text('重试')),
          ),
      ],
    );
  }
}
