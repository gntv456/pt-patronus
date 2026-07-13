import 'package:ptpatronus/core/api_client.dart';

// TransferHistory — 整理历史（对齐后端 models.TransferHistory）。
class TransferHistory {
  final int id;
  final String src;
  final String dest;
  final String mode; // link | move | copy | softlink
  final String type; // movie | tv
  final String title;
  final int year;
  final String source;
  final String sourceId;
  final String seasons;
  final String episodes;
  final String posterUrl;
  final int downloaderId;
  final String downloadHash;
  final String status; // success | failed
  final String errorMsg;
  final String createdAt;

  TransferHistory({
    required this.id,
    required this.src,
    required this.dest,
    required this.mode,
    required this.type,
    required this.title,
    required this.year,
    required this.source,
    required this.sourceId,
    required this.seasons,
    required this.episodes,
    required this.posterUrl,
    required this.downloaderId,
    required this.downloadHash,
    required this.status,
    required this.errorMsg,
    required this.createdAt,
  });

  factory TransferHistory.fromJson(Map<String, dynamic> j) => TransferHistory(
        id: (j['id'] as num?)?.toInt() ?? 0,
        src: j['src'] as String? ?? '',
        dest: j['dest'] as String? ?? '',
        mode: j['mode'] as String? ?? '',
        type: j['type'] as String? ?? '',
        title: j['title'] as String? ?? '',
        year: (j['year'] as num?)?.toInt() ?? 0,
        source: j['source'] as String? ?? '',
        sourceId: j['source_id'] as String? ?? '',
        seasons: j['seasons'] as String? ?? '',
        episodes: j['episodes'] as String? ?? '',
        posterUrl: j['poster_url'] as String? ?? '',
        downloaderId: (j['downloader_id'] as num?)?.toInt() ?? 0,
        downloadHash: j['download_hash'] as String? ?? '',
        status: j['status'] as String? ?? '',
        errorMsg: j['error_msg'] as String? ?? '',
        createdAt: j['created_at'] as String? ?? '',
      );
}

// TransferConfig — 整理配置（对齐后端 transfer.Config）。
class TransferConfig {
  bool enabled;
  String mode;
  String movieDir;
  String tvDir;
  List<String> categories;
  List<String> skipTags;
  String organizeTag;
  bool typeFolder;
  bool categoryFolder;
  String movieRename;
  String tvRename;
  String scrape; // missing_only | overwrite | skip
  bool refreshLibrary;
  bool overwriteFiles;

  TransferConfig({
    required this.enabled,
    required this.mode,
    required this.movieDir,
    required this.tvDir,
    required this.categories,
    required this.skipTags,
    required this.organizeTag,
    required this.typeFolder,
    required this.categoryFolder,
    required this.movieRename,
    required this.tvRename,
    required this.scrape,
    required this.refreshLibrary,
    required this.overwriteFiles,
  });

  factory TransferConfig.fromJson(Map<String, dynamic> j) => TransferConfig(
        enabled: j['enabled'] as bool? ?? true,
        mode: j['mode'] as String? ?? 'link',
        movieDir: j['movie_dir'] as String? ?? '/media/movies',
        tvDir: j['tv_dir'] as String? ?? '/media/tv',
        categories: ((j['categories'] as List?) ?? const []).map((e) => e.toString()).toList(),
        skipTags: ((j['skip_tags'] as List?) ?? const []).map((e) => e.toString()).toList(),
        organizeTag: j['organize_tag'] as String? ?? 'organize',
        typeFolder: j['type_folder'] as bool? ?? true,
        categoryFolder: j['category_folder'] as bool? ?? false,
        movieRename: j['movie_rename'] as String? ?? '',
        tvRename: j['tv_rename'] as String? ?? '',
        scrape: j['scrape'] as String? ?? 'missing_only',
        refreshLibrary: j['refresh_library'] as bool? ?? true,
        overwriteFiles: j['overwrite_files'] as bool? ?? false,
      );

  Map<String, dynamic> toJson() => {
        'enabled': enabled,
        'mode': mode,
        'movie_dir': movieDir,
        'tv_dir': tvDir,
        'categories': categories,
        'skip_tags': skipTags,
        'organize_tag': organizeTag,
        'type_folder': typeFolder,
        'category_folder': categoryFolder,
        'movie_rename': movieRename,
        'tv_rename': tvRename,
        'scrape': scrape,
        'refresh_library': refreshLibrary,
        'overwrite_files': overwriteFiles,
      };
}

class TransferApi {
  final _api = ApiClient();

  // GET /transfer/history?status=&limit=
  Future<List<TransferHistory>> history(String status, int limit) async {
    final q = <String>[];
    if (status.isNotEmpty) q.add('status=$status');
    q.add('limit=$limit');
    final r = await _api.get('/transfer/history?${q.join('&')}');
    final list = (r['history'] as List?) ?? const [];
    return list.map((e) => TransferHistory.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<TransferConfig> getConfig() async => TransferConfig.fromJson(await _api.get('/transfer/config'));

  Future<void> setConfig(TransferConfig c) => _api.put('/transfer/config', c.toJson());

  // POST /transfer/organize {downloader_id, hash} → {result}
  Future<Map<String, dynamic>?> organize(int downloaderId, String hash) async {
    final r = await _api.post('/transfer/organize', {'downloader_id': downloaderId, 'hash': hash});
    return r['result'] as Map<String, dynamic>?;
  }

  // POST /transfer/redo/:id → {result}
  Future<Map<String, dynamic>?> redo(int id) async {
    final r = await _api.post('/transfer/redo/$id', {});
    return r['result'] as Map<String, dynamic>?;
  }

  Future<void> deleteHistory(int id) => _api.delete('/transfer/history/$id');

  Future<void> sweep() => _api.post('/transfer/sweep', {});
}
