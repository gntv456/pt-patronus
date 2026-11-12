// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter/material.dart';

import 'package:ptpatronus/api/media_quality.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/glass_icon.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';
import 'package:ptpatronus/core/err_text.dart';

/// 媒体质量页：多评分订阅质量闸门配置 + 评分聚合预览。
class MediaQualityPage extends StatefulWidget {
  const MediaQualityPage({super.key});

  @override
  State<MediaQualityPage> createState() => _MediaQualityPageState();
}

class _MediaQualityPageState extends State<MediaQualityPage> {
  final _api = MediaQualityApi();
  late MediaQualityConfig _cfg = MediaQualityConfig(
    enabled: false,
    minScore: 6,
    minVotes: 100,
    minReleaseDays: 7,
    sources: const ['tmdb', 'douban', 'imdb', 'bangumi'],
  );
  bool _loading = true;
  bool _saving = false;
  String? _error;

  // 评分预览
  final _titleCtrl = TextEditingController();
  final _yearCtrl = TextEditingController();
  String _type = '';
  RatingSummary? _summary;
  bool _previewing = false;

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _titleCtrl.dispose();
    _yearCtrl.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      _cfg = await _api.getConfig();
      if (_cfg.sources.isEmpty) {
        _cfg = MediaQualityConfig(
          enabled: _cfg.enabled,
          minScore: _cfg.minScore,
          minVotes: _cfg.minVotes,
          minReleaseDays: _cfg.minReleaseDays,
          sources: const ['tmdb', 'douban', 'imdb', 'bangumi'],
        );
      }
    } catch (e) {
      _error = errTextOf(e) ?? '加载失败，请检查网络';
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _save() async {
    setState(() => _saving = true);
    try {
      await _api.setConfig(_cfg);
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('已保存')));
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('保存失败：$e')));
      }
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  Future<void> _preview() async {
    final title = _titleCtrl.text.trim();
    if (title.isEmpty) return;
    setState(() {
      _previewing = true;
      _summary = null;
    });
    try {
      final year = int.tryParse(_yearCtrl.text.trim()) ?? 0;
      _summary = await _api.rating(title, year: year, type: _type);
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('查询失败：$e')));
      }
    } finally {
      if (mounted) setState(() => _previewing = false);
    }
  }

  void _toggleSource(String value, bool on) {
    setState(() {
      final set = _cfg.sources.toSet();
      on ? set.add(value) : set.remove(value);
      _cfg = MediaQualityConfig(
        enabled: _cfg.enabled,
        minScore: _cfg.minScore,
        minVotes: _cfg.minVotes,
        minReleaseDays: _cfg.minReleaseDays,
        sources: set.toList(),
      );
    });
  }

  @override
  Widget build(BuildContext context) {
    return PTPageScaffold(
      title: '媒体质量',
      actions: [
        PTBarAction(
          icon: Icons.save_outlined,
          label: '保存',
          loading: _saving,
          onPressed: _saving ? null : _save,
        ),
      ],
      body: _loading
          ? const PTLoadingView()
          : _error != null
              ? PTEmptyState(text: _error, onRetry: _load)
              : RefreshIndicator(
                  onRefresh: _load,
                  child: ListView(padding: const EdgeInsets.all(PTSp.s4), children: [
                    _configCard(),
                    const SizedBox(height: PTSp.s4),
                    _previewCard(),
                  ]),
                ),
    );
  }

  Widget _configCard() => GlassCard(
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          const Text('质量闸门', style: TextStyle(fontSize: PTFs.lg, fontWeight: FontWeight.w700)),
          const SizedBox(height: PTSp.s3),
          _switchRow('启用订阅闸门', _cfg.enabled, (v) => setState(() => _cfg = _cfgCopyWith(enabled: v))),
          const SizedBox(height: PTSp.s2),
          _numRow('最低综合评分', _cfg.minScore, 0, 10, 0.1, (v) => setState(() => _cfg = _cfgCopyWith(minScore: v))),
          _numRowInt('最低投票数', _cfg.minVotes.toDouble(), 0, 1000000, 50, (v) => setState(() => _cfg = _cfgCopyWith(minVotes: v.toInt()))),
          _numRowInt('上映满 N 天', _cfg.minReleaseDays.toDouble(), 0, 365, 1, (v) => setState(() => _cfg = _cfgCopyWith(minReleaseDays: v.toInt()))),
          const SizedBox(height: PTSp.s2),
          Text('参与聚合源', style: TextStyle(fontSize: PTFs.sm, color: context.dimText, fontWeight: FontWeight.w600)),
          Wrap(
            spacing: 6,
            children: kRatingSources
                .map((o) => FilterChip(
                      label: Text(o.label),
                      selected: _cfg.sources.contains(o.value),
                      onSelected: (v) => _toggleSource(o.value, v),
                    ))
                .toList(),
          ),
        ]),
      );

  MediaQualityConfig _cfgCopyWith({
    bool? enabled,
    double? minScore,
    int? minVotes,
    int? minReleaseDays,
  }) =>
      MediaQualityConfig(
        enabled: enabled ?? _cfg.enabled,
        minScore: minScore ?? _cfg.minScore,
        minVotes: minVotes ?? _cfg.minVotes,
        minReleaseDays: minReleaseDays ?? _cfg.minReleaseDays,
        sources: _cfg.sources,
      );

  Widget _switchRow(String label, bool value, ValueChanged<bool> onChanged) => Row(
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        children: [
          Text(label, style: const TextStyle(fontSize: PTFs.body)),
          Switch(value: value, onChanged: onChanged),
        ],
      );

  Widget _numRow(String label, double value, double min, double max, double step, ValueChanged<double> onChanged) =>
      Padding(
        padding: const EdgeInsets.symmetric(vertical: PTSp.s1),
        child: Row(children: [
          Text(label, style: const TextStyle(fontSize: PTFs.body)),
          const Spacer(),
          SizedBox(
            width: 120,
            child: TextField(
              controller: TextEditingController(text: value.toStringAsFixed(value == value.roundToDouble() ? 0 : 1)),
              keyboardType: const TextInputType.numberWithOptions(decimal: true),
              onChanged: (s) {
                final v = double.tryParse(s);
                if (v != null) onChanged(v);
              },
              decoration: const InputDecoration(isDense: true),
            ),
          ),
        ]),
      );

  Widget _numRowInt(String label, double value, double min, double max, double step, ValueChanged<double> onChanged) =>
      _numRow(label, value, min, max, step, onChanged);

  Widget _previewCard() => GlassCard(
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          const Text('评分预览', style: TextStyle(fontSize: PTFs.lg, fontWeight: FontWeight.w700)),
          const SizedBox(height: PTSp.s3),
          TextField(
            controller: _titleCtrl,
            decoration: const InputDecoration(labelText: '影视标题', isDense: true),
            onSubmitted: (_) => _preview(),
          ),
          const SizedBox(height: PTSp.s2),
          Row(children: [
            SizedBox(
              width: 100,
              child: TextField(
                controller: _yearCtrl,
                keyboardType: TextInputType.number,
                decoration: const InputDecoration(labelText: '年份', isDense: true),
              ),
            ),
            const SizedBox(width: PTSp.s3),
            DropdownButton<String>(
              value: _type,
              items: const [
                DropdownMenuItem(value: '', child: Text('全部')),
                DropdownMenuItem(value: 'movie', child: Text('电影')),
                DropdownMenuItem(value: 'tv', child: Text('剧集')),
              ],
              onChanged: (v) => setState(() => _type = v ?? ''),
            ),
            const Spacer(),
            FilledButton(
              onPressed: _previewing ? null : _preview,
              child: const Text('查询'),
            ),
          ]),
          if (_previewing) const Padding(padding: EdgeInsets.all(PTSp.s4), child: Center(child: CircularProgressIndicator())),
          if (_summary != null) ...[
            const SizedBox(height: PTSp.s3),
            _summaryView(_summary!),
          ],
        ]),
      );

  Widget _summaryView(RatingSummary s) => Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Row(children: [
          GlassIcon(glyph: 'mediaquality', size: 30, from: const Color(0xFFF12711), to: const Color(0xFFF5AF19)),
          const SizedBox(width: PTSp.s3),
          Expanded(
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Text('${s.title}${s.year > 0 ? ' (${s.year})' : ''}',
                  style: const TextStyle(fontSize: PTFs.md, fontWeight: FontWeight.w700)),
              Text(
                '综合 ${s.aggregated.toStringAsFixed(1)} · 共 ${s.votes} 票 · 上映 ${s.releaseDays > 0 ? s.releaseDays : '?'} 天',
                style: TextStyle(fontSize: PTFs.xs, color: context.dimText),
              ),
            ]),
          ),
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
            decoration: BoxDecoration(
              color: (s.passed ? Colors.green : Colors.orange).withValues(alpha: 0.2),
              borderRadius: BorderRadius.circular(999),
            ),
            child: Text(s.passed ? '通过' : '未通过',
                style: TextStyle(
                    fontSize: PTFs.sm,
                    color: s.passed ? Colors.green : Colors.orange,
                    fontWeight: FontWeight.w700)),
          ),
        ]),
        if (s.reason.isNotEmpty)
          Padding(
            padding: const EdgeInsets.only(top: PTSp.s2),
            child: Text('原因：${s.reason}', style: TextStyle(fontSize: PTFs.sm, color: Colors.orange.shade300)),
          ),
        const SizedBox(height: PTSp.s2),
        Wrap(
          spacing: 6,
          runSpacing: 6,
          children: s.sources
              .map((r) => Chip(
                    label: Text('${sourceLabel(r.source)} ${r.score.toStringAsFixed(1)} · ${r.votes}票',
                        style: const TextStyle(fontSize: PTFs.xs)),
                  ))
              .toList(),
        ),
      ]);
}

