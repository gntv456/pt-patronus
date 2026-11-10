// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter/material.dart';

/// 屏幕宽度分档 —— 对齐 Web 的响应式语义，替代散落各页的裸 `MediaQuery` 判断。
///
/// - compact  <600      手机竖屏（卡片 / 底栏）
/// - medium   600–999   大手机横屏 / 小平板（卡片 / 底栏）
/// - expanded 1000–1439 桌面 / 大平板横屏（常驻侧栏生效；启用 Web 范式：表格 / 对话框 / 抽屉 / 多列网格）
/// - large    ≥1440     宽屏桌面（网格列数最密）
///
/// 1000 与 [kHomeWideBreakpoint]（home_controller.dart）对齐：侧栏常驻 ⇔ 宽屏 Web 范式同一切换点。
enum PTScreenTier { compact, medium, expanded, large }

const double kPtCompactMax = 600;
const double kPtMediumMax = 1000;
const double kPtExpandedMax = 1440;

/// 当前窗口所属的宽度档。随窗口缩放自动重算（MediaQuery 重建）。
PTScreenTier ptScreenTierOf(BuildContext context) {
  final w = MediaQuery.widthOf(context);
  if (w >= kPtExpandedMax) return PTScreenTier.large;
  if (w >= kPtMediumMax) return PTScreenTier.expanded;
  if (w >= kPtCompactMax) return PTScreenTier.medium;
  return PTScreenTier.compact;
}

/// 是否宽屏（expanded / large）—— 启用 Web 范式：数据表格、居中对话框、侧抽屉、多列网格。
/// 这是「桌面端像 Web」的单一开关，所有自适应组件 / 页面都读它。
bool isWideScreen(BuildContext context) {
  final t = ptScreenTierOf(context);
  return t == PTScreenTier.expanded || t == PTScreenTier.large;
}

/// 按可用宽度与「最小列宽」算自动填充列数（等价 CSS `repeat(auto-fit, minmax(N, 1fr))`）。
/// 用 floor：保证每列至少 [minItemWidth] 宽，与 Web 的 minmax 语义一致
/// （区别于 Flutter `maxCrossAxisExtent` 用 ceil、列更密但不保底宽）。
int ptAutoColumns(double availableWidth, double minItemWidth, {int minColumns = 1}) {
  if (availableWidth <= 0 || minItemWidth <= 0) return minColumns;
  final n = availableWidth ~/ minItemWidth;
  return n < minColumns ? minColumns : n;
}
