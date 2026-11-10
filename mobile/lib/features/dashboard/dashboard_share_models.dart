// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 分享图小组件数据模型（从 dashboard_page.dart 拆出，行为不变）。
import 'package:flutter/material.dart';

class ShareMetric {
  final String label;
  final String value;
  final Color color;

  const ShareMetric(this.label, this.value, this.color);
}

class ShareRank {
  final String label;
  final String value;
  final String siteName;
  final Color color;

  const ShareRank(this.label, this.value, this.siteName, this.color);
}


// 历史数据浮层内容：宽屏表格 / 窄屏卡片（自适应，对齐 Web 用户历史数据弹窗的 el-table）。
