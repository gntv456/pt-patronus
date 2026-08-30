import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:ptpatronus/api/rss.dart';
import 'package:ptpatronus/api/site.dart';
import 'package:ptpatronus/api/downloader.dart';
import 'package:ptpatronus/api/filter_rules.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';

part 'rss_form.dart';

/// RSS 订阅页：对齐 Web Rss.vue。订阅卡片列表 + 检查/条目/编辑/删除；
/// 表单走自适应浮层（宽屏对话框 / 窄屏底栏），含体积范围(min_size/max_size，修编辑丢数据)。
class RssPage extends StatefulWidget {
  const RssPage({super.key});
  @override
  State<RssPage> createState() => _RssPageState();
}

class _RssPageState extends State<RssPage> {
  final _api = RssApi();
  final _filterApi = FilterRuleApi();
  List<RssSubscription> _list = [];
  List<Site> _sites = const [];
  List<Downloader> _downloaders = const [];
  List<String> _filterGroups = const [];
  bool _loading = false;
  int? _checking;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() => _loading = true);
    try {
      _list = await _api.list();
      if (_sites.isEmpty) {
        _sites = await SiteApi().list().catchError((_) => <Site>[]);
      }
      if (_downloaders.isEmpty) {
        _downloaders = await DownloaderApi().list().catchError(
          (_) => <Downloader>[],
        );
      }
      if (_filterGroups.isEmpty) {
        final gs = await _filterApi.groups().catchError((_) => <FilterRuleGroup>[]);
        _filterGroups = gs.map((g) => g.name).toList();
      }
    } catch (e) {
      _toast('$e');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  void _toast(String s) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(s)));
  }

  Future<void> _check(RssSubscription r) async {
    setState(() => _checking = r.id);
    try {
      final n = await _api.check(r.id);
      _toast('新增 $n 条');
      await _load();
    } catch (e) {
      _toast('$e');
    } finally {
      if (mounted) setState(() => _checking = null);
    }
  }

  Future<void> _delete(RssSubscription r) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('确认删除'),
        content: Text('删除 RSS「${r.name}」？'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('删除'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await _api.remove(r.id);
      await _load();
    } catch (e) {
      _toast('$e');
    }
  }

  Future<void> _openForm([RssSubscription? r]) async {
    final saved = await showAdaptiveSheet<bool>(
      context,
      title: r == null ? '添加订阅' : '编辑订阅',
      wideWidth: 560,
      builder: (ctx) => _RssForm(
        editing: r,
        sites: _sites,
        downloaders: _downloaders,
        filterGroups: _filterGroups,
        onSaved: () => Navigator.pop(ctx, true),
      ),
    );
    if (saved == true) _load();
  }

  Future<void> _openItems(RssSubscription r) async {
    List<RssItem> items;
    try {
      items = await _api.items(r.id);
    } catch (e) {
      _toast('$e');
      return;
    }
    if (!mounted) return;
    await showAdaptiveSheet(
      context,
      title: '条目：${r.name}',
      wideWidth: 720,
      builder: (_) => items.isEmpty
          ? const PTEmptyState(text: '暂无条目')
          : PTAdaptiveTable<RssItem>(
              items: items,
              columns: [
                PTColumn(
                  label: '标题',
                  flex: 4,
                  cell: (c, it) => it.link.isNotEmpty
                      ? InkWell(
                          onTap: () => _launchUrl(it.link),
                          child: Text(
                            it.title,
                            maxLines: 2,
                            overflow: TextOverflow.ellipsis,
                            style: const TextStyle(
                              fontSize: 13,
                              color: PTColors.primary2,
                              decoration: TextDecoration.underline,
                            ),
                          ),
                        )
                      : Text(
                          it.title,
                          maxLines: 2,
                          overflow: TextOverflow.ellipsis,
                          style: const TextStyle(fontSize: 13),
                        ),
                ),
                PTColumn(
                  label: '大小',
                  flex: 1,
                  numeric: true,
                  cell: (_, it) => Text(
                    _fmtSize(it.size),
                    style: const TextStyle(fontSize: 12),
                  ),
                ),
                PTColumn(
                  label: '已下载',
                  flex: 1,
                  cell: (ctx, it) => Text(
                    it.downloaded ? '是' : '否',
                    style: TextStyle(
                      fontSize: 12,
                      color: it.downloaded ? PTColors.success : ctx.dimText,
                    ),
                  ),
                ),
                PTColumn(
                  label: '发现时间',
                  flex: 2,
                  cell: (ctx, it) => Text(
                    _fmtTime(it.createdAt),
                    style: TextStyle(fontSize: 12, color: ctx.dimText),
                  ),
                ),
              ],
              emptyText: '暂无条目',
              narrowCard: (ctx, it) => GlassCard(
                padding: const EdgeInsets.all(12),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    if (it.link.isNotEmpty)
                      InkWell(
                        onTap: () => _launchUrl(it.link),
                        child: Text(
                          it.title,
                          style: const TextStyle(
                            fontSize: 13,
                            fontWeight: FontWeight.w600,
                            color: PTColors.primary2,
                          ),
                        ),
                      )
                    else
                      Text(
                        it.title,
                        style: const TextStyle(
                          fontSize: 13,
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                    const SizedBox(height: 4),
                    Text(
                      '${_fmtSize(it.size)} · ${it.downloaded ? "已下载" : "未下载"} · ${_fmtTime(it.createdAt)}',
                      style: TextStyle(fontSize: 12, color: ctx.dimText),
                    ),
                  ],
                ),
              ),
            ),
    );
  }

  void _launchUrl(String url) {
    // 移动端无内嵌浏览器跳转需求时仅复制/忽略；保持与 Web 新标签打开一致的最小实现。
    _toast(url);
  }

  String _fmtTime(String? t) => t == null
      ? '—'
      : t.substring(0, t.length > 16 ? 16 : t.length).replaceAll('T', ' ');
  String _fmtSize(int b) {
    if (b <= 0) return '—';
    const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
    double n = b.toDouble();
    int u = 0;
    while (n >= 1024 && u < units.length - 1) {
      n /= 1024;
      u++;
    }
    return u <= 1
        ? '${n.toStringAsFixed(0)} ${units[u]}'
        : '${n.toStringAsFixed(2)} ${units[u]}';
  }

  @override
  Widget build(BuildContext context) {
    return PTPageScaffold(
      title: 'RSS 订阅',
      actions: [
        PTBarAction(onPressed: _load, icon: Icons.refresh, label: '刷新'),
        PTBarAction(
          onPressed: () => _openForm(),
          icon: Icons.add,
          label: '添加订阅',
        ),
      ],
      body: _loading && _list.isEmpty
          ? const PTLoadingView()
          : _list.isEmpty
          ? RefreshIndicator(
              onRefresh: _load,
              child: PTEmptyState(
                icon: Icons.rss_feed,
                text: '暂无 RSS 订阅',
                description: '订阅 RSS/Atom 源，按关键词/大小/分辨率过滤后自动下载',
                actionLabel: '添加订阅',
                onAction: () => _openForm(),
              ),
            )
          : RefreshIndicator(
              onRefresh: _load,
              child: ListView.builder(
                padding: const EdgeInsets.all(12),
                itemCount: _list.length,
                itemBuilder: (_, i) {
                  final r = _list[i];
                  return Padding(
                    padding: const EdgeInsets.only(bottom: 10),
                    child: GlassCard(
                      padding: const EdgeInsets.all(14),
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Row(
                            children: [
                              Expanded(
                                child: Text(
                                  r.name,
                                  style: const TextStyle(
                                    fontSize: 16,
                                    fontWeight: FontWeight.w700,
                                  ),
                                ),
                              ),
                              _chip(
                                r.enabled ? '启用' : '停用',
                                r.enabled ? PTColors.success : context.dimText,
                              ),
                              if (r.autoDownload) _chip('自动下载', PTColors.warn),
                              if (r.resolution.isNotEmpty)
                                _chip(r.resolution, PTColors.primary2),
                            ],
                          ),
                          const SizedBox(height: 4),
                          Text(
                            r.url,
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                            style: TextStyle(
                              fontSize: 12,
                              color: context.dimText,
                            ),
                          ),
                          Text(
                            '间隔 ${r.intervalMin} 分钟 · 上次 ${_fmtTime(r.lastCheckedAt)}${r.titleInclude.isNotEmpty ? ' · 包含: ${r.titleInclude}' : ''}',
                            style: TextStyle(
                              fontSize: 12,
                              color: context.dimText,
                            ),
                          ),
                          const SizedBox(height: 8),
                          Wrap(
                            spacing: 8,
                            children: [
                              FilledButton.icon(
                                onPressed: _checking == r.id
                                    ? null
                                    : () => _check(r),
                                icon: _checking == r.id
                                    ? const SizedBox(
                                        width: 14,
                                        height: 14,
                                        child: CircularProgressIndicator(
                                          strokeWidth: 2,
                                        ),
                                      )
                                    : const Icon(Icons.refresh, size: 16),
                                label: const Text('检查'),
                              ),
                              OutlinedButton(
                                onPressed: () => _openItems(r),
                                child: const Text('条目'),
                              ),
                              OutlinedButton(
                                onPressed: () => _openForm(r),
                                child: const Text('编辑'),
                              ),
                              TextButton(
                                onPressed: () => _delete(r),
                                child: const Text(
                                  '删除',
                                  style: TextStyle(color: PTColors.danger),
                                ),
                              ),
                            ],
                          ),
                        ],
                      ),
                    ),
                  );
                },
              ),
            ),
    );
  }

  Widget _chip(String text, Color c) => Container(
    margin: const EdgeInsets.only(left: 4),
    padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
    decoration: BoxDecoration(
      color: c.withValues(alpha: 0.18),
      borderRadius: BorderRadius.circular(999),
    ),
    child: Text(
      text,
      style: TextStyle(color: c, fontSize: 12, fontWeight: FontWeight.w600),
    ),
  );
}
