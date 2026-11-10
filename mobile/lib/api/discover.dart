// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import '../core/api_client.dart';
import 'recommend.dart';

export 'recommend.dart' show MediaInfo;

class DiscoverApi {
  final _api = ApiClient();

  // 跨源关键词搜索媒体：GET /media/discover?keyword=&year=&source=
  Future<List<MediaInfo>> search(String keyword, {int year = 0, String source = ''}) async {
    var q = '?keyword=${Uri.encodeQueryComponent(keyword)}';
    if (year > 0) q += '&year=$year';
    if (source.isNotEmpty) q += '&source=${Uri.encodeQueryComponent(source)}';
    final r = await _api.get('/media/discover$q');
    final list = (r['results'] as List?) ?? const [];
    return list.map((e) => MediaInfo.fromJson(e as Map<String, dynamic>)).toList();
  }
}
