import 'package:flutter/material.dart';
import 'package:ptpatronus/features/home/home_controller.dart';
import 'package:ptpatronus/features/home/home_nav_items.dart';
import 'package:ptpatronus/api/site.dart';
import 'package:ptpatronus/api/subscribe.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/media_poster.dart';
import 'package:ptpatronus/widgets/pt_filter_sheet.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';

// 订阅列表（对标 Web Subscriptions）：展示订阅状态，支持手动检查 / 暂停-恢复 /
// 改分辨率 / 删除。订阅的创建在影视详情页「订阅」按钮（同源同 id 幂等）。
class SubscriptionsPage extends StatefulWidget {
  const SubscriptionsPage({super.key});
  @override
  State<SubscriptionsPage> createState() => _SubscriptionsPageState();
}

class _SubscriptionsPageState extends State<SubscriptionsPage> {
  final _api = SubscribeApi();
  final _siteApi = SiteApi();
  List<Subscription> _subs = [];
  List<Site> _sites = const []; // 参与站点多选选项（活跃站）
  bool _loading = true;
  String? _error;
  final _checking = <int>{};

  @override
  void initState() {
    super.initState();
    _load();
    _loadSites();
  }

  Future<void> _loadSites() async {
    try {
      final all = await _siteApi.list();
      if (!mounted) return;
      setState(() => _sites = all.where((s) => s.isActive).toList());
    } catch (_) {
      // 站点列表加载失败不阻塞订阅展示，选择器显示空即可。
    }
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      _subs = await _api.list();
    } catch (e) {
      _error = '$e';
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  // 立即检查：返回本轮命中推送条数。
  Future<void> _check(Subscription s) async {
    setState(() => _checking.add(s.id));
    try {
      final r = await _api.check(s.id);
      final ok = r['ok'] == true;
      final pushed = (r['pushed'] as num?)?.toInt() ?? 0;
      final err = r['error'] as String?;
      _toast(
        ok
            ? (pushed > 0 ? '命中并推送 $pushed 条' : '本轮暂无新资源')
            : (err?.isNotEmpty == true ? err! : '检查失败'),
      );
      await _load();
    } catch (e) {
      _toast('$e');
    } finally {
      if (mounted) setState(() => _checking.remove(s.id));
    }
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

  // 洗版开关：开启后持续寻找更高画质版本，命中更优则替换库中旧版（电影达顶级 Pri100 自动完成）。
  Future<void> _toggleBestVersion(Subscription s) async {
    try {
      await _api.update(s.id, {'best_version': !s.bestVersion});
      await _load();
      _toast(s.bestVersion ? '已关闭洗版' : '已开启洗版（自动升级画质）');
    } catch (e) {
      _toast('$e');
    }
  }

  // 促销类型多选：弹对话框勾选（空=不限）。乐观更新 + 失败回滚。
  Future<void> _editPromoTypes(Subscription s) async {
    final result = await showDialog<List<String>>(
      context: context,
      builder: (_) => _PromoTypeDialog(initial: s.promoTypes),
    );
    if (result == null) return; // 取消
    final prev = s.promoTypes;
    setState(() {
      final i = _subs.indexWhere((x) => x.id == s.id);
      if (i >= 0) _subs[i] = s.copyWith(promoTypes: result);
    });
    try {
      await _api.update(s.id, {'promo_types': result});
    } catch (e) {
      if (mounted) {
        setState(() {
          final i = _subs.indexWhere((x) => x.id == s.id);
          if (i >= 0) _subs[i] = s.copyWith(promoTypes: prev); // 回滚
        });
      }
      _toast('$e');
    }
  }

  // 参与站点：弹多选面板，实时回调即保存（空=全部活跃站）。乐观更新本地。
  Future<void> _editSites(Subscription s) async {
    if (_sites.isEmpty) {
      _toast('站点列表加载中或为空');
      return;
    }
    showSitePickerSheet(
      context,
      title: '参与订阅的站点',
      selected: [...s.siteIds],
      options: _sites
          .map((site) => PTFilterOption<Object?>(value: site.id, label: site.name))
          .toList(),
      onChanged: (v) async {
        try {
          await _api.update(s.id, {'site_ids': v});
          if (!mounted) return;
          setState(() {
            final i = _subs.indexWhere((x) => x.id == s.id);
            if (i >= 0) _subs[i] = s.copyWith(siteIds: v);
          });
        } catch (e) {
          _toast('$e');
        }
      },
    );
  }

  // 分辨率过滤：弹窗编辑（留空=不过滤；后端空=不改，故已设值无法在此清空，与 Web 一致）。
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

  String _statusText(String s) =>
      const {'subscribed': '订阅中', 'completed': '已完成', 'paused': '已暂停'}[s] ?? s;

  Color _statusColor(String s) => switch (s) {
    'subscribed' => PTColors.success,
    'completed' => PTColors.primary,
    _ => PTColors.warn,
  };

  String _fmtTime(String? t) => (t == null || t.isEmpty)
      ? '—'
      : t.replaceAll('T', ' ').replaceAll(RegExp(r'\.\d+Z?$'), '');

  // 空状态 CTA：切到主菜单的「搜索」Tab（经 HomeShell 控制器按 label 定位索引，避免硬编码）。
  void _goSearch() {
    final idx = navItems.indexWhere((n) => n.label == '搜索');
    if (idx >= 0) HomeShellScope.of(context).select(idx);
  }

  @override
  Widget build(BuildContext context) {
    return PTPageScaffold(
      title: '订阅',
      actions: [
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

  Widget _buildBody() {
    if (_loading && _subs.isEmpty) {
      return const Center(child: CircularProgressIndicator());
    }
    if (_error != null) {
      return PTEmptyState(text: _error!, onRetry: _load);
    }
    if (_subs.isEmpty) {
      return PTEmptyState(
        icon: Icons.subscriptions_outlined,
        text: '暂无订阅',
        description: '去搜索/推荐找到影视 → 进详情页 → 订阅',
        actionLabel: '去搜索影视',
        onAction: _goSearch,
      );
    }
    return ListView.separated(
      padding: const EdgeInsets.fromLTRB(12, 4, 12, 24),
      itemCount: _subs.length,
      separatorBuilder: (_, _) => const SizedBox(height: 12),
      itemBuilder: (_, i) => _card(_subs[i]),
    );
  }

  Widget _card(Subscription s) {
    return GlassCard(
      padding: const EdgeInsets.all(12),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            width: 72,
            height: 104,
            child: ClipRRect(
              borderRadius: BorderRadius.circular(8),
              child: MediaPoster(
                url: s.posterUrl,
                kind: s.type == 'music' ? 'music' : 'movie',
              ),
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    Flexible(
                      child: Text(
                        s.title,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(
                          fontWeight: FontWeight.w700,
                          fontSize: 15,
                        ),
                      ),
                    ),
                    if (s.year > 0)
                      Padding(
                        padding: const EdgeInsets.only(left: 4),
                        child: Text(
                          '(${s.year})',
                          style: const TextStyle(
                            color: Colors.grey,
                            fontSize: 13,
                          ),
                        ),
                      ),
                  ],
                ),
                const SizedBox(height: 4),
                Wrap(
                  spacing: 6,
                  runSpacing: 4,
                  children: [
                    _chip(_statusText(s.status), _statusColor(s.status)),
                    if (s.resolution.isNotEmpty)
                      _chip(s.resolution, PTColors.primary2),
                    if (s.bestVersion)
                      _chip(
                        s.currentPriority > 0 ? '洗版 Pri${s.currentPriority}' : '洗版中',
                        PTColors.success,
                      ),
                  ],
                ),
                const SizedBox(height: 4),
                Text(
                  '上次检查：${_fmtTime(s.lastCheckedAt)} · 类型 ${s.type == 'tv' ? '剧集' : '电影'} · 源 ${s.source}',
                  style: const TextStyle(color: Colors.grey, fontSize: 12),
                ),
                const SizedBox(height: 8),
                Wrap(
                  spacing: 8,
                  runSpacing: 8,
                  children: [
                    FilledButton.tonalIcon(
                      onPressed: _checking.contains(s.id)
                          ? null
                          : () => _check(s),
                      icon: _checking.contains(s.id)
                          ? const SizedBox(
                              width: 14,
                              height: 14,
                              child: CircularProgressIndicator(strokeWidth: 2),
                            )
                          : const Icon(Icons.refresh, size: 16),
                      label: const Text('立即检查', style: TextStyle(fontSize: 12)),
                    ),
                    FilledButton.tonalIcon(
                      onPressed: () => _editResolution(s),
                      icon: const Icon(Icons.high_quality_outlined, size: 16),
                      label: Text(
                        s.resolution.isEmpty ? '设分辨率' : '改分辨率',
                        style: const TextStyle(fontSize: 12),
                      ),
                    ),
                    FilledButton.tonalIcon(
                      onPressed: () => _toggleBestVersion(s),
                      style: FilledButton.styleFrom(
                        foregroundColor: s.bestVersion ? PTColors.success : null,
                      ),
                      icon: Icon(
                        s.bestVersion ? Icons.auto_awesome : Icons.auto_awesome_outlined,
                        size: 16,
                      ),
                      label: Text(
                        s.bestVersion ? '洗版中' : '洗版',
                        style: const TextStyle(fontSize: 12),
                      ),
                    ),
                    FilledButton.tonalIcon(
                      onPressed: () => _editPromoTypes(s),
                      style: FilledButton.styleFrom(
                        foregroundColor: s.promoTypes.isNotEmpty ? PTColors.success : null,
                      ),
                      icon: const Icon(Icons.savings_outlined, size: 16),
                      label: Text(
                        s.promoTypes.isEmpty ? '促销' : '${s.promoTypes.length}种促销',
                        style: const TextStyle(fontSize: 12),
                      ),
                    ),
                    FilledButton.tonalIcon(
                      onPressed: () => _editSites(s),
                      style: FilledButton.styleFrom(
                        foregroundColor: s.siteIds.isNotEmpty ? PTColors.primary : null,
                      ),
                      icon: const Icon(Icons.public, size: 16),
                      label: Text(
                        s.siteIds.isEmpty ? '全部站' : '${s.siteIds.length}站',
                        style: const TextStyle(fontSize: 12),
                      ),
                    ),
                    FilledButton.tonalIcon(
                      onPressed: () => _toggle(s),
                      icon: Icon(
                        s.status == 'paused' ? Icons.play_arrow : Icons.pause,
                        size: 16,
                      ),
                      label: Text(
                        s.status == 'paused' ? '恢复' : '暂停',
                        style: const TextStyle(fontSize: 12),
                      ),
                    ),
                    FilledButton.tonalIcon(
                      onPressed: () => _remove(s),
                      style: FilledButton.styleFrom(
                        foregroundColor: PTColors.danger,
                      ),
                      icon: const Icon(Icons.delete_outline, size: 16),
                      label: const Text('删除', style: TextStyle(fontSize: 12)),
                    ),
                  ],
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _chip(String label, Color color) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.16),
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(
        label,
        style: TextStyle(
          color: color,
          fontSize: 11,
          fontWeight: FontWeight.w600,
        ),
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

// 促销类型多选对话框（CheckboxListTile 列表 + 确定/取消；空选=不限）。
class _PromoTypeDialog extends StatefulWidget {
  final List<String> initial;
  const _PromoTypeDialog({required this.initial});
  @override
  State<_PromoTypeDialog> createState() => _PromoTypeDialogState();
}

class _PromoTypeDialogState extends State<_PromoTypeDialog> {
  late final Set<String> _selected;

  @override
  void initState() {
    super.initState();
    _selected = {...widget.initial};
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: const Text('促销类型（空=不限）'),
      content: SizedBox(
        width: double.maxFinite,
        child: ListView(
          shrinkWrap: true,
          children: _promoOptions.map((o) {
            return CheckboxListTile(
              dense: true,
              contentPadding: const EdgeInsets.symmetric(horizontal: 8),
              value: _selected.contains(o.$1),
              title: Text(o.$2),
              onChanged: (v) => setState(() {
                if (v == true) {
                  _selected.add(o.$1);
                } else {
                  _selected.remove(o.$1);
                }
              }),
            );
          }).toList(),
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: const Text('取消'),
        ),
        TextButton(
          onPressed: () => Navigator.pop(context, _selected.toList()),
          child: const Text('确定'),
        ),
      ],
    );
  }
}
