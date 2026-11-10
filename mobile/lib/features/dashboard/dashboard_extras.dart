// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// dashboard_extras.dart —— 移动端仪表盘「并发区」的失败口径（纯函数，便于用例）。
import '../../core/api_client.dart';
//
// 仪表盘一次并发拉 6 个区（工作流统计 / AI 统计 / 刷流任务 / 契约规则 / 通知 / 系统资源），
// 旧写法每个 `catch` 都吞掉：请求失败时卡片照旧渲染，
// 「0/0」「0 token」「暂无记录」和「真的没有」在界面上完全同形。

/// 六个并发区的名字：取数处记录、渲染处读取，用常量避免两处字面量漂移。
const extrasSectionWorkflow = '工作流';
const extrasSectionAgent = 'AI';
const extrasSectionBrush = '刷流';
const extrasSectionContract = '契约';
const extrasSectionNotice = '通知';
const extrasSectionMetrics = '系统资源';

/// 管理员专属区：非管理员不是「读失败」，而是本来就没权限，催他刷新没有意义。
const extrasAdminOnly = '仅管理员可见';

/// 后端把没权限也当错误抛出（403 / admin_required）。这类不能记进失败账本：
/// 界面对它的正确说法是「仅管理员可见」，而不是「读取失败，请下拉刷新重试」。
bool extrasForbidden(Object e) =>
    e is ApiException && (e.status == 403 || e.code == 'admin_required');

/// 某个区的取数结果是否可用（失败时卡片该说「加载失败」，而不是补一个 0）。
bool extrasAvailable(List<String> failed, String section) =>
    !failed.contains(section);

/// 卡片数值文案：源读不出时给出可读的失败说明，绝不落到 0。
String extrasValue({
  required List<String> failed,
  required String section,
  required String value,
  String unavailable = '加载失败',
}) {
  return extrasAvailable(failed, section) ? value : unavailable;
}

/// 每站统计读取失败的顶栏账面：后端在读失败时仍回 200，只在响应里带
/// `stat_read_error`（dashboard.go 的 StatReadError）。这条与「读了但站点真的没数据」
/// 是两件事——不说清，手机上就是一屏 0 上传/0 下载，看着像大家的数据都没了。
/// 原因文本一律用后端给的，界面不自己编（同 Web 的 DashboardHealthStrip 口径）。
String statReadNotice(String? reason) {
  if (reason == null || reason.isEmpty) return '';
  return '每站统计读取失败：$reason，下面各站的 0 值是这次没读到，不是站点掉数据';
}

/// 顶栏提示：把失败的区逐个点名，让用户知道「看到的不是全部」。
String extrasNotice(List<String> failed) {
  if (failed.isEmpty) return '';
  return '部分数据加载失败：${failed.join('、')}（下方数字不是全部，请下拉刷新重试）';
}
