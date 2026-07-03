import 'package:flutter/material.dart';
import 'package:ptpatronus/features/home/home_menu_button.dart';
import 'package:ptpatronus/api/freeleech.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:url_launcher/url_launcher.dart';

/// 站免页（对齐 Web Freeleech.vue）：汇总各站全站免费/促销状态。
/// 宽屏/窄屏均用 PTMasonry 多列可变高卡片；多公告站点默认折叠+展开全部；
/// 每条公告可点「前往 →」打开外链。下拉刷新；单站可独立刷新。
class FreeleechPage extends StatefulWidget {
  const FreeleechPage({super.key});
  @override
  State<FreeleechPage> createState() => _FreeleechPageState();
}

class _FreeleechPageState extends State<FreeleechPage> {
  final _api = FreeleechApi();
  List<FreeleechResult> _results = [];
  bool _loading = true;
  String? _error;
  int? _syncingId;
  FreeleechStatus? _filter;
  final Set<int> _expanded = {}; // 多公告站点展开态

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load({bool refresh = false}) async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      _results = await _api.all(refresh: refresh);
    } catch (e) {
      _error = e.toString();
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _syncOne(FreeleechResult r) async {
    setState(() => _syncingId = r.siteId);
    try {
      final one = await _api.syncOne(r.siteId);
      final i = _results.indexWhere((x) => x.siteId == r.siteId);
      if (i >= 0) _results[i] = one;
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('$e')));
      }
    } finally {
      if (mounted) setState(() => _syncingId = null);
    }
  }

  Future<void> _openLink(String url) async {
    final uri = Uri.tryParse(url);
    if (uri == null) return;
    if (!await launchUrl(uri, mode: LaunchMode.externalApplication)) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(url)));
    }
  }

  Color _statusColor(FreeleechStatus s) => switch (s) {
        FreeleechStatus.active => const Color(0xFF2ECC71),
        FreeleechStatus.upcoming => const Color(0xFF3498DB),
        FreeleechStatus.failed => const Color(0xFFF56C6C),
        FreeleechStatus.none => const Color(0xFF95A5A6),
      };

  String _statusLabel(FreeleechStatus s) => switch (s) {
        FreeleechStatus.active => '正在站免',
        FreeleechStatus.upcoming => '即将站免',
        FreeleechStatus.failed => '读取失败',
        FreeleechStatus.none => '暂无站免',
      };

  FreeleechStatus _siteStatus(FreeleechResult r) {
    if (r.error.isNotEmpty ||
        (r.notices.isNotEmpty && r.notices.first.status == FreeleechStatus.failed)) {
      return FreeleechStatus.failed;
    }
    final top = r.notices.isNotEmpty ? r.notices.first.status : null;
    if (top == FreeleechStatus.active) return FreeleechStatus.active;
    if (top == FreeleechStatus.upcoming) return FreeleechStatus.upcoming;
    return FreeleechStatus.none;
  }

  int _count(FreeleechStatus s) => _results.where((r) => _siteStatus(r) == s).length;

  List<FreeleechResult> get _filtered =>
      _filter == null ? _results : _results.where((r) => _siteStatus(r) == _filter).toList();

  @override
  Widget build(BuildContext context) {
    return GlassBackground(
      child: Scaffold(
        backgroundColor: Colors.transparent,
        appBar: AppBar(
          leading: const HomeMenuButton(),
          title: const Text('站免'),
          backgroundColor: Colors.transparent,
          elevation: 0,
          actions: [
            PTIconAction(icon: Icons.refresh, tooltip: '刷新', loading: _loading, onPressed: _loading ? null : () => _load(refresh: true)),
          ],
        ),
        body: _loading
            ? const Center(child: CircularProgressIndicator())
            : _error != null
                ? PTEmptyState(text: _error!, onRetry: _load)
                : _results.isEmpty
                    ? PTEmptyState(icon: Icons.discount_outlined, text: '暂无数据', description: '请先添加并登录站点')
                    : Column(
                        children: [
                          _filterBar(),
                          Expanded(
                            child: RefreshIndicator(
                              onRefresh: () => _load(refresh: true),
                              child: _filtered.isEmpty
                                  ? PTEmptyState(
                                      icon: Icons.filter_alt_off_outlined,
                                      text: '没有「${_statusLabel(_filter!)}」的站点',
                                      actionLabel: '显示全部',
                                      onAction: () => setState(() => _filter = null),
                                    )
                                  : PTMasonry<FreeleechResult>(
                                      items: _filtered,
                                      minItemWidth: 340,
                                      padding: const EdgeInsets.fromLTRB(12, 0, 12, 12),
                                      itemBuilder: (_, r) => _siteCard(r),
                                    ),
                            ),
                          ),
                        ],
                      ),
      ),
    );
  }

  Widget _filterBar() {
    final cur = _filter;
    String label(FreeleechStatus? s) => s == null ? '全部' : _statusLabel(s);
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 6, 12, 8),
      child: Align(
        alignment: Alignment.centerLeft,
        child: PTFilterPill(
          label: '状态',
          value: label(cur),
          active: cur != null,
          onTap: () => showFilterSheet<FreeleechStatus?>(
            context,
            title: '筛选状态',
            selected: cur,
            onSelected: (v) => setState(() => _filter = v),
            options: [
              PTFilterOption(value: null, label: '全部', count: _results.length),
              PTFilterOption(value: FreeleechStatus.active, label: '正在站免', count: _count(FreeleechStatus.active)),
              PTFilterOption(value: FreeleechStatus.upcoming, label: '即将站免', count: _count(FreeleechStatus.upcoming)),
              PTFilterOption(value: FreeleechStatus.none, label: '暂无', count: _count(FreeleechStatus.none)),
              PTFilterOption(value: FreeleechStatus.failed, label: '失败', count: _count(FreeleechStatus.failed)),
            ],
          ),
        ),
      ),
    );
  }

  Widget _siteCard(FreeleechResult r) {
    final top = r.notices.isNotEmpty ? r.notices.first : null;
    final color = top != null ? _statusColor(top.status) : const Color(0xFF95A5A6);
    final expanded = _expanded.contains(r.siteId);
    final showAll = expanded || r.notices.length <= 1;
    final visible = showAll ? r.notices : r.notices.take(1).toList();
    return GlassCard(
      padding: const EdgeInsets.all(14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(children: [
            Text(r.siteName, style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 15)),
            const SizedBox(width: 8),
            if (top != null)
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
                decoration: BoxDecoration(color: color.withValues(alpha: 0.18), borderRadius: BorderRadius.circular(999)),
                child: Text(_statusLabel(top.status), style: TextStyle(color: color, fontSize: 12)),
              ),
            const Spacer(),
            if (_syncingId == r.siteId)
              const Padding(padding: EdgeInsets.all(8), child: SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2)))
            else
              IconButton(icon: const Icon(Icons.refresh, size: 18), onPressed: () => _syncOne(r), visualDensity: VisualDensity.compact),
          ]),
          if (r.error.isNotEmpty)
            Padding(padding: const EdgeInsets.only(top: 6), child: Text(r.error, style: const TextStyle(color: Color(0xFFF56C6C), fontSize: 13)))
          else if (r.notices.isEmpty)
            const Padding(padding: EdgeInsets.only(top: 6), child: Text('该站点未开启站免检测', style: TextStyle(color: Colors.grey, fontSize: 13)))
          else ...[
            ...visible.map((n) => _notice(n)),
            if (r.notices.length > 1)
              Align(
                alignment: Alignment.centerLeft,
                child: TextButton(
                  style: TextButton.styleFrom(padding: const EdgeInsets.symmetric(horizontal: 4), minimumSize: const Size(0, 28), tapTargetSize: MaterialTapTargetSize.shrinkWrap),
                  onPressed: () => setState(() {
                    if (expanded) {
                      _expanded.remove(r.siteId);
                    } else {
                      _expanded.add(r.siteId);
                    }
                  }),
                  child: Text(expanded ? '收起' : '展开全部 (${r.notices.length})', style: const TextStyle(fontSize: 12)),
                ),
              ),
          ],
        ],
      ),
    );
  }

  Widget _notice(SiteFreeleech n) {
    return Padding(
      padding: const EdgeInsets.only(top: 8),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Text(n.title.isNotEmpty ? n.title : _statusLabel(n.status), style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 14)),
        if (n.content.isNotEmpty || n.error.isNotEmpty)
          Padding(padding: const EdgeInsets.only(top: 2), child: Text(n.content.isNotEmpty ? n.content : n.error, style: const TextStyle(fontSize: 13))),
        if (n.link.isNotEmpty)
          Padding(
            padding: const EdgeInsets.only(top: 2),
            child: GestureDetector(onTap: () => _openLink(n.link), child: Text('前往 →', style: TextStyle(fontSize: 12, color: PTColors.primary2, decoration: TextDecoration.underline, decorationColor: PTColors.primary2))),
          ),
      ]),
    );
  }
}
