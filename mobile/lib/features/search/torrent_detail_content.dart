// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 从 search_page.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/features/search/search_page.dart';

/// 种子详情浮层内容（对齐 Web Search.vue 的 el-drawer，宽屏对话框 / 窄屏底栏由 showAdaptiveSheet 套壳）。
/// 抓取 detail_url 解析海报/规格/截图/简介/MediaInfo/文件列表 + 下载/复制链接。内容为纵向 Column，由外层滚动。
class _TorrentDetailContent extends StatefulWidget {
  final int siteId;
  final String siteName;
  final String detailUrl;
  final SearchTorrent torrent;
  final VoidCallback onDownload;

  const _TorrentDetailContent({
    required this.siteId,
    required this.siteName,
    required this.detailUrl,
    required this.torrent,
    required this.onDownload,
  });

  @override
  State<_TorrentDetailContent> createState() => _TorrentDetailContentState();
}

class _TorrentDetailContentState extends State<_TorrentDetailContent> {
  TorrentDetail? _detail;
  bool _loading = true;
  String? _err;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    try {
      _detail = await SeedBrowseApi().detail(widget.siteId, widget.detailUrl);
    } catch (e) {
      _err = '加载详情失败：$e';
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _copyLink() async {
    final url = _detail?.downloadUrl ?? widget.torrent.downloadUrl;
    if (url.isEmpty) return;
    await Clipboard.setData(ClipboardData(text: url));
    if (mounted) {
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(const SnackBar(content: Text('已复制下载链接')));
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return const Padding(
        padding: EdgeInsets.all(40),
        child: PTLoadingView(),
      );
    }
    if (_err != null) {
      return Padding(
        padding: const EdgeInsets.all(24),
        child: Text(
          _err!,
          style: const TextStyle(color: Colors.redAccent),
          textAlign: TextAlign.center,
        ),
      );
    }
    final d = _detail;
    if (d == null) {
      return const Padding(padding: EdgeInsets.all(24), child: Text('无详情'));
    }
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(
          widget.siteName,
          style: TextStyle(color: context.dimText, fontSize: 12),
        ),
        const SizedBox(height: 8),
        _header(d),
        if (d.screenshots.isNotEmpty) ...[
          _section('截图（${d.screenshots.length}）'),
          _screens(d.screenshots),
        ],
        if (d.description.trim().isNotEmpty) ...[
          _section('简介'),
          HtmlWidget(d.description, textStyle: const TextStyle(height: 1.6)),
        ],
        if (d.mediainfo.isNotEmpty) ...[
          _section('MediaInfo'),
          _mediainfo(d.mediainfo),
        ],
        if (d.fileList.isNotEmpty) ...[
          _section('文件列表（${d.fileList.length}）'),
          _fileList(d.fileList),
        ],
        const SizedBox(height: 24),
      ],
    );
  }

  Widget _header(TorrentDetail d) {
    final dlUrl = d.downloadUrl.isEmpty
        ? widget.torrent.downloadUrl
        : d.downloadUrl;
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        if (d.posterUrl.isNotEmpty) ...[
          ClipRRect(
            borderRadius: BorderRadius.circular(8),
            child: SizedBox(
              width: 96,
              height: 138,
              child: MediaPoster(url: d.posterUrl, kind: 'movie'),
            ),
          ),
          const SizedBox(width: 14),
        ],
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              if (d.subtitle.isNotEmpty)
                Padding(
                  padding: const EdgeInsets.only(bottom: 6),
                  child: Text(
                    d.subtitle,
                    style: TextStyle(color: context.dimText, fontSize: 12),
                  ),
                ),
              Wrap(
                spacing: 6,
                runSpacing: 6,
                children: [
                  if (d.category.isNotEmpty)
                    _spec(d.category, PTColors.primary),
                  if (d.resolution.isNotEmpty)
                    _spec(d.resolution, const Color(0xFF4285F4)),
                  if (d.codec.isNotEmpty)
                    _spec(d.codec, const Color(0xFF9C27B0)),
                  if (d.source.isNotEmpty)
                    _spec(d.source, const Color(0xFF009688)),
                  if (d.size.isNotEmpty) _spec(d.size, Colors.blueGrey),
                  if (d.seeders > 0) _spec('↑ ${d.seeders}', PTColors.success),
                ],
              ),
              const SizedBox(height: 10),
              Row(
                children: [
                  FilledButton.icon(
                    onPressed: dlUrl.isEmpty ? null : widget.onDownload,
                    icon: const Icon(Icons.download, size: 18),
                    label: const Text('下载'),
                  ),
                  const SizedBox(width: 8),
                  OutlinedButton.icon(
                    onPressed: dlUrl.isEmpty ? null : _copyLink,
                    icon: const Icon(Icons.copy, size: 16),
                    label: const Text('复制链接'),
                  ),
                ],
              ),
            ],
          ),
        ),
      ],
    );
  }

  Widget _screens(List<String> urls) {
    return GridView.builder(
      shrinkWrap: true,
      physics: const NeverScrollableScrollPhysics(),
      itemCount: urls.length,
      gridDelegate: const SliverGridDelegateWithMaxCrossAxisExtent(
        maxCrossAxisExtent: 160,
        mainAxisSpacing: 8,
        crossAxisSpacing: 8,
        childAspectRatio: 16 / 9,
      ),
      itemBuilder: (_, i) => ClipRRect(
        borderRadius: BorderRadius.circular(6),
        child: Image.network(
          urls[i],
          fit: BoxFit.cover,
          errorBuilder: (_, _, _) => Container(color: Colors.black12),
        ),
      ),
    );
  }

  Widget _mediainfo(String text) => Container(
    width: double.infinity,
    padding: const EdgeInsets.all(10),
    decoration: BoxDecoration(
      color: const Color(0xEE000000),
      borderRadius: BorderRadius.circular(8),
    ),
    child: SelectableText(
      text,
      style: const TextStyle(
        color: Color(0xFFD4D4D4),
        fontSize: 12,
        height: 1.5,
        fontFamily: 'monospace',
      ),
    ),
  );

  Widget _fileList(List<String> files) => Container(
    decoration: BoxDecoration(
      color: Colors.black.withValues(alpha: 0.04),
      borderRadius: BorderRadius.circular(8),
    ),
    child: Column(
      children: [
        for (int i = 0; i < files.length; i++)
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Text('📄 ', style: TextStyle(fontSize: 12)),
                Expanded(
                  child: SelectableText(
                    files[i],
                    style: const TextStyle(
                      fontSize: 12,
                      fontFamily: 'monospace',
                    ),
                  ),
                ),
              ],
            ),
          ),
      ],
    ),
  );

  Widget _section(String title) => Padding(
    padding: const EdgeInsets.fromLTRB(0, 18, 0, 8),
    child: Text(
      title,
      style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 14),
    ),
  );

  Widget _spec(String label, Color c) => Container(
    padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
    decoration: BoxDecoration(
      color: c.withValues(alpha: 0.16),
      borderRadius: BorderRadius.circular(6),
    ),
    child: Text(
      label,
      style: TextStyle(color: c, fontSize: 11, fontWeight: FontWeight.w600),
    ),
  );
}
