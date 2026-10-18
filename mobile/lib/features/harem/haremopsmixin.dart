// HaremPage 的 HaremOpsMixin mixin（含 setState 段；从 harem_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/harem/harem_page.dart';

mixin HaremOpsMixin on State<HaremPage> {

  // 宿主 _HaremPageState 提供的钩子。
  Future<void> _export();
  Widget _overview(HaremOverview ov);
  Widget _prescription(PrescriptionItem p);
  void _showInvitees(PrescriptionItem p);


  HaremOverview get _emptyOverview => HaremOverview(
        totalSites: 0,
        totalInvitees: 0,
        totalPermInvites: 0,
        totalTempInvites: 0,
        totalLowRatio: 0,
        totalBanned: 0,
        totalNoData: 0,
      );

  @override
  Widget build(BuildContext context) {
    final ov = _data?.overview;
    return PTPageScaffold(
      title: '后宫',
      actions: [
        PTBarAction(onPressed: _export, icon: Icons.receipt_long, label: '导出'),
        PTBarAction(
          icon: Icons.refresh,
          label: '刷新',
          loading: _loading,
          onPressed: _loading ? null : () => _load(refresh: true),
        ),
      ],
      siteFilter: (_data?.prescriptions ?? []).isEmpty
          ? null
          : PTSiteFilterConfig(
              selected: _selectedSiteIds,
              onChanged: (v) => setState(() => _selectedSiteIds = v),
              options: _siteOptions,
            ),
      body: _loading && (_data?.prescriptions ?? []).isEmpty
          ? const PTLoadingView()
          : _error != null
          ? PTEmptyState(text: _error!, onRetry: _load)
          : _data == null
          ? PTEmptyState(text: '无数据')
          : Column(
              children: [
                StreamNoticeBanner(text: _streamBanner),
                if (ov != null)
                  Padding(
                    padding: const EdgeInsets.fromLTRB(12, 12, 12, 4),
                    child: _overview(ov),
                  ),
                Expanded(
                  child: RefreshIndicator(
                    onRefresh: () => _load(refresh: true),
                    child: PTAdaptiveTable<PrescriptionItem>(
                      items: _filteredPrescriptions,
                      columns: [
                        PTColumn<PrescriptionItem>(
                          label: '站点',
                          flex: 2,
                          cell: (_, p) => Text(
                            p.siteName,
                            style: const TextStyle(
                              fontWeight: FontWeight.w600,
                              fontSize: 13,
                            ),
                          ),
                        ),
                        PTColumn<PrescriptionItem>(
                          label: '现药',
                          flex: 1,
                          numeric: true,
                          cell: (_, p) => Text(
                            '${p.remain}',
                            style: const TextStyle(fontSize: 12),
                          ),
                        ),
                        PTColumn<PrescriptionItem>(
                          label: '可买',
                          flex: 1,
                          numeric: true,
                          cell: (_, p) => Text(
                            '${p.canBuy}',
                            style: const TextStyle(fontSize: 12),
                          ),
                        ),
                        PTColumn<PrescriptionItem>(
                          label: '权限',
                          flex: 1,
                          cell: (_, p) => Text(
                            p.canInvite ? '✓' : '✗',
                            style: TextStyle(
                              color: p.canInvite
                                  ? PTColors.success
                                  : PTColors.danger,
                              fontSize: 13,
                            ),
                          ),
                        ),
                        PTColumn<PrescriptionItem>(
                          label: '后宫',
                          flex: 1,
                          numeric: true,
                          cell: (_, p) => Text(
                            '${p.haremCount}${p.deadCount > 0 ? " (${p.deadCount})" : ""}',
                            style: const TextStyle(fontSize: 12),
                          ),
                        ),
                        PTColumn<PrescriptionItem>(
                          label: '操作',
                          flex: 2,
                          cell: (_, p) => p.invitees.isEmpty
                              ? Text(
                                  '—',
                                  style: TextStyle(
                                    color: context.dimText,
                                    fontSize: 12,
                                  ),
                                )
                              : TextButton(
                                  style: TextButton.styleFrom(
                                    padding: EdgeInsets.zero,
                                    minimumSize: const Size(0, 28),
                                    tapTargetSize:
                                        MaterialTapTargetSize.shrinkWrap,
                                  ),
                                  onPressed: () => _showInvitees(p),
                                  child: Text(
                                    '花名册 (${p.invitees.length})',
                                    style: const TextStyle(fontSize: 12),
                                  ),
                                ),
                        ),
                      ],
                      narrowCard: (_, p) => _prescription(p),
                      emptyText: '暂无处方',
                    ),
                  ),
                ),
              ],
            ),
    );
  }
  late final _api = HaremApi(frames: widget.frames);
  StreamSubscription<HaremStreamEvent>? _sub; // 流式首屏订阅（切换/刷新时 cancel）
  HaremResult? _data;
  bool _loading = true;
  String? _error;
  String? _streamBanner; // done 帧账面：已有数据时的顶部常驻说明
  List<int> _selectedSiteIds = []; // 站点多选筛选，空=全部
  // 按 selectedSiteIds 过滤处方（数据已全量在 _data，纯前端筛选）。
  List<PrescriptionItem> get _filteredPrescriptions {
    final list = _data?.prescriptions ?? const <PrescriptionItem>[];
    if (_selectedSiteIds.isEmpty) return list;
    return list.where((p) => _selectedSiteIds.contains(p.siteId)).toList();
  }
  // 站点筛选候选项（从处方去重 siteId/siteName）。
  List<PTFilterOption<Object?>> get _siteOptions {
    final seen = <int, String>{};
    for (final p in _data?.prescriptions ?? const <PrescriptionItem>[]) {
      seen.putIfAbsent(p.siteId, () => p.siteName);
    }
    return seen.entries
        .map((e) => PTFilterOption<Object?>(value: e.key, label: e.value))
        .toList();
  }
  Future<void> _load({bool refresh = false}) {
    _sub?.cancel();
    setState(() {
      _data = HaremResult(overview: _emptyOverview, prescriptions: []);
      _loading = true;
      _error = null;
      _streamBanner = null;
    });
    _sub = _api.allStream(refresh: refresh).listen(
      (ev) {
        if (!mounted) return;
        setState(() {
          final cur = _data!;
          final merged = [...cur.prescriptions, ...ev.items];
          _data = HaremResult(
            overview: ev.overview ?? cur.overview,
            prescriptions: merged,
          );
          // 清单读失败且一条药单都没到：说「读不到」，别让空态冒充「没有药单」。
          final notice = streamNoticeOf(
            arrived: merged.length,
            listError: ev.listError,
          );
          _error = notice.error ?? _error;
          _streamBanner = notice.banner ?? _streamBanner;
        });
      },
      onDone: () {
        if (mounted) setState(() => _loading = false);
      },
      onError: (e) {
        if (!mounted) return;
        setState(() {
          _loading = false;
          _error = e.toString();
        });
      },
    );
    return Future.value();
  }
}
