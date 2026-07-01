import 'package:http/http.dart' as http;

import '../core/api_client.dart';

class Downloader {
  final int id;
  final String name;
  final String type;
  final String url;
  final bool isDefault;
  Downloader({
    required this.id,
    required this.name,
    required this.type,
    required this.url,
    required this.isDefault,
  });
  factory Downloader.fromJson(Map<String, dynamic> j) => Downloader(
        id: (j['id'] as num?)?.toInt() ?? 0,
        name: j['name'] as String? ?? '',
        type: j['type'] as String? ?? '',
        url: j['url'] as String? ?? '',
        isDefault: j['is_default'] as bool? ?? false,
      );
}

class Torrent {
  final String hash;
  final String name;
  final int size;
  final double progress;
  final String state;
  final int downRate; // 下载速率 bytes/s（后端 dl_rate）
  final int upRate; // 上传速率 bytes/s（后端 up_rate）
  final String category;
  final List<String> tags;
  final double ratio;
  // 详情字段（qB 全有，TR 子集；缺失走默认值）
  final int downloaded;
  final int uploaded;
  final int numSeeds; // 当前连接做种数
  final int numLeechs; // 当前连接下载数
  final int numComplete; // swarm 做种总数（qB）
  final int numIncomplete; // swarm 下载总数（qB）
  final double availability; // 可用性（qB）
  final double ratioLimit; // -2=无限 -1=全局默认 >=0=值
  final int seedingTimeLimit; // 分钟，同上语义
  final bool forceStart;
  final bool autoTmm;
  final bool sequential; // 顺序下载（qB）
  final bool firstLastPiece; // 首尾优先（qB）
  final int priority; // 队列优先级（qB）
  final String addedAt; // ISO
  final String completedAt; // ISO；0001=未完成

  Torrent({
    required this.hash,
    required this.name,
    required this.size,
    required this.progress,
    required this.state,
    required this.downRate,
    required this.upRate,
    required this.category,
    required this.tags,
    this.ratio = 0,
    this.downloaded = 0,
    this.uploaded = 0,
    this.numSeeds = 0,
    this.numLeechs = 0,
    this.numComplete = 0,
    this.numIncomplete = 0,
    this.availability = 0,
    this.ratioLimit = -1,
    this.seedingTimeLimit = -1,
    this.forceStart = false,
    this.autoTmm = false,
    this.sequential = false,
    this.firstLastPiece = false,
    this.priority = 0,
    this.addedAt = '',
    this.completedAt = '',
  });
  factory Torrent.fromJson(Map<String, dynamic> j) => Torrent(
        hash: j['hash'] as String? ?? '',
        name: j['name'] as String? ?? '',
        size: (j['size'] as num?)?.toInt() ?? 0,
        progress: (j['progress'] as num?)?.toDouble() ?? 0,
        state: j['state'] as String? ?? '',
        downRate: (j['dl_rate'] as num?)?.toInt() ?? 0,
        upRate: (j['up_rate'] as num?)?.toInt() ?? 0,
        category: j['category'] as String? ?? '',
        tags: ((j['tags'] as List?) ?? const []).map((e) => e as String).toList(),
        ratio: (j['ratio'] as num?)?.toDouble() ?? 0,
        downloaded: (j['downloaded'] as num?)?.toInt() ?? 0,
        uploaded: (j['uploaded'] as num?)?.toInt() ?? 0,
        numSeeds: (j['num_seeds'] as num?)?.toInt() ?? 0,
        numLeechs: (j['num_leechs'] as num?)?.toInt() ?? 0,
        numComplete: (j['num_complete'] as num?)?.toInt() ?? 0,
        numIncomplete: (j['num_incomplete'] as num?)?.toInt() ?? 0,
        availability: (j['availability'] as num?)?.toDouble() ?? 0,
        ratioLimit: (j['ratio_limit'] as num?)?.toDouble() ?? -1,
        seedingTimeLimit: (j['seeding_time_limit'] as num?)?.toInt() ?? -1,
        forceStart: j['force_start'] as bool? ?? false,
        autoTmm: j['auto_tmm'] as bool? ?? false,
        sequential: j['sequential'] as bool? ?? false,
        firstLastPiece: j['first_last_piece'] as bool? ?? false,
        priority: (j['priority'] as num?)?.toInt() ?? 0,
        addedAt: j['added_at'] as String? ?? '',
        completedAt: j['completed_at'] as String? ?? '',
      );

  /// 磁力链（hash + 名称）。
  String get magnet => 'magnet:?xt=urn:btih:$hash&dn=${Uri.encodeComponent(name)}';
}

class TorrentFile {
  final int index;
  final String name;
  final int size;
  final double progress;
  final int priority; // qB: 0/1/2/7；TR: -1/0/1
  TorrentFile({
    required this.index,
    required this.name,
    required this.size,
    required this.progress,
    required this.priority,
  });
  factory TorrentFile.fromJson(Map<String, dynamic> j) => TorrentFile(
        index: (j['index'] as num?)?.toInt() ?? 0,
        name: j['name'] as String? ?? '',
        size: (j['size'] as num?)?.toInt() ?? 0,
        progress: (j['progress'] as num?)?.toDouble() ?? 0,
        priority: (j['priority'] as num?)?.toInt() ?? 0,
      );
}

class Tracker {
  final String url;
  final int tier;
  final String status;
  Tracker({required this.url, required this.tier, required this.status});
  factory Tracker.fromJson(Map<String, dynamic> j) => Tracker(
        url: j['url'] as String? ?? '',
        tier: (j['tier'] as num?)?.toInt() ?? 0,
        status: j['status'] as String? ?? '',
      );
}

class Peer {
  final String ip;
  final String client;
  final double progress;
  final int dlRate; // peer→我们
  final int upRate; // 我们→peer
  final String flags;
  Peer({
    required this.ip,
    required this.client,
    required this.progress,
    required this.dlRate,
    required this.upRate,
    required this.flags,
  });
  factory Peer.fromJson(Map<String, dynamic> j) => Peer(
        ip: j['ip'] as String? ?? '',
        client: j['client'] as String? ?? '',
        progress: (j['progress'] as num?)?.toDouble() ?? 0,
        dlRate: (j['dl_rate'] as num?)?.toInt() ?? 0,
        upRate: (j['up_rate'] as num?)?.toInt() ?? 0,
        flags: j['flags'] as String? ?? '',
      );
}

/// 限速（bytes/s，0=不限）
class Limits {
  final int uploadLimit;
  final int downloadLimit;
  const Limits({required this.uploadLimit, required this.downloadLimit});
}

/// 做种（分享）限制。-2=无限 -1=全局默认 >=0=具体值
class ShareLimits {
  final double ratioLimit;
  final int seedingTimeLimit; // 分钟（TR 为空闲时长）
  const ShareLimits({required this.ratioLimit, required this.seedingTimeLimit});
}

class DownloaderApi {
  final _api = ApiClient();

  Future<List<Downloader>> list() async {
    final r = await _api.get('/downloaders');
    final list = (r['downloaders'] as List?) ?? const [];
    return list.map((e) => Downloader.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<Downloader> add({
    required String name,
    required String type,
    required String url,
    String username = '',
    String password = '',
  }) async {
    final r = await _api.post('/downloaders', {
      'name': name,
      'type': type,
      'url': url,
      'username': username,
      'password': password,
      'is_default': false,
    });
    return Downloader.fromJson(r);
  }

  Future<void> remove(int id) async => _api.delete('/downloaders/$id');

  /// 编辑下载器（对齐 Web PUT /downloaders/:id）。password 留空 = 不修改。
  Future<Downloader> update(
    int id, {
    required String name,
    required String type,
    required String url,
    String username = '',
    String password = '',
    bool isDefault = false,
  }) async {
    final body = <String, dynamic>{
      'name': name,
      'type': type,
      'url': url,
      'username': username,
      'is_default': isDefault,
    };
    if (password.isNotEmpty) body['password'] = password;
    final r = await _api.put('/downloaders/$id', body);
    return Downloader.fromJson(r);
  }

  /// 连接测试（对齐 Web POST /downloaders/:id/test），返回 {ok, error?}。
  Future<Map<String, dynamic>> test(int id) async =>
      _api.post('/downloaders/$id/test', <String, dynamic>{});

  Future<List<Torrent>> torrents(int id) async {
    final r = await _api.get('/downloaders/$id/torrents');
    final list = (r['torrents'] as List?) ?? const [];
    return list.map((e) => Torrent.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<void> pause(int id, List<String> hashes) async =>
      _api.post('/downloaders/$id/pause', {'hashes': hashes});

  Future<void> resume(int id, List<String> hashes) async =>
      _api.post('/downloaders/$id/resume', {'hashes': hashes});

  Future<void> removeTorrents(int id, List<String> hashes, {bool deleteFiles = false}) async =>
      _api.post('/downloaders/$id/remove', {'hashes': hashes, 'delete_files': deleteFiles});

  // addTorrent 把一个种子 URL（download.php 链接 / magnet）推送到下载器。
  // 成功返回 true；失败（后端返回 ok:false）抛出带 error 的异常。
  Future<bool> addTorrent(int id, String url, {Map<String, dynamic>? opts}) async {
    final r = await _api.post('/downloaders/$id/torrents', {'url': url, ...?opts});
    if (r['ok'] == true) return true;
    throw Exception(r['error'] ?? '添加失败');
  }

  /// 上传 .torrent 文件。成功返回 true；失败抛带 error 异常。
  Future<bool> uploadTorrent(int id, http.MultipartFile file, {Map<String, String>? fields}) async {
    final r = await _api.postMultipart('/downloaders/$id/torrents/upload',
        files: [file], fields: fields ?? const {});
    if (r['ok'] == true) return true;
    throw Exception(r['error'] ?? '上传失败');
  }

  // —— 分类 / 标签 ——
  Future<List<String>> categories(int id) async {
    final r = await _api.get('/downloaders/$id/categories');
    return ((r['categories'] as List?) ?? const []).map((e) => e as String).toList();
  }

  Future<List<String>> tags(int id) async {
    final r = await _api.get('/downloaders/$id/tags');
    return ((r['tags'] as List?) ?? const []).map((e) => e as String).toList();
  }

  Future<void> setCategory(int id, List<String> hashes, String category) async =>
      _api.post('/downloaders/$id/torrents/category', {'hashes': hashes, 'category': category});

  Future<void> editTags(int id, List<String> hashes, List<String> tags, String mode) async =>
      _api.post('/downloaders/$id/torrents/tags', {'hashes': hashes, 'tags': tags, 'mode': mode});

  Future<void> createCategory(int id, String name, String savePath) async =>
      _api.post('/downloaders/$id/categories/create', {'name': name, 'save_path': savePath});

  Future<void> editCategory(int id, String name, String savePath) async =>
      _api.post('/downloaders/$id/categories/edit', {'name': name, 'save_path': savePath});

  Future<void> deleteCategories(int id, List<String> names) async =>
      _api.post('/downloaders/$id/categories/delete', {'names': names});

  // —— 生命周期 ——
  Future<void> recheck(int id, List<String> hashes) async =>
      _api.post('/downloaders/$id/torrents/recheck', {'hashes': hashes});

  Future<void> reannounce(int id, List<String> hashes) async =>
      _api.post('/downloaders/$id/torrents/reannounce', {'hashes': hashes});

  Future<void> setLocation(int id, List<String> hashes, String location) async =>
      _api.post('/downloaders/$id/torrents/location', {'hashes': hashes, 'location': location});

  Future<void> setForceStart(int id, List<String> hashes, bool enable) async =>
      _api.post('/downloaders/$id/torrents/force-start', {'hashes': hashes, 'enable': enable});

  // —— 队列优先级（action: top|up|down|bottom）——
  Future<void> setQueuePriority(int id, List<String> hashes, String action) async =>
      _api.post('/downloaders/$id/torrents/queue', {'hashes': hashes, 'action': action});

  // —— 顺序下载 / 首尾优先 ——
  Future<void> toggleSequential(int id, List<String> hashes) async =>
      _api.post('/downloaders/$id/torrents/sequential', {'hashes': hashes});

  Future<void> toggleFirstLast(int id, List<String> hashes) async =>
      _api.post('/downloaders/$id/torrents/first-last', {'hashes': hashes});

  // —— 做种限制 ——
  Future<void> setShareLimits(int id, List<String> hashes, ShareLimits limits) async => _api.post(
      '/downloaders/$id/torrents/share-limits',
      {'hashes': hashes, 'ratio_limit': limits.ratioLimit, 'seeding_time_limit': limits.seedingTimeLimit});

  // —— 限速 ——
  Future<void> setLimits(int id, List<String> hashes, Limits limits) async => _api.post(
      '/downloaders/$id/torrents/limits',
      {'hashes': hashes, 'upload_limit': limits.uploadLimit, 'download_limit': limits.downloadLimit});

  Future<Limits> getSpeedLimits(int id) async {
    final r = await _api.get('/downloaders/$id/speed-limits');
    return Limits(
      uploadLimit: (r['upload_limit'] as num?)?.toInt() ?? 0,
      downloadLimit: (r['download_limit'] as num?)?.toInt() ?? 0,
    );
  }

  Future<void> setSpeedLimits(int id, Limits limits) async => _api.post('/downloaders/$id/speed-limits',
      {'upload_limit': limits.uploadLimit, 'download_limit': limits.downloadLimit});

  // —— 文件 ——
  Future<List<TorrentFile>> files(int id, String hash) async {
    final r = await _api.get('/downloaders/$id/torrents/files?hash=${Uri.encodeComponent(hash)}');
    return ((r['files'] as List?) ?? const [])
        .map((e) => TorrentFile.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<void> setFilePriority(int id, String hash, List<int> indices, int priority) async =>
      _api.post('/downloaders/$id/torrents/file-priority',
          {'hash': hash, 'indices': indices, 'priority': priority});

  // —— Peers ——
  Future<List<Peer>> peers(int id, String hash) async {
    final r = await _api.get('/downloaders/$id/torrents/peers?hash=${Uri.encodeComponent(hash)}');
    return ((r['peers'] as List?) ?? const [])
        .map((e) => Peer.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  // —— Tracker ——
  Future<List<Tracker>> trackers(int id, String hash) async {
    final r = await _api.get('/downloaders/$id/torrents/trackers?hash=${Uri.encodeComponent(hash)}');
    return ((r['trackers'] as List?) ?? const [])
        .map((e) => Tracker.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<void> addTrackers(int id, String hash, List<String> urls) async =>
      _api.post('/downloaders/$id/torrents/trackers/add', {'hash': hash, 'urls': urls});

  Future<void> removeTrackers(int id, String hash, List<String> urls) async =>
      _api.post('/downloaders/$id/torrents/trackers/remove', {'hash': hash, 'urls': urls});
}
