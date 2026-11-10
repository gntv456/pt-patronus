// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'dart:typed_data';

import '../core/api_client.dart';

/// 字幕搜索结果（对齐后端 subtitle.Subtitle）。snake_case → camelCase。
class SubtitleResult {
  final String source; // assrt | opensubtitles | subhd | zimuku
  final String sourceId;
  final String title;
  final String language;
  final String langCode; // chs | cht | eng | bi | und
  final String? fileName;
  final String? ext;
  final String? extra;

  SubtitleResult({
    required this.source,
    required this.sourceId,
    required this.title,
    required this.language,
    required this.langCode,
    this.fileName,
    this.ext,
    this.extra,
  });

  factory SubtitleResult.fromJson(Map<String, dynamic> j) => SubtitleResult(
        source: j['source'] as String? ?? '',
        sourceId: j['source_id'] as String? ?? '',
        title: j['title'] as String? ?? '',
        language: j['language'] as String? ?? '',
        langCode: j['lang_code'] as String? ?? 'und',
        fileName: j['file_name'] as String?,
        ext: j['ext'] as String?,
        extra: j['extra'] as String?,
      );

  /// 下载请求体（回传后端）。
  Map<String, dynamic> toBody() => {
        'source': source,
        'source_id': sourceId,
        'title': title,
        'lang_code': langCode,
        if (fileName != null) 'file_name': fileName,
        if (ext != null) 'ext': ext,
        if (extra != null) 'extra': extra,
      };
}

/// 字幕源配置（脱敏回显 + _configured）。
class SubtitleConfig {
  final bool assrtConfigured;
  final bool opensubtitlesConfigured;
  final bool subhdConfigured;
  final bool zimukuConfigured;
  final String targetLang;

  SubtitleConfig({
    required this.assrtConfigured,
    required this.opensubtitlesConfigured,
    required this.subhdConfigured,
    required this.zimukuConfigured,
    required this.targetLang,
  });

  factory SubtitleConfig.fromJson(Map<String, dynamic> j) => SubtitleConfig(
        assrtConfigured: j['assrt_token_configured'] as bool? ?? false,
        opensubtitlesConfigured: j['opensubtitles_key_configured'] as bool? ?? false,
        subhdConfigured: j['subhd_cookie_configured'] as bool? ?? false,
        zimukuConfigured: j['zimuku_cookie_configured'] as bool? ?? false,
        targetLang: j['target_lang'] as String? ?? '',
      );
}

const Map<String, String> subtitleSourceLabel = {
  'assrt': 'ASSRT',
  'opensubtitles': 'OpenSubtitles',
  'subhd': 'SubHD',
  'zimuku': '字幕库',
};

const Map<String, String> subtitleLangLabel = {
  'chs': '简体',
  'cht': '繁体',
  'eng': '英文',
  'bi': '双语',
  'und': '未知',
};

/// 字幕 API（对齐后端 /subtitle/*）。
class SubtitleApi {
  final _api = ApiClient();

  Future<List<SubtitleResult>> search(String q, {String language = ''}) async {
    final path = language.isEmpty
        ? '/subtitle/search?q=${Uri.encodeQueryComponent(q)}'
        : '/subtitle/search?q=${Uri.encodeQueryComponent(q)}&language=$language';
    final r = await _api.get(path);
    final list = (r['results'] as List?) ?? const [];
    return list.map((e) => SubtitleResult.fromJson(e as Map<String, dynamic>)).toList();
  }

  /// 下载字幕（POST → 字节）。
  Future<Uint8List> download(SubtitleResult sub) =>
      _api.postBytes('/subtitle/download', sub.toBody());

  Future<SubtitleConfig> getConfig() async {
    final r = await _api.get('/subtitle/config');
    return SubtitleConfig.fromJson(r);
  }

  Future<void> setConfig(Map<String, dynamic> body) => _api.put('/subtitle/config', body);
}
