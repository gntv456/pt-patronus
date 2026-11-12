// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'dart:async';
import 'package:flutter/material.dart';
import 'package:ptpatronus/api/assessment.dart';
import 'package:ptpatronus/features/common/stream_account.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_filter_sheet.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';
import 'package:ptpatronus/core/err_text.dart';

class AssessmentPage extends StatefulWidget {
  const AssessmentPage({super.key, this.frames});
  /// 帧源注入缝（同勋章墙）：done 帧账面要吸收成 _error / _streamBanner 才在界面上证得了。
  final StreamFrames? frames;
  @override
  State<AssessmentPage> createState() => _AssessmentPageState();
}

class _AssessmentPageState extends State<AssessmentPage> {
  late final _api = AssessmentApi(frames: widget.frames);
  StreamSubscription<StreamBatch<AssessmentCard>>? _sub; // 流式首屏订阅（切换/刷新时 cancel）
  List<AssessmentCard> _cards = [];
  bool _loading = true;
  String? _error;
  String? _streamBanner; // done 帧账面：已有数据时的顶部常驻说明
  String _filter = 'all';
  List<int> _selectedSiteIds = []; // 站点多选筛选，空=全部

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

  Future<void> _load({bool refresh = false}) {
    _sub?.cancel();
    setState(() {
      _cards = [];
      _loading = true;
      _error = null;
      _streamBanner = null;
    });
    _sub = _api.allStream(refresh: refresh).listen(
      (batch) {
        if (!mounted) return;
        setState(() {
          _cards = [..._cards, ...batch.items];
          // 一张卡都没到却说清单读失败：别说成「没有考核」。
          final notice = streamNoticeOf(
            arrived: _cards.length,
            listError: batch.listError,
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
          _error = errTextOf(e) ?? '加载失败，请检查网络';
        });
      },
    );
    return Future.value();
  }

  List<AssessmentCard> get _filtered {
    var list = _cards;
    if (_filter != 'all') {
      list = list.where((c) => c.status.toLowerCase() == _filter).toList();
    }
    if (_selectedSiteIds.isNotEmpty) {
      list = list.where((c) => _selectedSiteIds.contains(c.siteId)).toList();
    }
    return list;
  }

  // 站点筛选候选项（从结果去重 siteId/siteName）。
  List<PTFilterOption<Object?>> get _siteOptions {
    final seen = <int, String>{};
    for (final c in _cards) {
      seen.putIfAbsent(c.siteId, () => c.siteName);
    }
    return seen.entries
        .map((e) => PTFilterOption<Object?>(value: e.key, label: e.value))
        .toList();
  }

  Color _statusColor(String s) => switch (s) {
    'SAFE' => const Color(0xFF2ECC71),
    'PROGRESS' => const Color(0xFF3498DB),
    'DANGER' => const Color(0xFFF56C6C),
    _ => const Color(0xFF95A5A6),
  };

  int _count(String s) =>
      _cards.where((c) => c.status.toLowerCase() == s).length;

  @override
  Widget build(BuildContext context) {
    return PTPageScaffold(
      title: '站点考核',
      actions: [
        PTBarAction(
          icon: Icons.refresh,
          label: '刷新',
          loading: _loading,
          onPressed: _loading ? null : () => _load(refresh: true),
        ),
      ],
      filterDimensions: _filterDimensions(),
      siteFilter: _cards.isEmpty
          ? null
          : PTSiteFilterConfig(
              selected: _selectedSiteIds,
              onChanged: (v) => setState(() => _selectedSiteIds = v),
              options: _siteOptions,
            ),
      filterOnClearAll: () => setState(() {
        _filter = 'all';
        _selectedSiteIds = [];
      }),
      body: _loading && _cards.isEmpty
          ? const PTLoadingView()
          : _error != null
          ? PTEmptyState(text: _error!, onRetry: _load)
          : _filtered.isEmpty
          ? PTEmptyState(text: '该筛选下无站点')
          : RefreshIndicator(
              onRefresh: () => _load(refresh: true),
              child: Column(
                children: [
                  StreamNoticeBanner(text: _streamBanner),
                  Expanded(
                    child: PTMasonry<AssessmentCard>(
                      items: _filtered,
                      minItemWidth: 320,
                      itemBuilder: (_, c) => _card(c),
                    ),
                  ),
                ],
              ),
            ),
    );
  }

  List<PTFilterDimension> _filterDimensions() {
    return [
      PTFilterDimension.choice(
        label: '状态',
        selected: _filter,
        isDefault: _filter == 'all',
        onChanged: (v) => setState(() => _filter = (v as String?) ?? 'all'),
        options: [
          PTFilterOption<Object?>(
            value: 'all',
            label: '全部',
            count: _cards.length,
          ),
          PTFilterOption<Object?>(
            value: 'safe',
            label: '安全',
            count: _count('safe'),
          ),
          PTFilterOption<Object?>(
            value: 'progress',
            label: '进行',
            count: _count('progress'),
          ),
          PTFilterOption<Object?>(
            value: 'danger',
            label: '危险',
            count: _count('danger'),
          ),
          PTFilterOption<Object?>(
            value: 'exempt',
            label: '免考',
            count: _count('exempt'),
          ),
        ],
      ),
    ];
  }

  Widget _card(AssessmentCard c) {
    final color = _statusColor(c.status);
    return Container(
      decoration: BoxDecoration(
        border: Border(left: BorderSide(color: color, width: 3)),
      ),
      child: GlassCard(
        padding: const EdgeInsets.all(14),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Expanded(
                  child: Text(
                    c.siteName,
                    style: const TextStyle(fontWeight: FontWeight.w600),
                  ),
                ),
                Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 8,
                    vertical: 2,
                  ),
                  decoration: BoxDecoration(
                    color: color.withValues(alpha: 0.18),
                    borderRadius: BorderRadius.circular(999),
                  ),
                  child: Text(
                    c.status,
                    style: TextStyle(
                      color: color,
                      fontSize: 11,
                      fontWeight: FontWeight.w600,
                    ),
                  ),
                ),
              ],
            ),
            if (c.error.isNotEmpty)
              Padding(
                padding: const EdgeInsets.only(top: 6),
                child: Text(
                  c.error,
                  style: const TextStyle(
                    color: Color(0xFFF56C6C),
                    fontSize: 13,
                  ),
                ),
              )
            else ...[
              const SizedBox(height: 10),
              ClipRRect(
                borderRadius: BorderRadius.circular(4),
                child: LinearProgressIndicator(
                  value: c.progressPercent / 100,
                  minHeight: 8,
                  backgroundColor: Colors.black12,
                  valueColor: AlwaysStoppedAnimation(color),
                ),
              ),
              const SizedBox(height: 4),
              Row(
                children: [
                  Text(
                    '${c.progressPercent.toStringAsFixed(0)}%',
                    style: TextStyle(color: context.dimText, fontSize: 12),
                  ),
                  const Spacer(),
                  if (c.endTime != null) ...[
                    Text(
                      '截止 ${_fmtDate(c.endTime)}',
                      style: TextStyle(color: context.dimText, fontSize: 12),
                    ),
                    const SizedBox(width: 8),
                  ],
                  if (c.remainingDays >= 0)
                    Text(
                      '剩 ${c.remainingDays} 天',
                      style: TextStyle(color: context.dimText, fontSize: 12),
                    ),
                ],
              ),
              if (c.assessmentType.isNotEmpty)
                Padding(
                  padding: const EdgeInsets.only(top: 4),
                  child: Text(
                    c.assessmentType,
                    style: const TextStyle(
                      color: Color(0xFF6C5CE7),
                      fontSize: 12,
                    ),
                  ),
                ),
              if (c.description.isNotEmpty)
                Padding(
                  padding: const EdgeInsets.only(top: 4),
                  child: Text(
                    c.description,
                    style: TextStyle(
                      color: context.dimText,
                      fontSize: 12,
                      height: 1.4,
                    ),
                  ),
                ),
              if (c.metrics.isNotEmpty)
                Padding(
                  padding: const EdgeInsets.only(top: 8),
                  child: Column(
                    children: c.metrics
                        .map(
                          (m) => Padding(
                            padding: const EdgeInsets.only(top: 4),
                            child: Row(
                              children: [
                                Expanded(
                                  child: Text(
                                    m.label,
                                    style: const TextStyle(fontSize: 13),
                                  ),
                                ),
                                Text(
                                  '${m.current.toStringAsFixed(1)}/${m.required.toStringAsFixed(1)} ${m.unit}',
                                  style: TextStyle(
                                    color: context.dimText,
                                    fontSize: 12,
                                  ),
                                ),
                                const SizedBox(width: 8),
                                Text(
                                  m.isCompleted ? '✓' : '✗',
                                  style: TextStyle(
                                    color: m.isCompleted
                                        ? const Color(0xFF2ECC71)
                                        : const Color(0xFFF56C6C),
                                  ),
                                ),
                              ],
                            ),
                          ),
                        )
                        .toList(),
                  ),
                ),
            ],
          ],
        ),
      ),
    );
  }

  String _fmtDate(String? t) {
    if (t == null || t.isEmpty) return '—';
    final s = t.replaceAll('T', ' ');
    return s.length > 10 ? s.substring(0, 10) : s;
  }
}
