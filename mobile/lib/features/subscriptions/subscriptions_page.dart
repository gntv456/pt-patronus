import 'package:flutter/material.dart';
import 'package:ptpatronus/features/home/home_controller.dart';
import 'package:ptpatronus/features/home/home_nav_items.dart';
import 'package:ptpatronus/api/site.dart';
import 'package:ptpatronus/api/subscribe.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/media_poster.dart';
import 'package:ptpatronus/widgets/pt_filter_sheet.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';

part 'promo_type_dialog.dart';
part 'global_defaults_dialog.dart';

part 'subscriptionsopsmixin.dart';
part 'subscriptions_widgets.dart';

// 订阅列表（对标 Web Subscriptions）：展示订阅状态，支持手动检查 / 暂停-恢复 /
// 改分辨率 / 删除。订阅的创建在影视详情页「订阅」按钮（同源同 id 幂等）。
class SubscriptionsPage extends StatefulWidget {
  const SubscriptionsPage({super.key});
  @override
  State<SubscriptionsPage> createState() => _SubscriptionsPageState();
}

class _SubscriptionsPageState extends State<SubscriptionsPage> with SubscriptionsOpsMixin {

  @override
  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }
  @override
  void initState() {
    super.initState();
    _load();
    _loadSites();
    _loadGlobal();
  }

  Future<void> _toggle(Subscription s) async {
    final next = s.status == 'paused' ? 'subscribed' : 'paused';
    try {
      await _api.update(s.id, {'status': next});
      await _load();
    } catch (e) {
      _toast('$e');
    }
  }

  Future<void> _resetBestVersion(Subscription s) async {
    try {
      await _api.reset(s.id, ['best_version']);
      await _load();
      _toast('洗版已恢复跟随全局');
    } catch (e) {
      _toast('$e');
    }
  }

  Future<void> _resetRatingGuard(Subscription s) async {
    try {
      await _api.reset(s.id, ['enable_rating_guard']);
      await _load();
      _toast('评分保护已恢复跟随全局');
    } catch (e) {
      _toast('$e');
    }
  }

  // 分辨率过滤：弹窗编辑（留空=不过滤；后端用字段存在性区分清空与不修改）。
  Future<void> _editResolution(Subscription s) async {
    final ctrl = TextEditingController(text: s.resolution);
    final v = await showDialog<String>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('分辨率过滤'),
        content: TextField(
          controller: ctrl,
          autofocus: true,
          decoration: const InputDecoration(hintText: '如 1080p / 2160p，留空=不限'),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('取消'),
          ),
          TextButton(
            onPressed: () => Navigator.pop(context, ctrl.text.trim()),
            child: const Text('确定'),
          ),
        ],
      ),
    );
    if (v == null) return;
    try {
      await _api.update(s.id, {'resolution': v});
      _toast('分辨率已更新');
      await _load();
    } catch (e) {
      _toast('$e');
    }
  }

  Future<void> _remove(Subscription s) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('删除订阅'),
        content: Text('确认删除「${s.title}」的订阅？'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          TextButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('删除'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await _api.remove(s.id);
      _toast('已删除');
      await _load();
    } catch (e) {
      _toast('$e');
    }
  }

  @override
  Widget build(BuildContext context) {
    return PTPageScaffold(
      title: '订阅',
      actions: [
        PTBarAction(
          icon: Icons.tune,
          label: '全局设置',
          onPressed: _openGlobal,
        ),
        PTBarAction(
          icon: Icons.refresh,
          label: '刷新',
          loading: _loading,
          onPressed: _loading ? null : _load,
        ),
      ],
      body: SafeArea(
        child: RefreshIndicator(onRefresh: _load, child: _buildBody()),
      ),
    );
  }

}

// 促销类型选项 (value, label；与后端 site.PromoType 对齐)。
const _promoOptions = <(String, String)>[
  ('free', '免费'),
  ('2xfree', '2X免费'),
  ('2x', '双倍上传'),
  ('discount', '折扣下载'),
  ('2xdiscount', '2X+折扣'),
];

// 促销值 → 中文标签（_promoOptions 查表；全局设置弹窗摘要用）。
String promoLabel(String v) {
  for (final o in _promoOptions) {
    if (o.$1 == v) return o.$2;
  }
  return v;
}
