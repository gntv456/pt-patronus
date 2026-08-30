// 从 adaptive.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/widgets/adaptive.dart';

/// 很小的筛选触发器药丸。两种用法：
/// - 选择型：[onTap] 由调用方打开 `showFilterSheet`，显示 `label value ▾`。
/// - 开关型：[onTap] 直接切换（二元），[active] 控制高亮（如「近期站庆」）。
class PTFilterPill extends StatelessWidget {
  final String value; // 当前值文本（如 "正在站免"）
  final String? label; // 维度名前缀（如 "状态"），可空
  final VoidCallback onTap;
  final bool active; // 非默认值时 true → 主色高亮
  final IconData? icon;

  const PTFilterPill({
    super.key,
    required this.value,
    required this.onTap,
    this.label,
    this.active = false,
    this.icon,
  });

  @override
  Widget build(BuildContext context) {
    final c = PTColors.primary;
    final dim = active ? c : Theme.of(context).colorScheme.onSurfaceVariant;
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
        constraints: const BoxConstraints(minHeight: 36),
        decoration: BoxDecoration(
          color: active ? c.withValues(alpha: 0.14) : Colors.transparent,
          border: Border.all(
            color: active
                ? c.withValues(alpha: 0.5)
                : Theme.of(context).colorScheme.onSurfaceVariant.withValues(alpha: 0.4),
          ),
          borderRadius: BorderRadius.circular(999),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            if (icon != null) ...[
              Icon(icon, size: 15, color: dim),
              const SizedBox(width: 4),
            ],
            if (label != null) ...[
              Text(label!, style: TextStyle(fontSize: 12, color: dim)),
              const SizedBox(width: 4),
            ],
            Text(
              value,
              style: TextStyle(
                fontSize: 13,
                fontWeight: active ? FontWeight.w700 : FontWeight.w500,
                color: active ? c : Theme.of(context).colorScheme.onSurface,
              ),
            ),
            Icon(Icons.arrow_drop_down, size: 18, color: dim),
          ],
        ),
      ),
    );
  }
}
