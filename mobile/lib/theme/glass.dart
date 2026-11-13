// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// 设计 token —— 与前端 styles/tokens.css 语义层对齐，三端唯一真源。
/// 组件内禁止裸 hex，一律引用本类常量。
class PTColors {
  /// 交互主色（浅 #2563EB / 暗 #4C8DFF）。
  static const primary = Color(0xFF2563EB);
  static const primaryDark = Color(0xFF4C8DFF);
  /// 旧双色渐变副色：收敛为主色悬停态，避免紫→蓝对撞。
  static const primary2 = Color(0xFF1D4ED8);
  static const primaryHover = Color(0xFF1D4ED8);
  static const primarySoft = Color(0xFFEFF6FF);
  static const primarySoftDark = Color(0x244C8DFF);

  /// 语义状态色（浅色底 >= 4.5:1）。
  static const success = Color(0xFF15803D);
  static const successSoft = Color(0xFFF0FDF4);
  static const danger = Color(0xFFDC2626);
  static const dangerSoft = Color(0xFFFEF2F2);
  static const warn = Color(0xFFB45309);
  static const warning = warn;
  static const warningSoft = Color(0xFFFFFBEB);

  /// 彩色文字 / 图标前景色。
  static const successFg = Color(0xFF0F766E);
  static const warnFg = Color(0xFFB45309);
  static const dangerFg = Color(0xFFB91C1C);
  static const infoFg = Color(0xFF1D4ED8);

  /// 表面与文字（浅 / 暗）。
  static const bg = Color(0xFFF8FAFC);
  static const bgDark = Color(0xFF0F1115);
  static const surface = Color(0xFFFFFFFF);
  static const surfaceDark = Color(0xFF161A21);
  static const surface2 = Color(0xFFF1F5F9);
  static const surface2Dark = Color(0xFF1C222C);
  static const border = Color(0xFFE2E8F0);
  static const borderDark = Color(0xFF262C36);
  static const borderStrong = Color(0xFFCBD5E1);
  static const borderStrongDark = Color(0xFF333B48);
  static const text = Color(0xFF1E293B);
  static const textDark = Color(0xFFE6E9EF);
  static const textSecondary = Color(0xFF475569);
  static const textSecondaryDark = Color(0xFF9AA4B2);
  static const textMuted = Color(0xFF64748B);
  static const textMutedDark = Color(0xFF8B95A5);

  /// 图表系列（配合实线/虚线/直接标注，不只靠颜色区分）。
  static const chart1 = Color(0xFF2563EB);
  static const chart2 = Color(0xFF0F766E);
  static const chart3 = Color(0xFFB45309);
  static const chart4 = Color(0xFF7C3AED);
}

/// 间距阶梯（4 倍数网格）—— 与 glass.css --sp-* 对齐。
/// 优先用这些常量，替代散落各页的裸 EdgeInsets 数值。
class PTSp {
  static const double s1 = 4;
  static const double s2 = 8;
  static const double s3 = 12;
  static const double s4 = 16;
  static const double s5 = 20;
  static const double s6 = 24;
  static const double s8 = 32;
  static const double s10 = 40;
}

/// 字号阶梯 —— 移动端正文 >= 16px（中文可读、输入不触发 iOS 缩放），
/// 密集表格元信息最低 12px；与 Web --fs-* 阶梯对齐。
class PTFs {
  static const double xs = 12;
  static const double sm = 13;
  static const double body = 16;
  static const double md = 16;
  static const double lg = 18;
  static const double xl = 20;
  static const double title = 24;
  static const double hero = 28;
}

/// 圆角阶梯 —— 8px 网格 / 克制圆角（6 / 8 / 10px），与 Web --radius-* 对齐。
class PTRadius {
  static const double xs = 6;
  static const double sm = 6;
  static const double md = 8;
  static const double lg = 8;
  static const double xl = 10;
  static const double pill = 999; // 仅状态点，不用于按钮/徽章
}

/// 应用主题（Material 3，seed 自品牌主色）。
ThemeData buildAppTheme(Brightness brightness) {
  final isDark = brightness == Brightness.dark;
  return ThemeData(
    useMaterial3: true,
    brightness: brightness,
    colorSchemeSeed: PTColors.primary,
    // 中性实色底，去掉全屏紫蓝渐变与壁纸。
    scaffoldBackgroundColor: isDark ? PTColors.bgDark : PTColors.bg,
    // 平台标准转场（iOS/macOS Cupertino、其余 Material M3），并显式短路
    // MediaQuery.disableAnimations（系统「减少动态效果」）。
    pageTransitionsTheme: const PageTransitionsTheme(
      builders: {
        TargetPlatform.android: _PTPlatformPageTransitionsBuilder(),
        TargetPlatform.iOS: _PTPlatformPageTransitionsBuilder(),
        TargetPlatform.macOS: _PTPlatformPageTransitionsBuilder(),
        TargetPlatform.windows: _PTPlatformPageTransitionsBuilder(),
        TargetPlatform.linux: _PTPlatformPageTransitionsBuilder(),
        TargetPlatform.fuchsia: _PTPlatformPageTransitionsBuilder(),
      },
    ),
    // 48dp 工具栏：满足 44pt/48dp 触摸下限，actions 的默认 IconButton 不再溢出。
    appBarTheme: AppBarTheme(
      toolbarHeight: 48,
      elevation: 0,
      scrolledUnderElevation: 0,
      backgroundColor: Colors.transparent,
      iconTheme: const IconThemeData(size: 20),
      actionsIconTheme: const IconThemeData(size: 20),
      titleTextStyle: TextStyle(
        fontSize: 17,
        fontWeight: FontWeight.w700,
        color: isDark ? Colors.white : Colors.black87,
      ),
    ),
    snackBarTheme: const SnackBarThemeData(
      behavior: SnackBarBehavior.floating,
      insetPadding: EdgeInsets.fromLTRB(16, 0, 16, 88),
      showCloseIcon: true,
    ),
  );
}

/// 背景：纯色（浅 #F8FAFC / 暗 #0F1115），替代旧的全屏紫蓝渐变。
class GlassBackground extends StatelessWidget {
  final Widget child;
  const GlassBackground({super.key, required this.child});

  @override
  Widget build(BuildContext context) {
    final isDark = Theme.of(context).brightness == Brightness.dark;
    return ColoredBox(
      color: isDark ? PTColors.bgDark : PTColors.bg,
      child: child,
    );
  }
}

// 全局降级开关：默认 true —— 所有卡片都是实色卡（无 BackdropFilter），省 GPU。
// 保留旧开关与持久化入口作为性能模式，用户显式设置过则以用户值为准。
final glassReduced = ValueNotifier<bool>(true);

const _glassPrefsKey = 'glass_reduced';

/// 启动调用：用户设置过则覆盖默认（含 Web 用户手动关闭、原生用户手动开启两种情况）。
Future<void> loadGlassPrefs() async {
  final prefs = await SharedPreferences.getInstance();
  if (prefs.containsKey(_glassPrefsKey)) {
    glassReduced.value = prefs.getBool(_glassPrefsKey) ?? glassReduced.value;
  } else if (kIsWeb) {
    // 历史 Web 安装无 pref 时也拿到同一默认值（旧版仅 Web 默认降级）。
    glassReduced.value = true;
  }
}

/// 切换并持久化；[glassReduced] 通知所有 GlassCard 自动重建。
Future<void> setGlassReduced(bool v) async {
  glassReduced.value = v;
  final prefs = await SharedPreferences.getInstance();
  await prefs.setBool(_glassPrefsKey, v);
}

// —— 主题模式（三态：跟随系统 / 浅色 / 深色）——
// 与 glassReduced 同范式：ValueNotifier 持久化到 SharedPreferences，main() 启动加载，
// MaterialApp 用 ValueListenableBuilder 监听重建，账户页 SegmentedButton 切换。
final themeModeNotifier = ValueNotifier<ThemeMode>(ThemeMode.system);

const _themeModePrefsKey = 'theme_mode';

ThemeMode _themeModeFromString(String? s) {
  switch (s) {
    case 'light':
      return ThemeMode.light;
    case 'dark':
      return ThemeMode.dark;
    default:
      return ThemeMode.system;
  }
}

Future<void> loadThemeModePrefs() async {
  final prefs = await SharedPreferences.getInstance();
  themeModeNotifier.value = _themeModeFromString(
    prefs.getString(_themeModePrefsKey),
  );
}

Future<void> setThemeMode(ThemeMode m) async {
  themeModeNotifier.value = m;
  final prefs = await SharedPreferences.getInstance();
  final s = m == ThemeMode.light
      ? 'light'
      : (m == ThemeMode.dark ? 'dark' : 'system');
  await prefs.setString(_themeModePrefsKey, s);
}

// —— 以下纯函数与渲染解耦，便于单测 ——

/// 降级时强制 sigma=0（触发跳过 BackdropFilter）。
double effectiveGlassSigma(double requested, bool reduced) =>
    reduced ? 0.0 : requested;

/// 卡片底色不透明度。降级(无模糊)时抬高，保证文字可读：亮 0.55→0.80 / 暗 0.06→0.14。
double glassCardAlpha(bool isDark, bool reduced) =>
    isDark ? (reduced ? 0.14 : 0.06) : (reduced ? 0.80 : 0.55);

/// 实色卡片：中性表面 + 1px 边框 + 8px 圆角 + 极轻投影，无模糊/渐变/内高光。
///
/// 兼容旧调用：保留 `blurSigma` / `radius` / `padding` / `tint` 参数名，但默认不再模糊
/// （`blurSigma = 0` 且不再创建 BackdropFilter）。可点击时只保留标准 InkWell 高亮，
/// 不再做「按进液态玻璃」的缩放位移。
class GlassCard extends StatelessWidget {
  final Widget child;
  final EdgeInsets padding;
  final double blurSigma;
  final double radius;
  final VoidCallback? onTap;
  final Color? tint; // 可选淡色底（如监护室按健康状态着色）：与 cardColor 做 alphaBlend，0.12 不透明。

  const GlassCard({
    super.key,
    required this.child,
    this.padding = const EdgeInsets.all(16),
    this.blurSigma = 0,
    this.radius = 8,
    this.onTap,
    this.tint,
  });

  @override
  Widget build(BuildContext context) {
    final isDark = Theme.of(context).brightness == Brightness.dark;
    var cardColor = isDark ? PTColors.surfaceDark : PTColors.surface;
    if (tint != null) {
      cardColor = Color.alphaBlend(
        tint!.withValues(alpha: 0.12),
        cardColor,
      );
    }
    final borderColor = isDark ? PTColors.borderDark : PTColors.border;

    final inner = Container(
      decoration: BoxDecoration(
        color: cardColor,
        borderRadius: BorderRadius.circular(radius),
        border: Border.all(color: borderColor),
        boxShadow: [
          BoxShadow(
            color: Colors.black.withValues(alpha: isDark ? 0.30 : 0.04),
            blurRadius: 2,
            offset: const Offset(0, 1),
          ),
        ],
      ),
      padding: padding,
      child: child,
    );

    // 不再创建 BackdropFilter：实色卡，无 backdrop 采集、无抖动。
    final content = ClipRRect(
      borderRadius: BorderRadius.circular(radius),
      child: inner,
    );

    if (onTap == null) return content;
    // 标准 InkWell 高亮，不做缩放/位移，避免布局跳动。
    return Material(
      color: Colors.transparent,
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(radius),
        child: content,
      ),
    );
  }
}


/// 平台标准页面转场：iOS/macOS 用 Cupertino 侧滑，其余用 Material M3 默认。
/// 显式读取 MediaQuery.disableAnimations（系统「减少动态效果」）并短路为静态切换。
class _PTPlatformPageTransitionsBuilder extends PageTransitionsBuilder {
  const _PTPlatformPageTransitionsBuilder();

  @override
  Widget buildTransitions<T>(
    PageRoute<T> route,
    BuildContext context,
    Animation<double> animation,
    Animation<double> secondaryAnimation,
    Widget child,
  ) {
    if (MediaQuery.disableAnimationsOf(context)) return child;
    final platform = Theme.of(context).platform;
    final PageTransitionsBuilder standard =
        (platform == TargetPlatform.iOS || platform == TargetPlatform.macOS)
            ? const CupertinoPageTransitionsBuilder()
            : const ZoomPageTransitionsBuilder();
    return standard.buildTransitions<T>(
      route,
      context,
      animation,
      secondaryAnimation,
      child,
    );
  }
}
/// Context extension — one-line theme-aware color accessors.
/// Replaces scattered hardcoded Colors.grey / Colors.black87 across pages.
extension PTContextColors on BuildContext {
  /// Secondary/muted text color (replaces Colors.grey).
  Color get dimText => Theme.of(this).colorScheme.onSurfaceVariant;
  /// Primary text color (replaces Colors.black87).
  Color get mainText => Theme.of(this).colorScheme.onSurface;
  /// Faint decorative color for empty-state icons etc.
  Color get faint => Theme.of(this).colorScheme.onSurfaceVariant.withValues(alpha: 0.4);
}
