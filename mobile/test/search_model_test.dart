// 搜索模型解析测试：匹配后端 /search 响应形状（含 error 站点的容错）。
import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/api/search.dart';

void main() {
  test('SearchTorrent.fromJson parses all fields + snake_case', () {
    final t = SearchTorrent.fromJson({
      'id': 42,
      'title': 'The Movie 2024',
      'subtitle': '1080p Blu-ray',
      'size': '1.5 GB',
      'seeders': 5,
      'leechers': 2,
      'completed': 40,
      'free': true,
      'detail_url': 'https://x/details.php?id=42',
      'download_url': 'https://x/download.php?id=42&passkey=k',
    });
    expect(t.id, '42'); // 数字 id 也能解析为字符串
    expect(t.title, 'The Movie 2024');
    expect(t.size, '1.5 GB');
    expect(t.seeders, 5);
    expect(t.free, isTrue);
    expect(t.downloadUrl, startsWith('https://x/download.php'));
  });

  test('SearchTorrent.fromJson tolerates missing fields', () {
    final t = SearchTorrent.fromJson({'title': 'Bare'});
    expect(t.title, 'Bare');
    expect(t.id, '');
    expect(t.seeders, 0);
    expect(t.free, isFalse);
    expect(t.downloadUrl, '');
  });

  test('SiteSearchResult.fromJson parses torrents + error (graceful)', () {
    final s = SiteSearchResult.fromJson({
      'site_id': 7,
      'site_name': '好学',
      'torrents': [
        {'id': 1, 'title': 'A', 'size': '2 GB', 'seeders': 3},
        {'id': 2, 'title': 'B', 'free': true},
      ],
      'error': '',
    });
    expect(s.siteId, 7);
    expect(s.siteName, '好学');
    expect(s.torrents.length, 2);
    expect(s.torrents[1].free, isTrue);
    expect(s.error, '');
  });

  test('SiteSearchResult.fromJson surfaces site error (e.g. cookie expired)', () {
    final s = SiteSearchResult.fromJson({
      'site_id': 9,
      'site_name': '某站',
      'torrents': [],
      'error': 'not logged in',
    });
    expect(s.torrents, isEmpty);
    expect(s.error, 'not logged in');
  });
}
