// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// _DashboardPageState 的单站卡组：站点卡/告警卡/连通性/等级标签/行操作。
// （二次拆分；行为不变。）
part of 'package:ptpatronus/features/dashboard/dashboard_page.dart';

extension on _DashboardPageState {
  // 窄屏站点卡：站点名 + 等级 + 分享率 + 三行统计 + 加入/更新 + 操作（历史/刷新）。
  Widget _siteStatCard(BuildContext context, SiteStatItem s) {
    Widget cell(String label, String value) =>
        Expanded(child: _statCell(label, value));
    final metrics = <MapEntry<String, String>>[
      if (_config.showUpload) MapEntry('上传', _fmtBytes(s.uploaded)),
      if (_config.showDownload) MapEntry('下载', _fmtBytes(s.downloaded)),
      if (_config.showSeedingCount) MapEntry('做种', '${s.seeding}'),
      if (_config.showSeedingVolume)
        MapEntry('做种体积', s.seedingSize > 0 ? _fmtBytes(s.seedingSize) : '-'),
      if (_config.showUploadCount)
        MapEntry('发布', s.uploads > 0 ? '${s.uploads}' : '-'),
      MapEntry('下载中', '${s.leeching}'),
      if (_config.showBonus)
        MapEntry('魔力', s.bonus > 0 ? _fmtCompact(s.bonus) : '-'),
      if (_config.showBonusPerHour)
        MapEntry(
          '时魔/h',
          s.bonusPerHour > 0 ? _fmtCompact(s.bonusPerHour) : '-',
        ),
      if (_config.showRatio) MapEntry('分享率', _fmtRatio(s.ratio)),
    ];
    Widget row(List<MapEntry<String, String>> items) => Padding(
      padding: const EdgeInsets.only(bottom: 6),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          for (final item in items) cell(item.key, item.value),
          for (var i = items.length; i < 3; i++)
            const Expanded(child: SizedBox()),
        ],
      ),
    );
    return GlassCard(
      padding: const EdgeInsets.fromLTRB(12, 10, 10, 10),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Expanded(
                child: Text(
                  _displaySiteName(s).isEmpty ? '站点信息已隐藏' : _displaySiteName(s),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    fontWeight: FontWeight.w700,
                    fontSize: 14,
                  ),
                ),
              ),
              _connectivityPill(s),
              const SizedBox(width: 6),
              if (_config.showUsername && s.level.isNotEmpty)
                _levelTag(_config.blurUserInfo ? '***' : s.level),
              const SizedBox(width: 6),
              if (_config.showRatio)
                Text(
                  '分享率 ${_fmtRatio(s.ratio)}',
                  style: TextStyle(
                    color: s.ratio >= 1 ? PTColors.success : PTColors.danger,
                    fontSize: 11,
                    fontWeight: FontWeight.w600,
                  ),
                ),
            ],
          ),
          if (_config.showUsername) ...[
            const SizedBox(height: 3),
            Text(
              _displayUserIdentity(s),
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(color: PTColors.primary, fontSize: 11),
            ),
          ],
          const SizedBox(height: 8),
          for (final chunk in metrics.take(9).toList().chunked(3)) row(chunk),
          Row(
            children: [
              Expanded(
                child: Text(
                  '加入 ${s.joinDate.isNotEmpty ? s.joinDate : '-'} · 更新 ${_fmtDate(s.updatedAt)}',
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(color: context.dimText, fontSize: 11),
                ),
              ),
              const SizedBox(width: 8),
              _rowActions(context, s),
            ],
          ),
        ],
      ),
    );
  }

  // 告警卡：Cookie 失效 / H&R 危险（最高优先级，红色左边框，对齐 Web .alert-danger）。
  Widget _alertCard(AlertKind kind) {
    final isCookie = kind == AlertKind.cookie;
    final list = isCookie ? _cookieAlerts : <String>[];
    return GlassCard(
      padding: const EdgeInsets.fromLTRB(14, 14, 14, 14),
      tint: PTColors.danger,
      child: DecoratedBox(
        decoration: BoxDecoration(
          border: Border(left: BorderSide(color: PTColors.danger, width: 3)),
        ),
        child: Padding(
          padding: const EdgeInsets.only(left: 10),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  Icon(
                    isCookie
                        ? Icons.error_outline
                        : Icons.warning_amber_rounded,
                    color: PTColors.danger,
                    size: 20,
                  ),
                  const SizedBox(width: 6),
                  Text(
                    isCookie
                        ? 'Cookie 失效：${list.length} 个站点'
                        : 'H&R 告警：${_hrAlerts.length} 条需关注',
                    style: const TextStyle(
                      fontWeight: FontWeight.w700,
                      color: PTColors.danger,
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 6),
              if (isCookie) ...[
                Text(
                  '以下站点 cookie 已失效，签到/统计/H&R 自动化已停摆，请尽快重新登录：',
                  style: TextStyle(color: context.dimText, fontSize: 12),
                ),
                const SizedBox(height: 8),
                Wrap(
                  spacing: 6,
                  runSpacing: 6,
                  children: list
                      .map(
                        (s) => Chip(
                          label: Text(s),
                          labelStyle: const TextStyle(
                            color: Colors.white,
                            fontSize: 12,
                          ),
                          backgroundColor: PTColors.danger,
                          visualDensity: VisualDensity.compact,
                          padding: EdgeInsets.zero,
                        ),
                      )
                      .toList(),
                ),
              ] else ...[
                ..._hrAlerts
                    .take(5)
                    .map(
                      (a) => Padding(
                        padding: const EdgeInsets.only(top: 6),
                        child: Row(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Container(
                              margin: const EdgeInsets.only(top: 6),
                              width: 6,
                              height: 6,
                              decoration: const BoxDecoration(
                                color: Colors.redAccent,
                                shape: BoxShape.circle,
                              ),
                            ),
                            const SizedBox(width: 8),
                            Expanded(
                              child: Column(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                children: [
                                  Text(
                                    a.title,
                                    maxLines: 1,
                                    overflow: TextOverflow.ellipsis,
                                    style: const TextStyle(
                                      fontWeight: FontWeight.w600,
                                      fontSize: 13,
                                    ),
                                  ),
                                  Text(
                                    '${a.site} · ${a.status}${a.deadline.isNotEmpty ? ' · 剩 ${a.deadline}' : ''}',
                                    style: TextStyle(
                                      color: context.dimText,
                                      fontSize: 11,
                                    ),
                                  ),
                                ],
                              ),
                            ),
                          ],
                        ),
                      ),
                    ),
                if (_hrAlerts.length > 5)
                  Padding(
                    padding: const EdgeInsets.only(top: 6),
                    child: Text(
                      '…及其它 ${_hrAlerts.length - 5} 条',
                      style: TextStyle(color: context.dimText, fontSize: 11),
                    ),
                  ),
              ],
            ],
          ),
        ),
      ),
    );
  }

  Widget _connectivityPill(SiteStatItem site) {
    final status = _connectivity[site.siteId];
    final pending = _connectivityLoading;
    final state = status?.state;
    // #403 「读不到」与「真没检测过」必须分开。
    //
    // 从前 status == null 一律显示「未检测」，于是 _loadConnectivity 读失败时
    // （静默回 {}）整页显示一片「未检测」—— 与「这些站真没检测过」完全同形。
    // 而这一页正是用户判断站点是否掉线的依据；显示成「未检测」时用户会以为是
    // 站点从没测过、于是去站点页逐个重测，而真相是「这次没读到连通状态」。
    //
    // 读失败时文案说「没读到」并挂上原因，且颜色不用 dimText（那是正常态的配色）。
    final readFailed = !pending && status == null && _connectivityError != null;
    final label = pending
        ? '检测中'
        : readFailed
        ? '未读到'
        : status == null
        ? '未检测'
        : state == SiteConnectivityState.connected
        ? '已联通'
        : state == SiteConnectivityState.disabled
        ? '已停用'
        : '未联通';
    final color = pending || status == null
        ? (readFailed ? PTColors.warn : context.dimText)
        : state == SiteConnectivityState.connected
        ? PTColors.success
        : state == SiteConnectivityState.disabled
        ? context.dimText
        : PTColors.danger;
    final child = Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
      decoration: BoxDecoration(
        color: color.withValues(alpha: pending || status == null ? 0.1 : 0.16),
        borderRadius: BorderRadius.circular(999),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            width: 7,
            height: 7,
            decoration: BoxDecoration(color: color, shape: BoxShape.circle),
          ),
          const SizedBox(width: 5),
          Text(
            label,
            style: TextStyle(
              fontSize: 11,
              color: color,
              fontWeight: FontWeight.w600,
            ),
          ),
        ],
      ),
    );
    // #403 读失败时也要能点开看原因：`status == null` 从前直接 return child（不可点），
    // 于是「这次没读到连通状态」这一档既显示成「未检测」、又点不出原因 ——
    // 两个症状叠在一起，用户彻底无从判断发生了什么。
    if (readFailed) {
      return GestureDetector(
        onTap: () => _showConnectivityReadError(site),
        child: child,
      );
    }
    if (pending || status == null || state == SiteConnectivityState.connected) {
      return child;
    }
    return GestureDetector(
      onTap: () => _showConnectivityReason(site),
      child: child,
    );
  }

  Widget _levelTag(String level) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
      decoration: BoxDecoration(
        color: const Color(0x286EA8FE),
        borderRadius: BorderRadius.circular(10),
      ),
      child: Text(
        level.isNotEmpty ? level : '-',
        style: const TextStyle(color: Color(0xFF6EA8FE), fontSize: 11),
      ),
    );
  }

  Widget _rowActions(BuildContext context, SiteStatItem s) {
    final refreshing = _refreshing.contains(s.siteId);
    final wide = isWideScreen(context);
    // 显式 40×40 命中区 + 零 padding：裸 IconButton 默认 48×48（visualDensity:compact 并不可靠地压到 40），
    // 两枚并排放进表格操作列(flex:1)会溢出→黄黑斜纹。40 在桌面表格（鼠标场景）够用且两枚=80px<列宽。
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        IconButton(
          tooltip: '用户历史数据',
          visualDensity: VisualDensity.compact,
          iconSize: wide ? 18 : 17,
          padding: EdgeInsets.zero,
          constraints: BoxConstraints(
            minWidth: wide ? 40 : 34,
            minHeight: wide ? 40 : 34,
          ),
          onPressed: () => _viewHistory(s),
          icon: Icon(Icons.history, color: PTColors.primary2),
        ),
        IconButton(
          tooltip: refreshing ? '刷新中…' : '刷新数据',
          visualDensity: VisualDensity.compact,
          iconSize: wide ? 18 : 17,
          padding: EdgeInsets.zero,
          constraints: BoxConstraints(
            minWidth: wide ? 40 : 34,
            minHeight: wide ? 40 : 34,
          ),
          onPressed: refreshing ? null : () => _syncOne(s),
          icon: refreshing
              ? const SizedBox(
                  width: 16,
                  height: 16,
                  child: CircularProgressIndicator(strokeWidth: 2),
                )
              : const Icon(Icons.refresh, color: PTColors.success),
        ),
      ],
    );
  }
}
