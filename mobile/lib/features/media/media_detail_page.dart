import 'package:flutter/material.dart';

import 'package:ptpatronus/api/media.dart';

import 'package:ptpatronus/api/subscribe.dart';

import 'package:ptpatronus/core/api_client.dart';

import 'package:ptpatronus/theme/glass.dart';

import 'package:ptpatronus/widgets/media_poster.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';

import '../search/search_page.dart';



// 媒体详情页（对标 MoviePilot / Web MediaDetail）：点海报卡片进入，展示

// 背景图/海报/简介/演职/推荐，并提供「搜索资源」一键跳种子搜索（按标题在站点找片）。

// 入参仅 source + id，富字段由 /media/detail 现取（搜索结果只有基础字段）。

class MediaDetailPage extends StatefulWidget {

  final String source;

  final String id;

  const MediaDetailPage({super.key, required this.source, required this.id});



  @override

  State<MediaDetailPage> createState() => _MediaDetailPageState();

}



class _MediaDetailPageState extends State<MediaDetailPage> {

  MediaInfo? _detail;

  bool _loading = true;

  bool _subscribing = false;

  String? _error;



  @override

  void initState() {

    super.initState();

    _load();

  }



  Future<void> _load() async {

    setState(() {

      _loading = true;

      _error = null;

    });

    try {

      _detail = await MediaApi().detail(widget.source, widget.id);

    } catch (e) {

      _error = '$e';

    } finally {

      if (mounted) setState(() => _loading = false);

    }

  }



  void _toast(String m) {

    if (!mounted) return;

    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));

  }



  // 一键用标题跨站搜种子（跳搜索页，预填关键词）。

  void _searchResources() {

    final d = _detail;

    if (d == null) return;

    Navigator.push(

      context,

      MaterialPageRoute(builder: (_) => SearchPage(initialKeyword: d.title)),

    );

  }



  // 订阅：创建后 cron 自动搜索+下载（命中推送到默认下载器 + 通知）。同源同 id 幂等。

  Future<void> _subscribe() async {

    final d = _detail;

    if (d == null) return;

    setState(() => _subscribing = true);

    try {

      await SubscribeApi().create(

        source: d.source,

        sourceId: d.sourceId,

        title: d.title,

        year: d.year,

        type: d.type,

        category: d.type == 'tv' ? 'tv' : 'movie',

        posterUrl: d.posterUrl,

      );

      _toast('已订阅，将自动搜索并下载到默认下载器');

    } catch (e) {

      _toast('$e');

    } finally {

      if (mounted) setState(() => _subscribing = false);

    }

  }



  // 点推荐项 → 换源/id 重载本页。

  void _openRec(MediaInfo m) {

    Navigator.push(

      context,

      MaterialPageRoute(

          builder: (_) => MediaDetailPage(source: m.source, id: m.sourceId)),

    );

  }



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



  @override

  Widget build(BuildContext context) {

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



  Widget get _errorView => Center(

        child: Column(

          mainAxisSize: MainAxisSize.min,

          children: [

            Text('⚠ $_error', textAlign: TextAlign.center,

                style: const TextStyle(color: Colors.redAccent)),

            const SizedBox(height: 12),

            FilledButton(onPressed: _load, child: const Text('重试')),

          ],

        ),

      );



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



  // hero：背景图 + 海报 + 标题/评分/类型/来源彩签。海报叠在背景图下沿。

  Widget _hero(MediaInfo m) {

    final surface = Theme.of(context).scaffoldBackgroundColor == Colors.transparent

        ? (Theme.of(context).brightness == Brightness.dark

            ? const Color(0xFF16181F)

            : const Color(0xFFE6F0FF))

        : Theme.of(context).scaffoldBackgroundColor;

    return SizedBox(

      height: 300,

      width: double.infinity,

      child: Stack(

        children: [

          // 背景图

          Positioned(

            top: 0,

            left: 0,

            right: 0,

            height: 220,

            child: m.backdropUrl.isNotEmpty

                ? Image.network(_imgUrl(m.backdropUrl),

                    headers: _imgHeaders(m.backdropUrl),

                    fit: BoxFit.cover,

                    errorBuilder: (_, _, _) => Container(color: Colors.black26))

                : Container(color: Colors.black26),

          ),

          // 渐变：向下淡出到页面底色，保证下沿文字可读

          Positioned(

            top: 0,

            left: 0,

            right: 0,

            height: 220,

            child: IgnorePointer(

              child: DecoratedBox(

                decoration: BoxDecoration(

                  gradient: LinearGradient(

                    begin: Alignment.topCenter,

                    end: Alignment.bottomCenter,

                    colors: [

                      Colors.black.withValues(alpha: 0.20),

                      surface.withValues(alpha: 0.0),

                      surface.withValues(alpha: 0.9),

                    ],

                    stops: const [0.0, 0.45, 1.0],

                  ),

                ),

              ),

            ),

          ),

          // 返回

          Positioned(

            top: 2,

            left: 0,

            child: IconButton(

              tooltip: '返回',

              icon: const Icon(Icons.arrow_back, color: Colors.white),

              onPressed: () => Navigator.maybePop(context),

            ),

          ),

          // 海报 + 标题/彩签

          Positioned(

            left: 16,

            right: 16,

            bottom: 6,

            child: Row(

              crossAxisAlignment: CrossAxisAlignment.end,

              children: [

                SizedBox(

                  width: 120,

                  height: 180,

                  child: ClipRRect(

                    borderRadius: BorderRadius.circular(10),

                    child: MediaPoster(

                        url: m.posterUrl,

                        kind: m.type == 'music' ? 'music' : 'movie'),

                  ),

                ),

                const SizedBox(width: 14),

                Expanded(

                  child: Padding(

                    padding: const EdgeInsets.only(bottom: 2),

                    child: Column(

                      crossAxisAlignment: CrossAxisAlignment.start,

                      mainAxisSize: MainAxisSize.min,

                      children: [

                        Text(

                          m.title + (m.year > 0 ? ' (${m.year})' : ''),

                          maxLines: 3,

                          overflow: TextOverflow.ellipsis,

                          style: TextStyle(

                            fontSize: 19,

                            fontWeight: FontWeight.w700,

                            color: Colors.white,

                            shadows: [

                              Shadow(

                                  color: Colors.black.withValues(alpha: 0.6),

                                  blurRadius: 6),

                            ],

                          ),

                        ),

                        const SizedBox(height: 6),

                        Wrap(

                          spacing: 6,

                          runSpacing: 4,

                          children: [

                            _chip(_typeBadge(m.type),

                                const LinearGradient(colors: [PTColors.primary, PTColors.primary2]),

                                Colors.white),

                            if (m.rating > 0)

                              _chip('⭐ ${m.rating.toStringAsFixed(1)}',

                                  const LinearGradient(colors: [Color(0xFFF39C12), Color(0xFFE74C3C)]),

                                  Colors.white),

                            _chip(_sourceBadge(m.source), null, Theme.of(context).colorScheme.onSurface),

                          ],

                        ),

                      ],

                    ),

                  ),

                ),

              ],

            ),

          ),

        ],

      ),

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



  Widget _recs(List<MediaInfo> recs) {

    return Padding(

      padding: const EdgeInsets.fromLTRB(16, 14, 16, 0),

      child: GlassCard(

        padding: const EdgeInsets.all(16),

        child: Column(

          crossAxisAlignment: CrossAxisAlignment.start,

          children: [

            const Text('推荐',

                style: TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),

            const SizedBox(height: 12),

            GridView.builder(

              shrinkWrap: true,

              physics: const NeverScrollableScrollPhysics(),

              gridDelegate: const SliverGridDelegateWithMaxCrossAxisExtent(

                maxCrossAxisExtent: 130,

                mainAxisSpacing: 10,

                crossAxisSpacing: 10,

                childAspectRatio: 0.52,

              ),

              itemCount: recs.length,

              itemBuilder: (_, i) {

                final m = recs[i];

                return GestureDetector(

                  onTap: () => _openRec(m),

                  child: ClipRRect(

                    borderRadius: BorderRadius.circular(8),

                    child: Stack(

                      fit: StackFit.expand,

                      children: [

                        MediaPoster(

                            url: m.posterUrl,

                            kind: m.type == 'music' ? 'music' : 'movie'),

                        Positioned(

                          left: 0,

                          right: 0,

                          bottom: 0,

                          child: Container(

                            padding: const EdgeInsets.fromLTRB(6, 18, 6, 6),

                            decoration: const BoxDecoration(

                              gradient: LinearGradient(

                                begin: Alignment.topCenter,

                                end: Alignment.bottomCenter,

                                colors: [Colors.transparent, Colors.black87],

                              ),

                            ),

                            child: Column(

                              crossAxisAlignment: CrossAxisAlignment.start,

                              mainAxisSize: MainAxisSize.min,

                              children: [

                                Text(m.title,

                                    maxLines: 2,

                                    overflow: TextOverflow.ellipsis,

                                    style: const TextStyle(

                                        color: Colors.white,

                                        fontSize: 12,

                                        fontWeight: FontWeight.w600,

                                        height: 1.25)),

                                if (m.rating > 0)

                                  Padding(

                                    padding: const EdgeInsets.only(top: 2),

                                    child: Text('⭐ ${m.rating.toStringAsFixed(1)}',

                                        style: const TextStyle(

                                            color: Colors.white70, fontSize: 11)),

                                  ),

                              ],

                            ),

                          ),

                        ),

                      ],

                    ),

                  ),

                );

              },

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

}

