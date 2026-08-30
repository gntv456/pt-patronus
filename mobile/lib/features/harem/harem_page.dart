import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:ptpatronus/api/harem.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_filter_sheet.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';

part 'haremopsmixin.dart';
part 'harem_widgets.dart';

/// 容错字节格式化（B/KB/MB/GB/TB/PB）；<=0 返回 '—'（替代原硬编码 G，大上传者不再显示 5120G——审计 P2）。
String _fmtBytes(int b) {
  if (b <= 0) return '—';
  const u = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  double n = b.toDouble();
  int i = 0;
  while (n >= 1024 && i < u.length - 1) {
    n /= 1024;
    i++;
  }
  return '${n.toStringAsFixed(i <= 1 ? 0 : 2)} ${u[i]}';
}

/// 后宫页（对齐 Web Harem.vue）：概览指标条 + 处方列表。
/// 处方宽屏=表格 / 窄屏=卡片；花名册与导出走自适应浮层（宽屏对话框 / 窄屏底栏），
/// 花名册含上传+下载两列；导出支持复制。对齐 Web 的 el-table + el-dialog/el-drawer。
class HaremPage extends StatefulWidget {
  const HaremPage({super.key});
  @override
  State<HaremPage> createState() => _HaremPageState();
}

class _HaremPageState extends State<HaremPage> with HaremOpsMixin {

  @override
  Future<void> _export() async {
    String text;
    try {
      text = await _api.exportText();
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text('$e')));
      }
      return;
    }
    if (!mounted) return;
    await showAdaptiveSheet(
      context,
      title: '导出文本',
      wideWidth: 640,
      builder: (_) => Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Align(
            alignment: Alignment.centerRight,
            child: TextButton.icon(
              onPressed: () async {
                await Clipboard.setData(ClipboardData(text: text));
                if (!mounted) return;
                ScaffoldMessenger.of(
                  context,
                ).showSnackBar(const SnackBar(content: Text('已复制')));
              },
              icon: const Icon(Icons.copy, size: 16),
              label: const Text('复制'),
            ),
          ),
          SelectableText(
            text,
            style: const TextStyle(
              fontFamily: 'monospace',
              fontSize: 13,
              height: 1.5,
            ),
          ),
        ],
      ),
    );
  }

  @override
  Widget _overview(HaremOverview ov) {
    // 单行横向滚动指标条，取代 7 个 width:84 双行瓦片的折行 Wrap。
    return PTStatStrip(
      items: [
        PTStat(value: '${ov.totalSites}', label: '站点'),
        PTStat(value: '${ov.totalInvitees}', label: '成员'),
        PTStat(value: '${ov.totalPermInvites}', label: '永邀'),
        PTStat(value: '${ov.totalTempInvites}', label: '临邀'),
        PTStat(value: '${ov.totalLowRatio}', label: '低分享率'),
        PTStat(value: '${ov.totalBanned}', label: '已禁用'),
        PTStat(value: '${ov.totalNoData}', label: '无数据'),
      ],
    );
  }

  @override
  Widget _prescription(PrescriptionItem p) {
    return GlassCard(
      padding: const EdgeInsets.all(14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Expanded(
                child: Text(
                  p.siteName,
                  style: const TextStyle(
                    fontWeight: FontWeight.w600,
                    fontSize: 15,
                  ),
                ),
              ),
              Text('现药 ${p.remain}', style: const TextStyle(fontSize: 13)),
              const SizedBox(width: 10),
              Text('可买 ${p.canBuy}', style: const TextStyle(fontSize: 13)),
              const SizedBox(width: 10),
              Text(
                p.canInvite ? '权限 ✓' : '权限 ✗',
                style: TextStyle(
                  color: p.canInvite ? PTColors.success : PTColors.danger,
                  fontSize: 13,
                ),
              ),
            ],
          ),
          const SizedBox(height: 6),
          if (p.error.isNotEmpty)
            Text(
              p.error,
              style: const TextStyle(color: PTColors.danger, fontSize: 13),
            )
          else
            Text(
              '后宫 ${p.haremCount} 人${p.deadCount > 0 ? "（禁用 ${p.deadCount}）" : ""}',
              style: TextStyle(color: context.dimText, fontSize: 13),
            ),
          if (p.invitees.isNotEmpty)
            Align(
              alignment: Alignment.centerLeft,
              child: TextButton(
                onPressed: () => _showInvitees(p),
                child: Text('查看花名册 (${p.invitees.length})'),
              ),
            ),
        ],
      ),
    );
  }

  @override
  void _showInvitees(PrescriptionItem p) {
    showAdaptiveSheet(
      context,
      title: '${p.siteName} 后宫成员',
      wideWidth: 760,
      builder: (_) => PTAdaptiveTable<InviteeInfo>(
        items: p.invitees,
        columns: [
          PTColumn<InviteeInfo>(
            label: '用户',
            flex: 2,
            cell: (_, iv) {
              final dead = iv.status == 'disabled';
              return Text(
                iv.username,
                style: TextStyle(
                  fontWeight: FontWeight.w600,
                  fontSize: 13,
                  color: dead ? PTColors.danger : null,
                ),
              );
            },
          ),
          PTColumn<InviteeInfo>(
            label: '状态',
            flex: 1,
            cell: (_, iv) => Text(
              iv.status,
              style: TextStyle(
                fontSize: 12,
                color: iv.status == 'disabled' ? PTColors.danger : context.dimText,
              ),
            ),
          ),
          PTColumn<InviteeInfo>(
            label: '上传',
            flex: 1,
            numeric: true,
            cell: (_, iv) => Text(
              _fmtBytes(iv.uploaded),
              style: const TextStyle(fontSize: 12),
            ),
          ),
          PTColumn<InviteeInfo>(
            label: '下载',
            flex: 1,
            numeric: true,
            cell: (_, iv) => Text(
              _fmtBytes(iv.downloaded),
              style: const TextStyle(fontSize: 12),
            ),
          ),
          PTColumn<InviteeInfo>(
            label: '分享率',
            flex: 1,
            cell: (_, iv) =>
                Text(iv.ratio, style: const TextStyle(fontSize: 12)),
          ),
        ],
        narrowCard: (_, iv) {
          final dead = iv.status == 'disabled';
          return GlassCard(
            padding: const EdgeInsets.all(12),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  iv.username,
                  style: TextStyle(
                    fontWeight: FontWeight.w600,
                    fontSize: 14,
                    color: dead ? PTColors.danger : null,
                  ),
                ),
                const SizedBox(height: 4),
                Text(
                  '状态 ${iv.status} · 分享率 ${iv.ratio}',
                  style: TextStyle(fontSize: 12, color: context.dimText),
                ),
                const SizedBox(height: 2),
                Text(
                  '↑ ${_fmtBytes(iv.uploaded)}   ↓ ${_fmtBytes(iv.downloaded)}',
                  style: TextStyle(fontSize: 12, color: context.dimText),
                ),
              ],
            ),
          );
        },
        emptyText: '暂无成员',
        embedded: true,
      ),
    );
  }
  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _sub?.cancel();
    super.dispose();
  }


}
