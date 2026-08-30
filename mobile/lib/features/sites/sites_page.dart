import 'dart:async';
import '../webview_login/webview_login_page.dart';

import 'package:flutter/material.dart';
import 'package:ptpatronus/api/site.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/utils/site_def_search.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';

part 'def_picker.dart';
part 'sites_checkin.dart';
part 'sites_form.dart';
part 'sites_rows.dart';
part 'sites_widgets.dart';

/// 站点页：宽屏数据表格（名称/定义/平台/地址/状态/操作）+ 窄屏卡片，对齐 Web Sites.vue 的 el-table。
/// cookie 走 WebView 登录（移动端更优）；表单也支持手填 cookie/账号/密码（对齐 Web 添加表单能力）。
class SitesPage extends StatefulWidget {
  const SitesPage({super.key});
  @override
  State<SitesPage> createState() => _SitesPageState();
}

class _SitesPageState extends State<SitesPage> with SitesCheckinMixin, SitesFormMixin {


  @override
  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final results = await Future.wait([
        SiteApi().list(),
        SiteApi().definitions(),
      ]);
      _sites = results[0] as List<Site>;
      _defs = results[1] as List<SiteDef>;
    } catch (e) {
      if (_sites.isEmpty) {
        _error = e.toString();
      } else {
        _toast(e.toString());
      }
    } finally {
      if (mounted) setState(() => _loading = false);
    }
    unawaited(_loadConnectivity());
  }

  // —— 签到记录：自适应浮层（宽屏对话框 / 窄屏底栏），对齐 Web el-drawer ——
  Future<void> _showRecords() async {
    List<CheckinRecord> recs;
    try {
      recs = await SiteApi().records();
    } catch (e) {
      _toast('加载签到记录失败：$e');
      return;
    }
    if (!mounted) return;
    await showAdaptiveSheet(
      context,
      title: '签到记录',
      wideWidth: 640,
      builder: (_) => recs.isEmpty
          ? const PTEmptyState(text: '暂无签到记录')
          : PTAdaptiveTable<CheckinRecord>(
              items: recs,
              columns: [
                PTColumn(
                  label: '时间',
                  flex: 2,
                  cell: (_, r) => Text(
                    _fmtRecordTime(r.createdAt),
                    style: const TextStyle(fontSize: 12),
                  ),
                ),
                PTColumn(
                  label: '站点',
                  flex: 1,
                  cell: (_, r) =>
                      Text(r.siteName, style: const TextStyle(fontSize: 12)),
                ),
                PTColumn(
                  label: '结果',
                  flex: 2,
                  cell: (_, r) => _recordResult(r),
                ),
                PTColumn(
                  label: '来源',
                  flex: 1,
                  cell: (_, r) => Text(
                    r.source == 'cron' ? '自动' : '手动',
                    style: const TextStyle(fontSize: 12),
                  ),
                ),
              ],
              emptyText: '暂无签到记录',
              narrowCard: (_, r) => GlassCard(
                padding: const EdgeInsets.all(12),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        Expanded(
                          child: Text(
                            '${r.siteName} · ${r.source == 'cron' ? '自动' : '手动'}',
                            style: const TextStyle(
                              fontWeight: FontWeight.w600,
                              fontSize: 13,
                            ),
                          ),
                        ),
                        Text(
                          _fmtRecordTime(r.createdAt),
                          style: TextStyle(
                            fontSize: 11,
                            color: context.dimText,
                          ),
                        ),
                      ],
                    ),
                    const SizedBox(height: 4),
                    Text(
                      r.success
                          ? (r.bonus > 0
                                ? '+${r.bonus} 魔力'
                                : (r.message.isNotEmpty ? r.message : '成功'))
                          : (r.error.isNotEmpty
                                ? r.error
                                : (r.message.isNotEmpty ? r.message : '失败')),
                      style: TextStyle(
                        fontSize: 12,
                        color: r.success ? PTColors.success : PTColors.danger,
                      ),
                    ),
                  ],
                ),
              ),
            ),
    );
  }


  @override
  Widget build(BuildContext context) {
    return PTPageScaffold(
      title: '站点',
      actions: [
        PTBarAction(icon: Icons.refresh, label: '刷新', onPressed: _load),
        PTBarAction(
          icon: Icons.add,
          label: '添加站点',
          onPressed: () => _showForm(),
        ),
        PTBarAction(
          icon: Icons.history,
          label: '签到记录',
          onPressed: _showRecords,
          overflow: true,
        ),
        PTBarAction(
          icon: Icons.card_giftcard,
          label: '一键签到',
          onPressed: _checkinAllLoading ? null : _checkinAll,
          overflow: true,
        ),
        if (_isAdmin)
          PTBarAction(
            icon: Icons.delete_sweep,
            label: '批量删除',
            onPressed: _batchDelete,
            overflow: true,
          ),
      ],
      body: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          if (_loading && _sites.isEmpty)
            const LinearProgressIndicator(minHeight: 2),
          Expanded(child: _listBody()),
        ],
      ),
    );
  }

}
