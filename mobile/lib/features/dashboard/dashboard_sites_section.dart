// _DashboardPageState 的站点卡片区：站点列表/单站卡/行操作/等级标签/连通性胶囊/告警卡/格式化。
// （二次拆分；行为不变。）
part of 'package:ptpatronus/features/dashboard/dashboard_page.dart';

extension on _DashboardPageState {
  // 站点明细：宽屏表格（对齐 Web el-table），窄屏卡片。操作列含「历史数据」「刷新数据」。
  Widget _sitesSection(BuildContext context) {
    if (!_config.showSiteList) return const SizedBox.shrink();
    final sites = _visibleSites();
    final columns = <PTColumn<SiteStatItem>>[
      PTColumn(
        label: '站点',
        flex: 2,
        cell: (_, s) => Text(
          s.siteName,
          style: const TextStyle(fontWeight: FontWeight.w600),
        ),
      ),
      PTColumn(label: '联通状态', flex: 1, cell: (_, s) => _connectivityPill(s)),
      PTColumn(label: '等级', flex: 1, cell: (_, s) => _levelTag(s.level)),
      PTColumn(
        label: '上传',
        flex: 1,
        numeric: true,
        cell: (_, s) => Text(_fmtBytes(s.uploaded)),
      ),
      PTColumn(
        label: '下载',
        flex: 1,
        numeric: true,
        cell: (_, s) => Text(_fmtBytes(s.downloaded)),
      ),
      PTColumn(
        label: '分享率',
        flex: 1,
        numeric: true,
        cell: (ctx, s) => Text(
          s.ratio > 0 ? s.ratio.toStringAsFixed(2) : '-',
          style: TextStyle(
            color: s.ratio <= 0
                ? (Theme.of(ctx).brightness == Brightness.dark
                      ? Colors.white38
                      : Colors.black38)
                : (s.ratio >= 1 ? PTColors.success : PTColors.danger),
            fontWeight: FontWeight.w600,
          ),
        ),
      ),
      PTColumn(
        label: '做种',
        flex: 1,
        numeric: true,
        cell: (_, s) => Text('${s.seeding}'),
      ),
      PTColumn(label: '操作', flex: 1, cell: (ctx, s) => _rowActions(ctx, s)),
    ];

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: EdgeInsets.only(
            left: 2,
            bottom: isWideScreen(context) ? 8 : 6,
          ),
          child: Text(
            '站点明细（${sites.length}）',
            style: TextStyle(
              fontWeight: FontWeight.w600,
              fontSize: isWideScreen(context) ? 15 : 14,
            ),
          ),
        ),
        PTAdaptiveTable<SiteStatItem>(
          items: sites,
          columns: columns,
          emptyText: '暂无数据，等待统计同步',
          narrowCard: (c, s) => _siteStatCard(c, s),
          // 宽屏表格最小宽 860：窗口更窄时整表横向滚动（对齐 Web el-table min-width + 横滚），
          // 而非把 8 列硬塞进窗口宽度——否则操作列(flex:1)塞不下两个按钮→黄黑溢出斜纹，
          // 相邻站点列被压到极窄→英文站名逐字竖排（曾见于馒头/MTeam 行的刷新按钮处）。
          // 860÷9(flex 总和)≈95：操作列够放两枚 40px 按钮，站点列(flex:2)≈190 不压字。
          minTableWidth: 860,
          embedded: true, // 嵌在外层 ListView：窄屏卡片列表 shrinkWrap，避免塌缩空白
        ),
      ],
    );
  }

  Widget _statCell(String label, String value) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(label, style: TextStyle(color: context.dimText, fontSize: 11)),
        const SizedBox(height: 2),
        Text(
          value,
          style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w600),
        ),
      ],
    );
  }

  // 字节 → 人类可读（1024 进制，对齐后端 humanBytes）。
  String _fmtBytes(int b) {
    const unit = 1024.0;
    if (b <= 0) return '0 B';
    if (b < unit) return '$b B';
    double n = b / unit;
    int exp = 0;
    while (n >= unit && exp < 5) {
      n /= unit;
      exp++;
    }
    const units = ['KB', 'MB', 'GB', 'TB', 'PB'];
    return '${n.toStringAsFixed(1)} ${units[exp]}';
  }

  String _fmtCompact(double value) {
    if (value <= 0) return '-';
    if (value >= 1000000000) {
      return '${(value / 1000000000).toStringAsFixed(2)} G';
    }
    if (value >= 1000000) return '${(value / 1000000).toStringAsFixed(2)} M';
    if (value >= 1000) return '${(value / 1000).toStringAsFixed(2)} K';
    return value.toStringAsFixed(value >= 100 ? 0 : 1);
  }

  String _fmtRatio(double ratio) {
    if (ratio <= 0) return '-';
    if (ratio >= 10000) return '∞';
    if (ratio >= 100) return ratio.toStringAsFixed(0);
    return ratio.toStringAsFixed(2);
  }

  // RFC3339 →「MM-dd HH:mm」（对齐 Web 站点明细更新时间）。
  String _fmtDate(String s) {
    if (s.isEmpty) {
      return '-';
    }
    final d = DateTime.tryParse(s);
    if (d == null) {
      return s;
    }
    String p(int n) => n.toString().padLeft(2, '0');
    return '${p(d.month)}-${p(d.day)} ${p(d.hour)}:${p(d.minute)}';
  }

  // RFC3339 →「YYYY-MM-DD HH:mm」（历史快照时间，对齐 Web fmtTime）。
  String _fmtTimeFull(String s) {
    if (s.isEmpty) {
      return '-';
    }
    final d = DateTime.tryParse(s);
    if (d == null) {
      return s;
    }
    String p(int n) => n.toString().padLeft(2, '0');
    return '${d.year}-${p(d.month)}-${p(d.day)} ${p(d.hour)}:${p(d.minute)}';
  }
}
