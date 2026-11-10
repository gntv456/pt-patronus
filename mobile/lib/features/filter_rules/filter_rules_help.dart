// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 从 filter_rules_page.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/features/filter_rules/filter_rules_page.dart';

/// 小节标题。
class _SectionLabel extends StatelessWidget {
  final String text;
  const _SectionLabel(this.text);
  @override
  Widget build(BuildContext context) =>
      Padding(padding: const EdgeInsets.fromLTRB(2, 6, 0, 6), child: Text(text, style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w700, color: PTColors.primary)));
}

/// rule_string 语法帮助卡。
class _HelpCard extends StatelessWidget {
  const _HelpCard();
  @override
  Widget build(BuildContext context) {
    return GlassCard(
      padding: const EdgeInsets.all(12),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Row(children: [
          const Icon(Icons.lightbulb_outline, size: 16, color: PTColors.warn),
          const SizedBox(width: 6),
          const Text('rule_string 语法', style: TextStyle(fontSize: 12, fontWeight: FontWeight.w700)),
        ]),
        const SizedBox(height: 6),
        Text(
          '> 分优先级层（左=最高优先），每层用 ! & | () 连接规则 id；'
          '种子按 first-match-wins 赋 pri_order，未命中任何层即被剔除。'
          '内置规则如 4K / 1080P / BLU / H265 等可直接引用。',
          style: TextStyle(fontSize: 11, color: Theme.of(context).hintColor, height: 1.5),
        ),
        const SizedBox(height: 6),
        Container(
          width: double.infinity,
          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
          decoration: BoxDecoration(color: PTColors.primary.withValues(alpha: 0.10), borderRadius: BorderRadius.circular(6)),
          child: const Text('例：4K & !BLU > 1080P', style: TextStyle(fontSize: 11, fontFamily: 'monospace', color: PTColors.primary)),
        ),
      ]),
    );
  }
}
