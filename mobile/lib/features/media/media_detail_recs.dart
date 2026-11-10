// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// _MediaDetailPageState 的推荐区渲染。
// （二次拆分；行为不变。）
part of 'package:ptpatronus/features/media/media_detail_page.dart';

extension on _MediaDetailPageState {
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
}
