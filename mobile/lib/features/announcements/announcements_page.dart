import 'package:flutter/material.dart';
import 'package:ptpatronus/features/home/home_menu_button.dart';
import 'package:ptpatronus/api/announcement.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';

/// 公告页：按站点分组展示公告条目，每条可展开正文。
class AnnouncementsPage extends StatefulWidget {
  const AnnouncementsPage({super.key});
  @override
  State<AnnouncementsPage> createState() => _AnnouncementsPageState();
}

class _AnnouncementsPageState extends State<AnnouncementsPage> {
  final _api = AnnouncementApi();
  List<AnnouncementResult> _results = [];
  bool _loading = true;
  String? _error;
  final _open = <String, bool>{};
  int? _siteFilter;
  int? _syncing; // 单站同步中

  // 按 _siteFilter 过滤（数据已全量在 _results，对齐 Web Announcements.vue 的 filtered computed）。
  List<AnnouncementResult> get _filteredResults =>
      _siteFilter == null ? _results : _results.where((r) => r.siteId == _siteFilter).toList();

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

  // 单站同步（对齐 Web group-head 的「刷新」按钮）：拉取该站最新公告并就地替换。
  Future<void> _syncOne(AnnouncementResult r) async {
    setState(() => _syncing = r.siteId);
    try {
      final one = await _api.syncOne(r.siteId);
      if (!mounted) return;
      final i = _results.indexWhere((x) => x.siteId == r.siteId);
      setState(() {
        if (i >= 0) _results[i] = one;
      });
    } catch (_) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('同步失败')));
      }
    } finally {
      if (mounted) setState(() => _syncing = null);
    }
  }

  @override
  Widget build(BuildContext context) {
    return GlassBackground(
      child: Scaffold(
        backgroundColor: Colors.transparent,
        appBar: AppBar(
          leading: const HomeMenuButton(),
          title: const Text('公告'),
          backgroundColor: Colors.transparent,
          elevation: 0,
          actions: [
            IconButton(icon: const Icon(Icons.refresh), onPressed: _loading ? null : () => _load(refresh: true)),
          ],
        ),
        body: Column(
          children: [
            if (_results.isNotEmpty) _siteFilterBar(),
            Expanded(
              child: RefreshIndicator(
                onRefresh: () => _load(refresh: true),
                child: _loading
                    ? const Center(child: CircularProgressIndicator())
                    : _error != null
                        ? PTEmptyState(text: _error!, onRetry: _load)
                        : _results.isEmpty
                            ? PTEmptyState(icon: Icons.campaign_outlined, text: '暂无数据', description: '请先添加并登录站点')
                            : _filteredResults.isEmpty
                                ? PTEmptyState(text: _siteFilter == null ? '暂无数据' : '该站点暂无公告')
                                : ListView.builder(
                                    padding: const EdgeInsets.all(12),
                                    itemCount: _filteredResults.length,
                                    itemBuilder: (_, i) => _group(_filteredResults[i]),
                                  ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  // 站点筛选药丸：点按弹候选列表（选项由已拉取的 _results 派生，一站一项）。
  String _siteNameOf(int? id) {
    if (id == null) return '全部站点';
    for (final r in _results) {
      if (r.siteId == id) return r.siteName;
    }
    return '全部站点';
  }

  Widget _siteFilterBar() {
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 6, 12, 8),
      child: Align(
        alignment: Alignment.centerLeft,
        child: PTFilterPill(
          label: '站点',
          value: _siteNameOf(_siteFilter),
          active: _siteFilter != null,
          onTap: () => showFilterSheet<int?>(
            context,
            title: '筛选站点',
            selected: _siteFilter,
            onSelected: (v) => setState(() => _siteFilter = v),
            options: [
              const PTFilterOption(value: null, label: '全部站点'),
              ..._results.map((r) => PTFilterOption(value: r.siteId, label: r.siteName)),
            ],
          ),
        ),
      ),
    );
  }

  Widget _group(AnnouncementResult r) {
    final hasItems = r.items.isNotEmpty;
    return GlassCard(
      padding: const EdgeInsets.all(14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(children: [
            Flexible(
              child: Text(r.siteName, maxLines: 1, overflow: TextOverflow.ellipsis,
                  style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 15)),
            ),
            const SizedBox(width: 8),
            if (hasItems)
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
                decoration: BoxDecoration(color: PTColors.primary.withValues(alpha: 0.16), borderRadius: BorderRadius.circular(999)),
                child: Text('${r.items.length} 条', style: const TextStyle(color: PTColors.primary, fontSize: 12)),
              )
            else if (r.error.isNotEmpty)
              Expanded(child: Text(r.error, maxLines: 2, overflow: TextOverflow.ellipsis, style: const TextStyle(color: PTColors.danger, fontSize: 12)))
            else
              const Text('暂无公告', style: TextStyle(color: Colors.grey, fontSize: 12)),
            const Spacer(),
            if (_syncing == r.siteId)
              const Padding(padding: EdgeInsets.all(8), child: SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2)))
            else
              IconButton(
                icon: const Icon(Icons.refresh, size: 18),
                tooltip: '同步该站',
                visualDensity: VisualDensity.compact,
                onPressed: () => _syncOne(r),
              ),
          ]),
          if (hasItems)
            ...r.items.asMap().entries.map((e) {
              final key = '${r.siteId}-${e.key}';
              final open = _open[key] ?? false;
              final it = e.value;
              return Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                InkWell(
                  onTap: () => setState(() => _open[key] = !open),
                  child: Padding(
                    padding: const EdgeInsets.only(top: 10),
                    child: Row(children: [
                      Expanded(child: Text(it.title.isNotEmpty ? it.title : '（无标题）', style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 14))),
                      if (it.date.isNotEmpty) Padding(padding: const EdgeInsets.only(right: 8), child: Text(it.date, style: const TextStyle(color: Colors.grey, fontSize: 12))),
                      Text(open ? '收起' : '展开', style: const TextStyle(color: Color(0xFF6C5CE7), fontSize: 12)),
                    ]),
                  ),
                ),
                if (open)
                  Padding(padding: const EdgeInsets.only(top: 6, left: 4), child: Text(it.content, style: const TextStyle(fontSize: 13, height: 1.5))),
              ]);
            })
          else if (r.rawText.isNotEmpty)
            Padding(padding: const EdgeInsets.only(top: 8), child: Text(r.rawText.length > 300 ? '${r.rawText.substring(0, 300)}…' : r.rawText, style: const TextStyle(color: Colors.grey, fontSize: 13))),
        ],
      ),
    );
  }
}
