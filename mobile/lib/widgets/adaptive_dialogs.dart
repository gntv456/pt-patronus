// 从 adaptive.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/widgets/adaptive.dart';

class _PTWideDialog extends StatelessWidget {
  final String? title;
  final double maxWidth;
  final bool dismissible;
  final Widget child;
  const _PTWideDialog({
    this.title,
    required this.maxWidth,
    required this.dismissible,
    required this.child,
  });

  @override
  Widget build(BuildContext context) {
    Widget panel = GlassCard(
      padding: const EdgeInsets.all(20),
      child: ConstrainedBox(
        constraints: BoxConstraints(maxWidth: maxWidth),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            if (title != null)
              Padding(
                padding: const EdgeInsets.only(bottom: 12),
                child: Row(
                  children: [
                    Expanded(
                      child: Text(
                        title!,
                        style: TextStyle(
                          fontSize: PTFs.xl,
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                    ),
                    IconButton(
                      visualDensity: VisualDensity.compact,
                      tooltip: '关闭',
                      icon: const Icon(Icons.close),
                      onPressed: () => Navigator.of(context).maybePop(),
                    ),
                  ],
                ),
              ),
            Flexible(child: SingleChildScrollView(child: child)),
          ],
        ),
      ),
    );
    return Center(
      child: Padding(padding: const EdgeInsets.all(24), child: panel),
    );
  }
}

class _PTBottomSheet extends StatelessWidget {
  final String? title;
  final Widget child;
  const _PTBottomSheet({this.title, required this.child});

  @override
  Widget build(BuildContext context) {
    final isDark = Theme.of(context).brightness == Brightness.dark;
    return Container(
      decoration: BoxDecoration(
        color: isDark ? const Color(0xFF1E2024) : Colors.white,
        borderRadius: const BorderRadius.vertical(top: Radius.circular(20)),
      ),
      child: Padding(
        padding: EdgeInsets.fromLTRB(
          16,
          8,
          16,
          16 + MediaQuery.viewInsetsOf(context).bottom,
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            if (title != null)
              Padding(
                padding: const EdgeInsets.only(bottom: 8, top: 4),
                child: Text(
                  title!,
                  style: TextStyle(
                    fontSize: PTFs.xl,
                    fontWeight: FontWeight.w700,
                  ),
                ),
              ),
            Flexible(child: SingleChildScrollView(child: child)),
          ],
        ),
      ),
    );
  }
}
