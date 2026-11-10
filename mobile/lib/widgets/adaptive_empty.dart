// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 从 adaptive.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/widgets/adaptive.dart';

/// 统一空状态：图标 + 标题 + 说明 + 可选动作。
///
/// 取代各页散落的裸 `Center(Text(...))`（约 12 页），对齐 Web 的 `EmptyState` 组件。
/// 渲染为可滚动 ListView，保证作为 `RefreshIndicator` 子件时空列表仍可下拉刷新。
class PTEmptyState extends StatelessWidget {
  final IconData icon;
  final String? title;
  final String? text; // 兼容仅一行文案的旧用法
  final String? description;
  final String? actionLabel;
  final VoidCallback? onAction;
  final VoidCallback? onRetry;
  final IconData actionIcon; // actionLabel 按钮图标（默认 add；原硬编码 refresh 语义错——审计 P2）

  const PTEmptyState({
    super.key,
    this.icon = Icons.inbox_outlined,
    this.title,
    this.text,
    this.description,
    this.actionLabel,
    this.onAction,
    this.onRetry,
    this.actionIcon = Icons.add,
  });

  @override
  Widget build(BuildContext context) {
    final isDark = Theme.of(context).brightness == Brightness.dark;
    final main = text ?? title ?? '暂无数据';
    return ListView(
      children: [
        const SizedBox(height: 96),
        Center(
          child: PTEntrance(
            child: Padding(
              padding: const EdgeInsets.all(24),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  PTBreathIcon(
                    icon: icon,
                    size: 56,
                    color: Theme.of(context).colorScheme.onSurfaceVariant.withValues(alpha: 0.4),
                  ),
                const SizedBox(height: 14),
                Text(
                  main,
                  textAlign: TextAlign.center,
                  style: TextStyle(
                    fontSize: PTFs.lg,
                    fontWeight: FontWeight.w600,
                    color: isDark ? Colors.white70 : context.dimText,
                  ),
                ),
                if (description != null && description!.isNotEmpty) ...[
                  const SizedBox(height: 6),
                  Text(
                    description!,
                    textAlign: TextAlign.center,
                    style: TextStyle(
                      fontSize: PTFs.body,
                      color: isDark ? Colors.white38 : Colors.black38,
                    ),
                  ),
                ],
                if (actionLabel != null && onAction != null) ...[
                  const SizedBox(height: 18),
                  FilledButton.icon(
                    onPressed: onAction,
                    icon: Icon(actionIcon),
                    label: Text(actionLabel!),
                  ),
                ] else if (onRetry != null) ...[
                  const SizedBox(height: 18),
                  OutlinedButton(onPressed: onRetry, child: const Text('重试')),
                ],
              ],
            ),
          ),
          ),
        ),
      ],
    );
  }
}
