// ContractPage 的 ContractOpsMixin mixin（含 setState 段；从 contract_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/contract/contract_page.dart';

mixin ContractOpsMixin on State<ContractPage> {

  // 宿主 _ContractPageState 提供的钩子。
  Future<void> _showCheckResult(ContractCheckResult r);
  String _siteName(int id);
  String _statusLabel(String s);
  void _toast(String msg);

  final _api = ContractApi();
  final _siteApi = SiteApi();

  List<ContractRule> _list = [];
  bool _loading = true;
  String? _error;

  // 站点名查表缓存（拉取一次，卡片 meta + 表单下拉共用）。
  List<Site> _sites = [];

  // 检查进行中的规则 id，禁用对应按钮。
  final _busy = <int>{};

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final list = await _api.list();
      if (!mounted) return;
      setState(() => _list = list);
    } catch (e) {
      if (!mounted) return;
      setState(() => _error = '$e');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _ensureSites() async {
    if (_sites.isNotEmpty) return;
    try {
      final sites = await _siteApi.list();
      if (!mounted) return;
      setState(() => _sites = sites);
    } catch (_) {
      // 名称解析回退到「#id」，不阻塞主流程。
    }
  }

  // 切换启用：乐观更新 + 失败回滚。upsert 按 site_id 全量写回。
  Future<void> _toggleEnabled(ContractRule r, bool v) async {
    setState(() {
      _list = _list
          .map((e) => e.id == r.id ? _copyWithEnabled(e, v) : e)
          .toList();
    });
    try {
      await _api.upsert(_ruleBody(r, enabled: v));
    } catch (e) {
      _toast('更新失败：$e');
      await _load();
    }
  }

  ContractRule _copyWithEnabled(ContractRule r, bool v) => ContractRule(
        id: r.id,
        siteId: r.siteId,
        enabled: v,
        minRatio: r.minRatio,
        minSeeding: r.minSeeding,
        minUploadGb: r.minUploadGb,
        officialOnly: r.officialOnly,
        minSeedingSizeGb: r.minSeedingSizeGb,
        period: r.period,
        startTime: r.startTime,
        lastCheckedAt: r.lastCheckedAt,
        lastStatus: r.lastStatus,
        lastDetail: r.lastDetail,
        createdAt: r.createdAt,
        updatedAt: r.updatedAt,
      );

  /// 由现有规则组装 upsert body（[enabled] 可覆盖，供乐观切换）。
  Map<String, dynamic> _ruleBody(ContractRule r, {bool? enabled}) => {
        if (r.id != 0) 'id': r.id,
        'site_id': r.siteId,
        'enabled': enabled ?? r.enabled,
        'min_ratio': r.minRatio,
        'min_seeding': r.minSeeding,
        'min_upload_gb': r.minUploadGb,
        'official_only': r.officialOnly,
        'min_seeding_size_gb': r.minSeedingSizeGb,
        'period': r.period,
        if (r.startTime != null)
          'start_time': r.startTime!.toUtc().toIso8601String(),
      };

  Future<void> _check(ContractRule r) async {
    setState(() => _busy.add(r.id));
    try {
      final res = await _api.check(r.id);
      if (!mounted) return;
      _toast('${_statusLabel(res.status)}：${_siteName(res.siteId)}');
      await _load();
      if (!mounted) return;
      await _showCheckResult(res);
    } catch (e) {
      _toast('检查失败：$e');
    } finally {
      if (mounted) setState(() => _busy.remove(r.id));
    }
  }

}
