// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import '../core/api_client.dart';

class AnnouncementItem {
  final String title;
  final String date;
  final String content;
  AnnouncementItem({required this.title, required this.date, required this.content});

  factory AnnouncementItem.fromJson(Map<String, dynamic> j) => AnnouncementItem(
        title: j['title'] as String? ?? '',
        date: j['date'] as String? ?? '',
        content: j['content'] as String? ?? '',
      );
}

class AnnouncementResult {
  final int siteId;
  final String siteName;
  final String rawText;
  final List<AnnouncementItem> items;
  final DateTime fetchedAt;
  final String error;
  AnnouncementResult({
    required this.siteId,
    required this.siteName,
    required this.rawText,
    required this.items,
    required this.fetchedAt,
    required this.error,
  });

  factory AnnouncementResult.fromJson(Map<String, dynamic> j) => AnnouncementResult(
        siteId: (j['site_id'] as num?)?.toInt() ?? 0,
        siteName: j['site_name'] as String? ?? '',
        rawText: j['raw_text'] as String? ?? '',
        items: ((j['items'] as List?) ?? const [])
            .map((e) => AnnouncementItem.fromJson(e as Map<String, dynamic>))
            .toList(),
        fetchedAt: DateTime.tryParse(j['fetched_at'] as String? ?? '') ?? DateTime.now(),
        error: j['error'] as String? ?? '',
      );
}

class AnnouncementApi {
  final _api = ApiClient();

  Future<List<AnnouncementResult>> all({bool refresh = false}) async {
    final r = await _api.get('/announcements${refresh ? '?refresh=1' : ''}');
    final list = (r['sites'] as List?) ?? const [];
    return list.map((e) => AnnouncementResult.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<AnnouncementResult> syncOne(int id) async {
    final r = await _api.post('/announcements/sync/$id', {});
    return AnnouncementResult.fromJson(r);
  }
}
