// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter/material.dart';
import 'package:ptpatronus/core/responsive.dart';
import 'package:ptpatronus/features/home/home_controller.dart';
import 'package:ptpatronus/features/home/home_menu_button.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_filter_sheet.dart';

part 'pt_page_app_bar.dart';

// ─────────────────────────────────────────────────────────────────────────
// 统一的每页顶栏 + 页壳。收敛全站 23 个导航页两套头部范式（手写 Row / 真 AppBar）
// 为同一根紧凑顶栏：leading 菜单（窄屏）/ 标题 / [站点按钮] / [筛选按钮] / [搜索按钮]
// / 操作图标按钮（>2 自动进 ⋮ 溢出）。
//
// 设计要点（见 plans/piped-booping-marshmallow.md Path B）：
//  - 不走 Scaffold.appBar：PTPageAppBar 放进 body 顶部 Column（AppBar + 可选搜索行），
//    这样搜索能从顶栏「向下弹出」一行（AnimatedSize 滑出），而非挤进标题位。
//  - wide 屏侧栏常驻，leading 菜单按钮自动隐藏（不再冗余）。
//  - 操作 narrow=PTIconAction(44×44) / wide=tonal 按钮；overflow=true 或 narrow>2 进 ⋮。
//  - 搜索：点顶栏搜索图标 → 顶栏下方弹出一行 TextField（✕ 收起），宽窄屏统一。
//    _searchOpen 是本 StatefulWidget 自持，仅 tap/✕ 触发，不 in-build。
//  - ⋮ 一律 44×44（修 sites/logs 现有 30×30 的 HIG 触摸违规）。
// ─────────────────────────────────────────────────────────────────────────

const int _kMaxInlineActionsNarrow = 2;

/// 顶栏一个操作。narrow→PTIconAction(44×44)；wide→tonal 文字按钮。
/// `overflow:true` 恒进尾部 ⋮ 菜单；narrow 下 inline 超过 [_kMaxInlineActionsNarrow]
/// 时尾部自动溢出进 ⋮。取代 PTHeaderAction 与各页散落 PopupMenuButton 项。
class PTBarAction {
  final IconData icon;
  final String label;
  final VoidCallback? onPressed;
  final bool loading; // narrow→PTIconAction(loading:) 显 spinner
  final bool overflow; // true→恒进 ⋮
  const PTBarAction({
    required this.icon,
    required this.label,
    this.onPressed,
    this.loading = false,
    this.overflow = false,
  });
}

/// 顶栏向下弹出搜索配置。点顶栏搜索图标 → 顶栏下方弹出一行 TextField（✕ 收起）。
/// 既用于「搜索是辅助筛选」的页（SavePT 搜站点名，onChanged 即时过滤），
/// 也用于搜索主页（Search 页，onSubmitted 回车搜索、删 body 搜索框避免重复）。
class PTSearchConfig {
  final TextEditingController controller;
  final ValueChanged<String> onChanged; // 即时过滤（SavePT onChanged）
  final ValueChanged<String>? onSubmitted; // 可选：回车搜索
  final String hintText;
  final bool autofocusOnOpen; // 弹出时自动聚焦
  const PTSearchConfig({
    required this.controller,
    required this.onChanged,
    this.onSubmitted,
    this.hintText = '搜索…',
    this.autofocusOnOpen = true,
  });
}

/// 页壳：糊样板——透明 Scaffold + body 顶部 Column[PTPageAppBar, Expanded(body)]
/// + 页 FAB。收敛 A 营手写 Row 头 + B 营真 AppBar 两套范式为同一根顶栏。
/// PTPageAppBar 放 body 顶部（非 appBar 槽），以支持搜索「向下弹出」。
///
/// [filterOnClearAll] 透传给筛选弹窗的「清除全部」；不传则 PTPageAppBar 用维度兜底重置。
class PTPageScaffold extends StatelessWidget {
  final Object title;
  final Widget? leading;
  final List<PTBarAction> actions;
  final List<PTFilterDimension> filterDimensions;
  final PTSearchConfig? search;
  final List<Widget>? trailing;
  final Widget body;
  final Widget? floatingActionButton;
  final FloatingActionButtonLocation? floatingActionButtonLocation;
  final VoidCallback? filterOnClearAll;
  final PTSiteFilterConfig? siteFilter;
  final double toolbarHeight;

  const PTPageScaffold({
    super.key,
    required this.title,
    required this.body,
    this.actions = const [],
    this.leading,
    this.filterDimensions = const [],
    this.search,
    this.trailing,
    this.floatingActionButton,
    this.floatingActionButtonLocation,
    this.filterOnClearAll,
    this.siteFilter,
    this.toolbarHeight = 48,
  });

  @override
  Widget build(BuildContext context) {
    return GlassBackground(
      child: Scaffold(
        backgroundColor: Colors.transparent,
        body: Column(
          children: [
            PTPageAppBar(
              title: title,
              leading: leading,
              actions: actions,
              filterDimensions: filterDimensions,
              filterOnClearAll: filterOnClearAll,
              siteFilter: siteFilter,
              search: search,
              trailing: trailing,
              toolbarHeight: toolbarHeight,
            ),
            Expanded(child: body),
          ],
        ),
        floatingActionButton: floatingActionButton,
        floatingActionButtonLocation: floatingActionButtonLocation,
      ),
    );
  }
}
