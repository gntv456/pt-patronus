// SubscriptionsPage 的 SubscriptionsOpsMixin mixin（含 setState 段；从 subscriptions_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/subscriptions/subscriptions_page.dart';

mixin SubscriptionsOpsMixin on State<SubscriptionsPage> {

  final _api = SubscribeApi();
  final _siteApi = SiteApi();
  List<Subscription> _subs = [];
  List<Site> _sites = const []; // 参与站点多选选项（活跃站）
  bool _loading = true;
  String? _error;
  final _checking = <int>{};
  GlobalDefaults _global = const GlobalDefaults();

  // 宿主提供的钩子。
  void _toast(String m);
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

  Future<void> _loadGlobal() async {
    try {
      final g = await _api.getGlobalDefaults();
      if (!mounted) return;
      setState(() => _global = g);
    } catch (_) {
      // 加载失败不阻塞订阅展示。
    }
  }

  Future<void> _openGlobal() async {
    final result = await showDialog<GlobalDefaults>(
      context: context,
      builder: (_) => _GlobalDefaultsDialog(initial: _global, sites: _sites),
    );
    if (result == null) return;
    try {
      final saved = await _api.updateGlobalDefaults(result);
      if (!mounted) return;
      setState(() => _global = saved);
      _toast('全局默认已保存');
    } catch (e) {
      _toast('$e');
    }
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

  // 洗版三态：单击 toggle 自定义值；长按重置跟随全局。
  Future<void> _toggleBestVersion(Subscription s) async {
    final next = !(s.bestVersion ?? _global.bestVersion);
    setState(() {
      final i = _subs.indexWhere((x) => x.id == s.id);
      if (i >= 0) _subs[i] = s.copyWith(bestVersion: next);
    });
    try {
      await _api.update(s.id, {'best_version': next});
      if (mounted) _toast(next ? '已开启洗版' : '已关闭洗版');
    } catch (e) {
      if (mounted) await _load();
      _toast('$e');
    }
  }

  // 评分保护三态（同洗版）。
  Future<void> _toggleRatingGuard(Subscription s) async {
    final next = !(s.enableRatingGuard ?? _global.enableRatingGuard);
    setState(() {
      final i = _subs.indexWhere((x) => x.id == s.id);
      if (i >= 0) _subs[i] = s.copyWith(enableRatingGuard: next);
    });
    try {
      await _api.update(s.id, {'enable_rating_guard': next});
      if (mounted) _toast(next ? '已开启评分保护' : '已关闭评分保护');
    } catch (e) {
      if (mounted) await _load();
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
}
