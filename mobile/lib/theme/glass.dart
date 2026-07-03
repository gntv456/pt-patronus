import 'dart:ui';
import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// 设计 token —— 与前端 styles/glass.css 对齐，保持双端视觉统一。
class PTColors {
  static const primary = Color(0xFF6C5CE7);
  static const primary2 = Color(0xFF4285F4);
  static const success = Color(0xFF2ECC71);
  static const danger = Color(0xFFE74C3C);
  static const warn = Color(0xFFF39C12);
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

/// 字号阶梯 —— 与 glass.css --fs-* 对齐。
class PTFs {
  // xs 由 11 提到 12：11pt 在手机上低于可读阈值（WCAG），走 token 的最小字号统一兜底到 12。
  // 装饰性纯图形可用裸值更小，但可读文字最低 12。与 glass.css --fs-xs 同步。
  static const double xs = 12;
  static const double sm = 12;
  static const double body = 13;
  static const double md = 14;
  static const double lg = 15;
  static const double xl = 18;
  static const double title = 22;
  static const double hero = 28;
}

/// 圆角阶梯 —— 与 glass.css --radius-* 对齐。
class PTRadius {
  static const double xs = 6;
  static const double sm = 10;
  static const double md = 14;
  static const double lg = 20;
  static const double xl = 28;
  static const double pill = 999;
}

/// 应用主题（Material 3，seed 自品牌主色）。
ThemeData buildAppTheme(Brightness brightness) {
  final isDark = brightness == Brightness.dark;
  return ThemeData(
    useMaterial3: true,
    brightness: brightness,
    colorSchemeSeed: PTColors.primary,
    scaffoldBackgroundColor: Colors.transparent,
    // AppBar 回退到可用红线：toolbarHeight 44（Material/Apple HIG 最小触摸高度）+ 图标 20 + 标题 17。
    // 此前压到 30 导致触摸目标 30×30 低于 44 红线（紧凑度治理过头 → 误触/难点中）。
    // 44 仍比默认 56 紧凑，但保证按钮命中区。注意：toolbarHeight<48 时 actions 里的裸
    // IconButton(默认 48) 仍会溢出，故 actions 统一用 PTIconAction(44×44 触摸区)。
    // 页面级 toolbarHeight 仍可按需覆盖。
    appBarTheme: AppBarTheme(
      toolbarHeight: 44,
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
  );
}

/// 液态玻璃（Liquid Glass）背景：浅蓝→浅紫（明）/ 深灰→深紫（暗）渐变。
class GlassBackground extends StatelessWidget {
  final Widget child;
  const GlassBackground({super.key, required this.child});

  @override
  Widget build(BuildContext context) {
    final isDark = Theme.of(context).brightness == Brightness.dark;
    return Container(
      decoration: BoxDecoration(
        gradient: LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: isDark
              ? [const Color(0xFF16181F), const Color(0xFF1D1B2E)]
              : [const Color(0xFFE6F0FF), const Color(0xFFF0E6FF)],
        ),
      ),
      child: child,
    );
  }
}

// 全局玻璃降级开关：true 则所有 GlassCard 跳过 BackdropFilter（纯色卡），省 GPU、避免掉帧。
// 默认 Web 开（Flutter Web 的 BackdropFilter 慢且不稳）；原生默认关。用户显式设置过则以用户值为准。
final glassReduced = ValueNotifier<bool>(kIsWeb);

const _glassPrefsKey = 'glass_reduced';

/// 启动调用：用户设置过则覆盖默认（含 Web 用户手动关闭、原生用户手动开启两种情况）。
Future<void> loadGlassPrefs() async {
  final prefs = await SharedPreferences.getInstance();
  if (prefs.containsKey(_glassPrefsKey)) {
    glassReduced.value = prefs.getBool(_glassPrefsKey) ?? glassReduced.value;
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

/// 液态玻璃卡片：BackdropFilter 模糊 + 半透明 + 大圆角 + 柔和阴影。
///
/// 优雅降级：`glassReduced`（Web 默认 / 用户「性能模式」）或调用方传 `blurSigma=0` 时
/// sigma 归零 → **跳过 BackdropFilter**，只渲染圆角实色卡，省掉 backdrop 采集（低端机/大列表不掉帧）。
/// 无模糊时抬高底色不透明度（见 [glassCardAlpha]）保证可读。
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
    this.padding = const EdgeInsets.all(20),
    this.blurSigma = 18,
    this.radius = 20,
    this.onTap,
    this.tint,
  });

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: glassReduced,
      builder: (context, _) {
        final isDark = Theme.of(context).brightness == Brightness.dark;
        final reduced = glassReduced.value;
        final sigma = effectiveGlassSigma(blurSigma, reduced);
        var cardColor = Colors.white.withValues(
          alpha: glassCardAlpha(isDark, reduced),
        );
        if (tint != null) {
          cardColor = Color.alphaBlend(
            tint!.withValues(alpha: 0.12),
            cardColor,
          );
        }
        final borderColor = Colors.white.withValues(
          alpha: isDark ? 0.08 : 0.65,
        );

        final inner = Container(
          decoration: BoxDecoration(
            color: cardColor,
            borderRadius: BorderRadius.circular(radius),
            border: Border.all(color: borderColor),
            boxShadow: [
              BoxShadow(
                color: Colors.black.withValues(alpha: isDark ? 0.40 : 0.10),
                blurRadius: 28,
                offset: const Offset(0, 8),
              ),
            ],
          ),
          padding: padding,
          child: child,
        );

        // sigma>0：BackdropFilter 模糊；sigma==0：跳过 BackdropFilter，只留圆角实色卡。
        final content = ClipRRect(
          borderRadius: BorderRadius.circular(radius),
          child: sigma > 0
              ? BackdropFilter(
                  filter: ImageFilter.blur(sigmaX: sigma, sigmaY: sigma),
                  child: inner,
                )
              : inner,
        );

        if (onTap == null) return content;
        return InkWell(
          onTap: onTap,
          borderRadius: BorderRadius.circular(radius),
          child: content,
        );
      },
    );
  }
}
