import 'package:flutter/material.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/glass_icon.dart';
import 'home_controller.dart';
import 'home_nav_items.dart';
import '../search/search_page.dart';

/// 移动端全屏命令面板（对齐 Web CommandPalette / menu-redesign §7）。
/// 模糊搜 [navItems]（含 hidden）+ PT 黑话别名；无菜单命中时跳转搜索页。
class CommandPaletteSheet extends StatefulWidget {
  final HomeController controller;
  const CommandPaletteSheet({super.key, required this.controller});

  /// 弹出全屏面板。
  static Future<void> open(BuildContext context, HomeController controller) {
    return showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      builder: (_) => CommandPaletteSheet(controller: controller),
    );
  }

  @override
  State<CommandPaletteSheet> createState() => _CommandPaletteSheetState();
}

class _CommandPaletteSheetState extends State<CommandPaletteSheet> {
  final _query = TextEditingController();
  final _focus = FocusNode();
  int _active = 0;

  /// PT 黑话别名 → label（与 Web navAliases 对齐）
  static const Map<String, List<String>> _aliases = {
    '刷流': ['刷量', '刷上传', 'auto brush'],
    '考核': ['保级', '升级', 'HR考核'],
    '监护室': ['HR', 'Hit and Run', '健康', '监护'],
    '保种契约': ['保种', '做种契约', '分享率告警'],
    '站免': ['FL', '免费', '全站免流', 'free'],
    '辅种': ['cross-seed', '辅种匹配', '跨站辅种'],
    '后宫': ['邀请', '邀请树', 'invite'],
    '勋章墙': ['勋章', 'badge', 'medal'],
    '娱乐': ['小游戏', '游戏', '抽奖'],
    '下载器': ['qb', 'qBittorrent', 'transmission', '下载'],
    '媒体服务器': ['Emby', 'Jellyfin', 'Plex', '媒体库'],
    '云盘·STRM': ['115', '夸克', '123', 'STRM', '网盘'],
    '整理': ['硬链', '整理入库', '刮削'],
    '诊断': ['自检', '诊断', 'health'],
    '备份': ['导出', '导入', '备份恢复'],
    '站点': ['PT站', '站点管理', 'cookie'],
    'AI 助手': ['AI', '助手', 'chat'],
    '搜索': ['搜种', '搜索', '种子', '影视'],
    '系统设置': ['设置', '配置'],
  };

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) => _focus.requestFocus());
  }

  @override
  void dispose() {
    _query.dispose();
    _focus.dispose();
    super.dispose();
  }

  int _score(String text, String q) {
    if (q.isEmpty) return 1;
    final t = text.toLowerCase();
    final s = q.toLowerCase().trim();
    if (s.isEmpty) return 1;
    if (t == s) return 1000;
    if (t.startsWith(s)) return 800 - s.length;
    if (t.contains(s)) return 500 - t.indexOf(s);
    var ti = 0;
    var gaps = 0;
    var last = -1;
    for (var ci = 0; ci < s.length; ci++) {
      final ch = s[ci];
      final found = t.indexOf(ch, ti);
      if (found < 0) return 0;
      if (last >= 0) gaps += found - last - 1;
      last = found;
      ti = found + 1;
    }
    final v = 200 - gaps * 3;
    return v < 1 ? 1 : v;
  }

  int _itemScore(NavItem item, String q) {
    if (q.trim().isEmpty) return 1;
    var best = _score(item.label, q);
    best = best > _score(item.glyph, q) ? best : _score(item.glyph, q);
    if (item.desc != null) {
      final d = _score(item.desc!, q);
      final scaled = (d * 0.5).round();
      if (scaled > best) best = scaled;
    }
    for (final a in _aliases[item.label] ?? const <String>[]) {
      final as = _score(a, q);
      if (as > best) best = as;
    }
    // also match alias keys when query hits alias list membership reverse
    _aliases.forEach((label, aliases) {
      if (label != item.label) return;
      for (final a in aliases) {
        final as = _score(a, q);
        if (as > best) best = as;
      }
    });
    return best;
  }

  List<({NavItem item, int index, int score})> get _filtered {
    final q = _query.text;
    final out = <({NavItem item, int index, int score})>[];
    for (var i = 0; i < navItems.length; i++) {
      final it = navItems[i];
      final sc = _itemScore(it, q);
      if (sc > 0) out.add((item: it, index: i, score: sc));
    }
    out.sort((a, b) => b.score.compareTo(a.score));
    if (out.length > 12) return out.sublist(0, 12);
    return out;
  }

  void _run(({NavItem item, int index, int score}) hit) {
    Navigator.of(context).pop();
    widget.controller.select(hit.index);
  }

  void _runSearch() {
    final q = _query.text.trim();
    Navigator.of(context).pop();
    if (q.isNotEmpty) {
      SearchPage.pendingKeyword.value = q;
    }
    final si = widget.controller.searchIndex;
    if (si != null && si >= 0) {
      widget.controller.select(si);
    }
  }

  @override
  Widget build(BuildContext context) {
    final h = MediaQuery.sizeOf(context).height;
    final filtered = _filtered;
    if (_active >= filtered.length) _active = 0;

    return Padding(
      padding: EdgeInsets.only(bottom: MediaQuery.viewInsetsOf(context).bottom),
      child: Align(
        alignment: Alignment.topCenter,
        child: Container(
          margin: EdgeInsets.fromLTRB(12, h * 0.08, 12, 12),
          constraints: BoxConstraints(maxHeight: h * 0.78),
          decoration: BoxDecoration(
            color: Theme.of(context).colorScheme.surface.withValues(alpha: 0.96),
            borderRadius: BorderRadius.circular(18),
            border: Border.all(color: PTColors.primary.withValues(alpha: 0.18)),
            boxShadow: [
              BoxShadow(
                color: Colors.black.withValues(alpha: 0.22),
                blurRadius: 28,
                offset: const Offset(0, 12),
              ),
            ],
          ),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Padding(
                padding: const EdgeInsets.fromLTRB(14, 12, 8, 8),
                child: Row(
                  children: [
                    const Icon(Icons.search, size: 20),
                    const SizedBox(width: 8),
                    Expanded(
                      child: TextField(
                        controller: _query,
                        focusNode: _focus,
                        decoration: const InputDecoration(
                          hintText: '跳转到… 菜单 / PT 黑话（刷流·保级·H&R）',
                          border: InputBorder.none,
                          isDense: true,
                        ),
                        textInputAction: TextInputAction.search,
                        onChanged: (_) => setState(() => _active = 0),
                        onSubmitted: (_) {
                          if (filtered.isNotEmpty) {
                            _run(filtered[_active]);
                          } else {
                            _runSearch();
                          }
                        },
                      ),
                    ),
                    TextButton(onPressed: () => Navigator.pop(context), child: const Text('关闭')),
                  ],
                ),
              ),
              const Divider(height: 1),
              Flexible(
                child: filtered.isEmpty
                    ? ListTile(
                        title: Text(
                          _query.text.trim().isEmpty
                              ? '输入关键词筛选菜单'
                              : '无匹配菜单 · 回车搜索「${_query.text.trim()}」',
                        ),
                        onTap: _query.text.trim().isEmpty ? null : _runSearch,
                      )
                    : ListView.builder(
                        shrinkWrap: true,
                        itemCount: filtered.length,
                        itemBuilder: (ctx, i) {
                          final hit = filtered[i];
                          final sel = i == _active;
                          return Material(
                            color: sel
                                ? PTColors.primary.withValues(alpha: 0.14)
                                : Colors.transparent,
                            child: ListTile(
                              leading: GlassIcon(
                                glyph: hit.item.glyph,
                                from: hit.item.from,
                                to: hit.item.to,
                                size: 28,
                              ),
                              title: Text(
                                hit.item.label,
                                style: TextStyle(
                                  fontWeight: sel ? FontWeight.w700 : FontWeight.w500,
                                ),
                              ),
                              subtitle: hit.item.desc == null
                                  ? null
                                  : Text(hit.item.desc!, style: const TextStyle(fontSize: 12)),
                              trailing: hit.item.hidden
                                  ? const Text('深链', style: TextStyle(fontSize: 11, color: Colors.grey))
                                  : null,
                              onTap: () => _run(hit),
                              onFocusChange: (f) {
                                if (f) setState(() => _active = i);
                              },
                            ),
                          );
                        },
                      ),
              ),
              Padding(
                padding: const EdgeInsets.fromLTRB(14, 6, 14, 10),
                child: Row(
                  children: [
                    Text('↑↓/点选', style: TextStyle(fontSize: 11, color: Colors.grey.shade600)),
                    const SizedBox(width: 12),
                    Text('回车打开', style: TextStyle(fontSize: 11, color: Colors.grey.shade600)),
                    const Spacer(),
                    Text('Esc/关闭', style: TextStyle(fontSize: 11, color: Colors.grey.shade600)),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
