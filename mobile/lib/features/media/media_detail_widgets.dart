// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// MediaDetailPage 的展示层扩展（无 setState 段；从 media_detail_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/media/media_detail_page.dart';

extension on _MediaDetailPageState {
  String _typeBadge(String t) =>

      const {'movie': '电影', 'tv': '剧集', 'music': '音乐'}[t] ?? t;

  String _sourceBadge(String s) =>

      const {'tmdb': 'TMDB', 'douban': '豆瓣', 'bangumi': 'Bangumi', 'musicbrainz': 'MB', 'cunguangji': '寸光集', 'xingguangji': '寸光集'}[s] ??

      s.toUpperCase();

  // 豆瓣 doubanio 防盗链图走后端反代（带 JWT），其它 CDN 直连。

  String _imgUrl(String u) =>

      (ApiClient().mediaImageUri(u) ?? Uri.parse(u)).toString();

  Map<String, String> _imgHeaders(String u) =>

      ApiClient().mediaImageUri(u) != null ? ApiClient().authHeaders : const {};

  Widget _buildScaffold(BuildContext context) {

    return GlassBackground(

      child: Scaffold(

        backgroundColor: Colors.transparent,

        body: SafeArea(

          child: _loading

              ? const PTLoadingView()

              : _error != null

                  ? _errorView

                  : _content(_detail!),

        ),

      ),

    );

  }

  Widget _content(MediaInfo m) {

    return ListView(

      padding: const EdgeInsets.fromLTRB(0, 0, 0, 32),

      children: [

        _hero(m),

        Padding(

          padding: const EdgeInsets.fromLTRB(16, 4, 16, 0),

          child: Wrap(

            spacing: 10,

            runSpacing: 8,

            children: [

              FilledButton.icon(

                onPressed: _searchResources,

                icon: const Icon(Icons.search, size: 18),

                label: const Text('搜索资源'),

              ),

              OutlinedButton.icon(

                onPressed: _subscribing ? null : _subscribe,

                icon: _subscribing

                    ? const SizedBox(

                        width: 16,

                        height: 16,

                        child: CircularProgressIndicator(strokeWidth: 2))

                    : const Icon(Icons.favorite_border, size: 18),

                label: Text(_subscribing ? '订阅中' : '订阅'),

              ),

            ],

          ),

        ),

        if (_metaRow(m).isNotEmpty)

          _section(children: [

            Padding(

              padding: const EdgeInsets.only(bottom: 6),

              child: Text(_metaRow(m),

                  style: TextStyle(color: context.dimText, fontSize: 13, height: 1.6)),

            ),

          ]),

        if (m.overview.isNotEmpty)

          _section(title: '简介', children: [

            Text(m.overview,

                style: const TextStyle(fontSize: 14, height: 1.7)),

          ]),

        if (m.cast.isNotEmpty) _cast(m.cast),

        if (m.recommendations.isNotEmpty) _recs(m.recommendations),

      ],

    );

  }

  String _metaRow(MediaInfo m) {

    final parts = <String>[

      if (m.genres.isNotEmpty) m.genres.join(' / '),

      if (m.runtime > 0) '${m.runtime} 分钟',

      if (m.countries.isNotEmpty) m.countries.join(' / '),

      if (m.directors.isNotEmpty) '导演：${m.directors.join('、')}',

    ];

    return parts.join('  |  ');

  }

  Widget _section({String? title, required List<Widget> children}) {

    return Padding(

      padding: const EdgeInsets.fromLTRB(16, 14, 16, 0),

      child: GlassCard(

        padding: const EdgeInsets.all(16),

        child: Column(

          crossAxisAlignment: CrossAxisAlignment.start,

          children: [

            if (title != null) ...[

              Text(title,

                  style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),

              const SizedBox(height: 10),

            ],

            ...children,

          ],

        ),

      ),

    );

  }

  Widget _cast(List<CreditPerson> cast) {

    return Padding(

      padding: const EdgeInsets.fromLTRB(16, 14, 16, 0),

      child: GlassCard(

        padding: const EdgeInsets.all(16),

        child: Column(

          crossAxisAlignment: CrossAxisAlignment.start,

          children: [

            const Text('演员阵容',

                style: TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),

            const SizedBox(height: 12),

            SizedBox(

              height: 124,

              child: ListView.separated(

                scrollDirection: Axis.horizontal,

                itemCount: cast.length,

                separatorBuilder: (_, _) => const SizedBox(width: 14),

                itemBuilder: (_, i) {

                  final p = cast[i];

                  return SizedBox(

                    width: 72,

                    child: Column(

                      children: [

                        ClipOval(

                          child: SizedBox(

                            width: 72,

                            height: 72,

                            child: MediaPoster(url: p.avatar, kind: 'movie'),

                          ),

                        ),

                        const SizedBox(height: 6),

                        Text(p.name,

                            maxLines: 2,

                            overflow: TextOverflow.ellipsis,

                            textAlign: TextAlign.center,

                            style: const TextStyle(

                                fontSize: 12, fontWeight: FontWeight.w600)),

                        if (p.role.isNotEmpty)

                          Text(p.role,

                              maxLines: 1,

                              overflow: TextOverflow.ellipsis,

                              textAlign: TextAlign.center,

                              style: TextStyle(

                                  fontSize: 11, color: context.dimText)),

                      ],

                    ),

                  );

                },

              ),

            ),

          ],

        ),

      ),

    );

  }

  Widget _chip(String label, Gradient? gradient, Color color) {

    return Container(

      padding: const EdgeInsets.symmetric(horizontal: 9, vertical: 2),

      decoration: BoxDecoration(

        borderRadius: BorderRadius.circular(999),

        gradient: gradient,

        color: gradient == null ? Colors.black.withValues(alpha: 0.06) : null,

      ),

      child: Text(label,

          style: TextStyle(

              color: color, fontSize: 12, fontWeight: FontWeight.w600)),

    );

  }

  void _openRec(MediaInfo m) {

    Navigator.push(

      context,

      MaterialPageRoute(

          builder: (_) => MediaDetailPage(source: m.source, id: m.sourceId)),

    );

  }

  void _searchResources() {

    final d = _detail;

    if (d == null) return;

    Navigator.push(

      context,

      MaterialPageRoute(builder: (_) => SearchPage(initialKeyword: d.title)),

    );

  }

  void _toast(String m) {

    if (!mounted) return;

    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));

  }
}
