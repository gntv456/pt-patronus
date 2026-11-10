// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 媒体模型解析测试：匹配后端 /media/search 响应形状（含缺字段容错）。
import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/api/media.dart';

void main() {
  test('MediaInfo.fromJson parses all fields + snake_case', () {
    final m = MediaInfo.fromJson({
      'source': 'tmdb',
      'source_id': '123',
      'title': 'Inception',
      'original_title': 'Inception',
      'year': 2010,
      'type': 'movie',
      'overview': 'A thief who steals corporate secrets...',
      'poster_url': 'https://image.tmdb.org/xx.jpg',
      'rating': 8.8,
      'genres': ['Action', 'Sci-Fi'],
    });
    expect(m.source, 'tmdb');
    expect(m.sourceId, '123');
    expect(m.title, 'Inception');
    expect(m.year, 2010);
    expect(m.type, 'movie');
    expect(m.posterUrl, startsWith('https://image.tmdb.org'));
    expect(m.rating, 8.8);
    expect(m.genres, ['Action', 'Sci-Fi']);
  });

  test('MediaInfo.fromJson tolerates missing fields', () {
    final m = MediaInfo.fromJson({'title': 'Bare'});
    expect(m.title, 'Bare');
    expect(m.source, '');
    expect(m.year, 0);
    expect(m.genres, isEmpty);
    expect(m.rating, 0);
    expect(m.posterUrl, '');
  });

  test('MediaInfo.fromJson parses douban + tv', () {
    final m = MediaInfo.fromJson({
      'source': 'douban',
      'title': '某剧',
      'type': 'tv',
      'year': 2024,
    });
    expect(m.source, 'douban');
    expect(m.type, 'tv');
    expect(m.year, 2024);
  });

  test('MediaInfo.detailUrl 按源映射详情页', () {
    expect(
      MediaInfo.fromJson({'source': 'douban', 'source_id': 'movie:1292052'}).detailUrl,
      'https://movie.douban.com/subject/1292052/',
    );
    expect(
      MediaInfo.fromJson({'source': 'tmdb', 'source_id': 'tv:456'}).detailUrl,
      'https://www.themoviedb.org/tv/456',
    );
    expect(
      MediaInfo.fromJson({'source': 'musicbrainz', 'source_id': '3882a330'}).detailUrl,
      'https://musicbrainz.org/release-group/3882a330',
    );
    // 缺 id / 未知源 → 空串（不跳转）
    expect(MediaInfo.fromJson({'source': 'douban'}).detailUrl, '');
    expect(MediaInfo.fromJson({'source': 'unknown', 'source_id': '1'}).detailUrl, '');
  });
}
