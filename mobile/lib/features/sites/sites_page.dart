// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'dart:async';
import '../webview_login/webview_login_page.dart';

import 'package:flutter/material.dart';
import 'package:ptpatronus/api/site.dart';
import 'package:ptpatronus/core/api_client.dart';
// #404 任意异常 → 用户可读文案（后端每个错误码都带着可操作的中文，别在 catch 里丢掉）。
import 'package:ptpatronus/core/err_text.dart';
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
  // 站点定义诊断（后端四类「配置被静默忽略」的账面）：读失败与「真的没问题」分开存。
  List<DefinitionDiagnostic> _defDiags = const [];
  String _defDiagsErr = '';

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
        _error = errTextOf(e) ?? '加载失败，请检查网络';
      } else {
        _toast(e.toString());
      }
    } finally {
      if (mounted) setState(() => _loading = false);
    }
    unawaited(_loadConnectivity());
    unawaited(_loadDefDiags());
  }

  /// 站点定义诊断单独一路读：不进 `_load()` 的 `Future.wait`，
  /// 否则这一路失败会把站点列表/定义一起拖没（本仓同族缺陷 #206/#279 的形状）。
  Future<void> _loadDefDiags() async {
    final res = await SiteApi().definitionDiagnostics();
    if (!mounted) return;
    setState(() {
      _defDiags = res.items;
      _defDiagsErr = res.readError;
    });
  }

  static String _defDiagKindLabel(String kind) => switch (kind) {
        'bad_regexp' => '正则写法编译不过',
        'unknown_key' => '有不认识的配置键',
        'duplicate_id' => '站点 id 撞车',
        'skipped_subdir' => '子目录没被读取',
        'load_failed' => '这份定义没被加载',
        // 后端将来新增一类时不许把裸 kind 当文案糊上去（对齐 Web 的 diagKind.other）。
        _ => '其它配置问题',
      };

  /// 诊断条：没问题就不占位；读失败说「读取失败+原因」，绝不能显示成「一切正常」。
  Widget _defDiagBanner() {
    if (_defDiags.isEmpty && _defDiagsErr.isEmpty) return const SizedBox.shrink();
    final shown = _defDiags.take(4).toList();
    return Material(
      color: const Color(0x14FFB020),
      child: Padding(
        padding: const EdgeInsets.fromLTRB(12, 8, 12, 8),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                const Icon(Icons.warning_amber_rounded, size: 16, color: Color(0xFFB26A00)),
                const SizedBox(width: 6),
                Expanded(
                  child: Text(
                    _defDiags.isNotEmpty
                        ? '站点定义里有 ${_defDiags.length} 条配置没有生效'
                        : '站点定义诊断读取失败：$_defDiagsErr',
                    style: const TextStyle(fontSize: 12),
                  ),
                ),
              ],
            ),
            for (final d in shown)
              Padding(
                padding: const EdgeInsets.only(left: 22, top: 2),
                child: Text(
                  '${_defDiagKindLabel(d.kind)} · ${d.target.isEmpty ? '—' : d.target}：${d.detail}',
                  style: const TextStyle(fontSize: 12, color: Colors.black87),
                ),
              ),
            if (_defDiags.length > shown.length)
              Padding(
                padding: const EdgeInsets.only(left: 22, top: 2),
                child: Text('另有 ${_defDiags.length - shown.length} 条未列出',
                    style: const TextStyle(fontSize: 12)),
              ),
          ],
        ),
      ),
    );
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
                            fontSize: 12,
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
          _defDiagBanner(),
          if (_loading && _sites.isEmpty)
            const LinearProgressIndicator(minHeight: 2),
          Expanded(child: _listBody()),
        ],
      ),
    );
  }

}
