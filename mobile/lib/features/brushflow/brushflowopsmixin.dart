// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// BrushFlowPage 的 BrushFlowOpsMixin mixin（含 setState 段；从 brushflow_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/brushflow/brushflow_page.dart';

mixin BrushFlowOpsMixin on State<BrushFlowPage> {

  final _api = BrushFlowApi();
  final _siteApi = SiteApi();
  final _dlApi = DownloaderApi();
  final _filterApi = FilterRuleApi();
  List<BrushFlowTask> _list = [];
  bool _loading = true;
  String? _error;
  // 站点 / 下载器名查表缓存（拉取一次，表单下拉 + 卡片 meta 共用）。
  List<Site> _sites = [];
  List<Downloader> _downloaders = [];
  // 可选过滤规则组名缓存（表单多选）。
  List<String> _filterGroups = [];
  // 选种 / 扫删进行中的任务 id，禁用对应按钮。
  final _busy = <int>{};

  // 宿主提供的钩子。
  void _toast(String msg);
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

  Future<void> _ensureRefs() async {
    try {
      final results = await Future.wait([
        _siteApi.list(),
        _dlApi.list(),
        _filterApi.groups(),
      ]);
      if (!mounted) return;
      setState(() {
        _sites = results[0] as List<Site>;
        _downloaders = results[1] as List<Downloader>;
        _filterGroups =
            (results[2] as List<FilterRuleGroup>).map((g) => g.name).toList();
      });
    } catch (_) {
      // 名称解析回退到「#id」，不阻塞主流程。
    }
  }

  // 切换启用：乐观更新 + 失败回滚。
  Future<void> _toggleEnabled(BrushFlowTask t, bool v) async {
    setState(() {
      _list = _list
          .map((e) => e.id == t.id ? e.copyWith(enabled: v) : e)
          .toList();
    });
    try {
      await _api.update(t.id, {'enabled': v});
    } catch (e) {
      _toast('更新失败：$e');
      await _load();
    }
  }

  Future<void> _select(BrushFlowTask t) async {
    setState(() => _busy.add(t.id));
    try {
      final r = await _api.select(t.id);
      _toast((r['message'] as String?) ?? '选种完成');
      await _load();
    } catch (e) {
      _toast('选种失败：$e');
    } finally {
      if (mounted) setState(() => _busy.remove(t.id));
    }
  }

  Future<void> _sweep(BrushFlowTask t) async {
    setState(() => _busy.add(t.id));
    try {
      final r = await _api.sweep(t.id);
      _toast((r['message'] as String?) ?? '扫删完成');
      await _load();
    } catch (e) {
      _toast('扫删失败：$e');
    } finally {
      if (mounted) setState(() => _busy.remove(t.id));
    }
  }
}
