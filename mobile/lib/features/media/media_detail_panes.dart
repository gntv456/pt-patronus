// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// _MediaDetailPageState 的主图区/推荐区渲染。
// （二次拆分；行为不变。）
part of 'package:ptpatronus/features/media/media_detail_page.dart';

extension on _MediaDetailPageState {
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

                                LinearGradient(colors: [PTColors.primary, PTColors.primary2]),

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

}
