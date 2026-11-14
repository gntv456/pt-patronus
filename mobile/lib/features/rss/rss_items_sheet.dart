// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// RSS 条目抽屉：取数窗口提示 + 行表（从 rss_page.dart 按域拆出，行为不变）。
part of 'package:ptpatronus/features/rss/rss_page.dart';

extension _RssItemsSheet on _RssPageState {
  Future<void> _openItems(RssSubscription r) async {
    List<RssItem> items;
    String note;
    try {
      final page = await _api.items(r.id);
      items = page.items;
      note = rssItemWindowLabel(page);
    } catch (e) {
      _toast('$e');
      return;
    }
    if (!mounted) return;
    await showAdaptiveSheet(
      context,
      title: '条目：${r.name}',
      wideWidth: 720,
      builder: (_) => Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        mainAxisSize: MainAxisSize.min,
        children: [
          if (note.isNotEmpty) _itemsNote(note),
          if (items.isEmpty)
            const PTEmptyState(text: '暂无条目')
          else
            PTAdaptiveTable<RssItem>(
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
                            style: TextStyle(
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
                          style: TextStyle(
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
        ],
      ),
    );
  }

  Widget _itemsNote(String note) => Padding(
    // 条目列表有单页上限（默认 100、封顶 500）：不说明就是「这一页=全部条目」。
    padding: const EdgeInsets.only(bottom: 8),
    child: Text(note, style: TextStyle(color: context.dimText, fontSize: 12)),
  );
}
