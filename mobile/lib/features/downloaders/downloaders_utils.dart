// downloaders 页的工具函数：品牌元数据/表单选项/错误视图/格式化。
// （从 downloaders_page.dart 拆出；行为不变。）
part of 'package:ptpatronus/features/downloaders/downloaders_page.dart';

// 下载器类型 → 品牌图标/配色/标签（与 Web Downloaders.vue 的 DL_META 1:1）。
// 新增下载器类型时在此加一条 case，卡片徽标与表单下拉会自动用上对应图标。
({String glyph, Color from, Color to, String label}) _dlMeta(String t) {
  switch (t) {
    case 'tr':
      return (
        glyph: 'transmission',
        from: const Color(0xFFE8443C),
        to: const Color(0xFFA82424),
        label: 'Transmission',
      );
    case 'ut':
      return (
        glyph: 'utorrent',
        from: const Color(0xFF5BB04C),
        to: const Color(0xFF3A7A2E),
        label: 'uTorrent',
      );
    case 'dl':
      return (
        glyph: 'deluge',
        from: const Color(0xFF4FB6E8),
        to: const Color(0xFF2A7AB3),
        label: 'Deluge',
      );
    case 'ar':
      return (
        glyph: 'aria2',
        from: const Color(0xFFF0A030),
        to: const Color(0xFFC07010),
        label: 'Aria2',
      );
    case 'rt':
      return (
        glyph: 'rutorrent',
        from: const Color(0xFF2E8B57),
        to: const Color(0xFF1A5C38),
        label: 'ruTorrent',
      );
    case 'fl':
      return (
        glyph: 'flood',
        from: const Color(0xFF5B8DEF),
        to: const Color(0xFF2F5FCC),
        label: 'Flood',
      );
    default:
      return (
        glyph: 'qbittorrent',
        from: const Color(0xFF72B4F5),
        to: const Color(0xFF356EBF),
        label: 'qBittorrent',
      );
  }
}

// 表单类型选项：品牌 GlassIcon + 名称（下拉选中态与展开列表均显示）。
Widget _dlOption(({String glyph, Color from, Color to, String label}) m) {
  return Row(
    children: [
      GlassIcon(glyph: m.glyph, from: m.from, to: m.to, size: 22),
      const SizedBox(width: 8),
      Text(m.label),
    ],
  );
}

// 子表（文件/Tracker/Peer/分类）加载失败的统一错误视图：文案 + 重试按钮，
// 与「真空态」（无文件/无 Tracker…）区分，避免失败被误显示为空。
Widget _sheetErrorView(String error, VoidCallback onRetry) {
  return Center(
    child: Padding(
      padding: const EdgeInsets.all(20),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            '加载失败：$error',
            style: const TextStyle(color: Colors.redAccent, fontSize: 13),
            textAlign: TextAlign.center,
          ),
          const SizedBox(height: 10),
          OutlinedButton(onPressed: onRetry, child: const Text('重试')),
        ],
      ),
    ),
  );
}

String _fmtBytes(int n) {
  if (n <= 0) return '-';
  const u = ['B', 'KB', 'MB', 'GB', 'TB'];
  double v = n.toDouble();
  int i = 0;
  while (v >= 1024 && i < u.length - 1) {
    v /= 1024;
    i++;
  }
  return '${v.toStringAsFixed(1)} ${u[i]}';
}

String _fmtSpeed(int? n) {
  if (n == null || n <= 0) return '0 B/s';
  return '${_fmtBytes(n)}/s';
}

String _fmtIso(String iso) {
  final dt = DateTime.tryParse(iso);
  if (dt == null || dt.year < 1971) return '-';
  String p(int v) => v.toString().padLeft(2, '0');
  return '${dt.year}-${p(dt.month)}-${p(dt.day)} ${p(dt.hour)}:${p(dt.minute)}';
}

bool _isCompleted(String iso) =>
    iso.isNotEmpty && !iso.startsWith('0001') && !iso.startsWith('1970');
