// 单站历史数据浮层（从 dashboard_page.dart 拆出，行为不变）：
// 宽屏表格 / 窄屏卡片自适应，对齐 Web 用户历史数据弹窗。
import 'package:flutter/material.dart';
import 'package:ptpatronus/api/dashboard.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';

class DashboardHistoryBody extends StatelessWidget {
  final List<SiteHistoryPoint> points;
  final bool loading;

  /// 非空 = 这次压根没读到，不能渲染成「暂无历史快照」。
  final String? error;

  /// 例如「该区间共 540 个快照，仅显示最近 500 个」。
  final String? note;
  final String Function(int) fmtBytes;
  final String Function(String) fmtTime;
  const DashboardHistoryBody({
    super.key,
    required this.points,
    required this.loading,
    this.error,
    this.note,
    required this.fmtBytes,
    required this.fmtTime,
  });

  @override
  Widget build(BuildContext context) {
   if (loading) {
     return const Padding(
       padding: EdgeInsets.symmetric(vertical: 40),
        child: PTLoadingView(),
     );
   }
    if (error != null) {
      return PTEmptyState(text: error!);
    }
    if (points.isEmpty) {
      return const PTEmptyState(text: '暂无历史快照');
    }
    final columns = <PTColumn<SiteHistoryPoint>>[
      PTColumn(
        label: '时间',
        flex: 2,
        cell: (_, p) =>
            Text(fmtTime(p.snapAt), style: const TextStyle(fontSize: 12)),
      ),
      PTColumn(
        label: '等级',
        flex: 1,
        cell: (_, p) => Text(
          p.level.isNotEmpty ? p.level : '-',
          style: const TextStyle(fontSize: 12),
        ),
      ),
      PTColumn(
        label: '上传',
        flex: 1,
        numeric: true,
        cell: (_, p) =>
            Text(fmtBytes(p.uploaded), style: const TextStyle(fontSize: 12)),
      ),
      PTColumn(
        label: '下载',
        flex: 1,
        numeric: true,
        cell: (_, p) =>
            Text(fmtBytes(p.downloaded), style: const TextStyle(fontSize: 12)),
      ),
      PTColumn(
        label: '分享率',
        flex: 1,
        numeric: true,
        cell: (_, p) => Text(
          p.ratio > 0 ? p.ratio.toStringAsFixed(2) : '-',
          style: const TextStyle(fontSize: 12),
        ),
      ),
      PTColumn(
        label: '做种',
        flex: 1,
        numeric: true,
        cell: (_, p) =>
            Text('${p.seeding}', style: const TextStyle(fontSize: 12)),
      ),
      PTColumn(
        label: '发布',
        flex: 1,
        numeric: true,
        cell: (_, p) => Text(
          p.uploads > 0 ? '${p.uploads}' : '-',
          style: const TextStyle(fontSize: 12),
        ),
      ),
      PTColumn(
        label: '魔力',
        flex: 1,
        numeric: true,
        cell: (_, p) => Text(
          p.bonus > 0 ? p.bonus.toStringAsFixed(0) : '-',
          style: const TextStyle(fontSize: 12),
        ),
      ),
    ];
    final table = SizedBox(
      width: double.infinity,
      child: PTAdaptiveTable<SiteHistoryPoint>(
        items: points,
        columns: columns,
        emptyText: '暂无历史快照',
        narrowCard: (_, p) => GlassCard(
          padding: const EdgeInsets.all(12),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                fmtTime(p.snapAt),
                style: const TextStyle(
                  fontWeight: FontWeight.w600,
                  fontSize: 13,
                ),
              ),
              const SizedBox(height: 6),
              Wrap(
                spacing: 12,
                runSpacing: 4,
                children: [
                  Text(
                    '等级 ${p.level.isNotEmpty ? p.level : '-'}',
                    style: TextStyle(fontSize: 12, color: context.dimText),
                  ),
                  Text(
                    '上传 ${fmtBytes(p.uploaded)}',
                    style: TextStyle(fontSize: 12, color: context.dimText),
                  ),
                  Text(
                    '下载 ${fmtBytes(p.downloaded)}',
                    style: TextStyle(fontSize: 12, color: context.dimText),
                  ),
                  Text(
                    '分享率 ${p.ratio > 0 ? p.ratio.toStringAsFixed(2) : '-'}',
                    style: TextStyle(fontSize: 12, color: context.dimText),
                  ),
                  Text(
                    '做种 ${p.seeding}',
                    style: TextStyle(fontSize: 12, color: context.dimText),
                  ),
                  Text(
                    '魔力 ${p.bonus > 0 ? p.bonus.toStringAsFixed(0) : '-'}',
                    style: TextStyle(fontSize: 12, color: context.dimText),
                  ),
                ],
              ),
            ],
          ),
        ),
      ),
    );
    if (note == null) return table;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(12, 8, 12, 0),
          child: Text(
            note!,
            style: TextStyle(fontSize: 12, color: context.dimText),
          ),
        ),
        Flexible(child: table),
      ],
    );
  }
}

