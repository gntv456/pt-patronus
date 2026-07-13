import 'package:ptpatronus/core/api_client.dart';

// ─────────────────────────────────────────────────────────────────────────
// 资源过滤规则（对标 MoviePilot-2 filter 模块）。后端：internal/filter + internal/api/v1/filter_rule.go。
//
// 规则组 rule_string 用 '>' 分优先级层，每层是规则 id 的布尔表达式（! & | ()），
// first-match-wins 赋 pri_order；内置规则只读、自定义规则 CRUD、规则组 CRUD、
// 三个全局绑定（搜索 / 订阅 / 洗版），以及单条 rule_string 的测试求值。
// ─────────────────────────────────────────────────────────────────────────

/// FilterRule 一条过滤规则（内置或自定义，对齐后端 filter.Rule）。
/// include/exclude 为正则列表（RE2，IGNORECASE）；match 重定向匹配属性（如 ["labels"]）。
class FilterRule {
  final String id;
  final String name;
  final List<String> include;
  final List<String> exclude;
  final List<String> match;
  final String sizeRange; // MB："1000-5000" | ">1000" | "<5000"
  final int seeders; // 最少做种人数
  final double? downloadVolumeFactor; // nil=不限；0=FREE
  final String publishTime; // 分钟："60"(age>=60) | "60-1440"
  final Map<String, String> tmdb; // 元数据短路匹配

  const FilterRule({
    required this.id,
    this.name = '',
    this.include = const [],
    this.exclude = const [],
    this.match = const [],
    this.sizeRange = '',
    this.seeders = 0,
    this.downloadVolumeFactor,
    this.publishTime = '',
    this.tmdb = const {},
  });

  factory FilterRule.fromJson(Map<String, dynamic> j) {
    double? dvf;
    final raw = j['downloadvolumefactor'];
    if (raw is num) dvf = raw.toDouble();
    final tm = j['tmdb'];
    final Map<String, String> tmdb = {};
    if (tm is Map) {
      tm.forEach((k, v) => tmdb[k.toString()] = v.toString());
    }
    return FilterRule(
      id: (j['id'] ?? '').toString(),
      name: (j['name'] ?? '').toString(),
      include: ((j['include'] as List?) ?? const []).map((e) => e.toString()).toList(),
      exclude: ((j['exclude'] as List?) ?? const []).map((e) => e.toString()).toList(),
      match: ((j['match'] as List?) ?? const []).map((e) => e.toString()).toList(),
      sizeRange: (j['size_range'] ?? '').toString(),
      seeders: (j['seeders'] as num?)?.toInt() ?? 0,
      downloadVolumeFactor: dvf,
      publishTime: (j['publish_time'] ?? '').toString(),
      tmdb: tmdb,
    );
  }
}

/// FilterCustomRuleInput 自定义规则 CRUD 请求体（对齐后端 filter.CustomRuleInput）。
/// include/exclude 为单个正则字符串（需 alternation 时在串里写 a|b）。
class FilterCustomRuleInput {
  final String id; // 仅字母数字（rule_string 表达式里作 token）
  final String name;
  final String include;
  final String exclude;
  final String sizeRange;
  final int seeders;
  final String publishTime;

  const FilterCustomRuleInput({
    required this.id,
    this.name = '',
    this.include = '',
    this.exclude = '',
    this.sizeRange = '',
    this.seeders = 0,
    this.publishTime = '',
  });

  Map<String, dynamic> toJson() => {
        'id': id,
        if (name.isNotEmpty) 'name': name,
        if (include.isNotEmpty) 'include': include,
        if (exclude.isNotEmpty) 'exclude': exclude,
        if (sizeRange.isNotEmpty) 'size_range': sizeRange,
        if (seeders > 0) 'seeders': seeders,
        if (publishTime.isNotEmpty) 'publish_time': publishTime,
      };

  /// 从已存的 Rule 反推编辑表单初值：include/exclude 取首个元素。
  factory FilterCustomRuleInput.fromRule(FilterRule r) => FilterCustomRuleInput(
        id: r.id,
        name: r.name,
        include: r.include.isNotEmpty ? r.include.first : '',
        exclude: r.exclude.isNotEmpty ? r.exclude.first : '',
        sizeRange: r.sizeRange,
        seeders: r.seeders,
        publishTime: r.publishTime,
      );
}

/// FilterRuleGroup 规则组（对齐后端 filter.RuleGroup）。
class FilterRuleGroup {
  final String name;
  final String ruleString; // 如 "4K & !BLU > 1080P"
  final String mediaType; // "" | 电影 | 电视剧
  final String category;

  const FilterRuleGroup({
    required this.name,
    required this.ruleString,
    this.mediaType = '',
    this.category = '',
  });

  factory FilterRuleGroup.fromJson(Map<String, dynamic> j) => FilterRuleGroup(
        name: (j['name'] ?? '').toString(),
        ruleString: (j['rule_string'] ?? '').toString(),
        mediaType: (j['media_type'] ?? '').toString(),
        category: (j['category'] ?? '').toString(),
      );
}

/// FilterRuleGroupInput 规则组 CRUD 请求体（对齐后端 filter.RuleGroupInput）。
class FilterRuleGroupInput {
  final String name; // required
  final String ruleString; // required
  final String mediaType; // "" | 电影 | 电视剧
  final String category;

  const FilterRuleGroupInput({
    required this.name,
    required this.ruleString,
    this.mediaType = '',
    this.category = '',
  });

  Map<String, dynamic> toJson() => {
        'name': name,
        'rule_string': ruleString,
        if (mediaType.isNotEmpty) 'media_type': mediaType,
        if (category.isNotEmpty) 'category': category,
      };
}

/// FilterBindings 三个全局绑定：搜索 / 订阅 / 洗版 各自应用的规则组名列表。
class FilterBindings {
  final List<String> search;
  final List<String> subscribe;
  final List<String> bestVersion;

  const FilterBindings({
    this.search = const [],
    this.subscribe = const [],
    this.bestVersion = const [],
  });

  factory FilterBindings.fromJson(Map<String, dynamic> j) => FilterBindings(
        search: ((j['search'] as List?) ?? const []).map((e) => e.toString()).toList(),
        subscribe: ((j['subscribe'] as List?) ?? const []).map((e) => e.toString()).toList(),
        bestVersion: ((j['best_version'] as List?) ?? const []).map((e) => e.toString()).toList(),
      );

  Map<String, dynamic> toJson() => {
        'search': search,
        'subscribe': subscribe,
        'best_version': bestVersion,
      };
}

/// FilterTestTorrent 测试请求里的种子（对齐后端 site.Torrent，仅 title 必填）。
class FilterTestTorrent {
  final String title;
  final String subTitle;
  final String size; // 如 "5000MB" / "5GB"
  final int seeders;
  final bool free;

  const FilterTestTorrent({
    required this.title,
    this.subTitle = '',
    this.size = '',
    this.seeders = 0,
    this.free = false,
  });

  Map<String, dynamic> toJson() {
    final m = <String, dynamic>{'title': title};
    if (subTitle.isNotEmpty) m['subtitle'] = subTitle;
    if (size.isNotEmpty) m['size'] = size;
    if (seeders > 0) m['seeders'] = seeders;
    if (free) m['free'] = true;
    return m;
  }

  factory FilterTestTorrent.fromJson(Map<String, dynamic> j) => FilterTestTorrent(
        title: (j['title'] ?? '').toString(),
        subTitle: (j['subtitle'] ?? '').toString(),
        size: (j['size'] ?? '').toString(),
        seeders: (j['seeders'] as num?)?.toInt() ?? 0,
        free: j['free'] == true,
      );
}

/// FilterMatchedTorrent 测试命中输出：种子 + 命中的优先级 pri_order（越大越优）。
/// 注意：后端 filter.MatchedTorrent 无 json tag，默认序列化为 {Torrent, PriOrder}（首字母大写），
/// 故此处大小写两种键都兼容解析。
class FilterMatchedTorrent {
  final FilterTestTorrent torrent;
  final int priOrder;

  const FilterMatchedTorrent({required this.torrent, this.priOrder = 0});

  factory FilterMatchedTorrent.fromJson(Map<String, dynamic> j) {
    final t = j['Torrent'] ?? j['torrent'];
    final p = j['PriOrder'] ?? j['pri_order'];
    return FilterMatchedTorrent(
      torrent: t is Map ? FilterTestTorrent.fromJson(t as Map<String, dynamic>) : const FilterTestTorrent(title: ''),
      priOrder: (p is num ? p.toInt() : 0),
    );
  }
}

/// FilterTestResult 测试求值结果：命中（带 pri）/ 被剔种子 / 命中的层级明细。
class FilterTestResult {
  final List<FilterMatchedTorrent> matched;
  final List<FilterTestTorrent> dropped;
  final List<String> levels;

  const FilterTestResult({
    this.matched = const [],
    this.dropped = const [],
    this.levels = const [],
  });

  factory FilterTestResult.fromJson(Map<String, dynamic> j) => FilterTestResult(
        matched: ((j['matched'] as List?) ?? const [])
            .whereType<Map>()
            .map((e) => FilterMatchedTorrent.fromJson(e as Map<String, dynamic>))
            .toList(),
        dropped: ((j['dropped'] as List?) ?? const [])
            .whereType<Map>()
            .map((e) => FilterTestTorrent.fromJson(e as Map<String, dynamic>))
            .toList(),
        levels: ((j['levels'] as List?) ?? const []).map((e) => e.toString()).toList(),
      );
}

/// FilterRuleApi 资源过滤规则 API（/filter/*）。
class FilterRuleApi {
  final _api = ApiClient();

  // GET /filter/rules/builtin → { rules: Rule[] }
  Future<List<FilterRule>> builtin() async {
    final r = await _api.get('/filter/rules/builtin');
    final list = (r['rules'] as List?) ?? const [];
    return list.whereType<Map>().map((e) => FilterRule.fromJson(e as Map<String, dynamic>)).toList();
  }

  // GET /filter/rules/custom → { rules: Rule[] }
  Future<List<FilterRule>> custom() async {
    final r = await _api.get('/filter/rules/custom');
    final list = (r['rules'] as List?) ?? const [];
    return list.whereType<Map>().map((e) => FilterRule.fromJson(e as Map<String, dynamic>)).toList();
  }

  // POST /filter/rules/custom (CustomRuleInput) → { rule: Rule }
  Future<FilterRule> createCustom(FilterCustomRuleInput in_) async {
    final r = await _api.post('/filter/rules/custom', in_.toJson());
    final rule = r['rule'];
    return rule is Map ? FilterRule.fromJson(rule as Map<String, dynamic>) : FilterRule(id: in_.id);
  }

  // PUT /filter/rules/custom/:id (CustomRuleInput) → { rule: Rule }
  Future<FilterRule> updateCustom(String id, FilterCustomRuleInput in_) async {
    final r = await _api.put('/filter/rules/custom/${Uri.encodeComponent(id)}', in_.toJson());
    final rule = r['rule'];
    return rule is Map ? FilterRule.fromJson(rule as Map<String, dynamic>) : FilterRule(id: id);
  }

  // DELETE /filter/rules/custom/:id → { deleted }
  Future<void> deleteCustom(String id) =>
      _api.delete('/filter/rules/custom/${Uri.encodeComponent(id)}');

  // GET /filter/groups → { groups: RuleGroup[] }
  Future<List<FilterRuleGroup>> groups() async {
    final r = await _api.get('/filter/groups');
    final list = (r['groups'] as List?) ?? const [];
    return list.whereType<Map>().map((e) => FilterRuleGroup.fromJson(e as Map<String, dynamic>)).toList();
  }

  // POST /filter/groups (RuleGroupInput) → { group: RuleGroup }
  Future<FilterRuleGroup> createGroup(FilterRuleGroupInput in_) async {
    final r = await _api.post('/filter/groups', in_.toJson());
    final g = r['group'];
    return g is Map ? FilterRuleGroup.fromJson(g as Map<String, dynamic>) : FilterRuleGroup(name: in_.name, ruleString: in_.ruleString);
  }

  // PUT /filter/groups/:name (RuleGroupInput) → { group: RuleGroup }
  Future<FilterRuleGroup> updateGroup(String name, FilterRuleGroupInput in_) async {
    final r = await _api.put('/filter/groups/${Uri.encodeComponent(name)}', in_.toJson());
    final g = r['group'];
    return g is Map ? FilterRuleGroup.fromJson(g as Map<String, dynamic>) : FilterRuleGroup(name: in_.name, ruleString: in_.ruleString);
  }

  // DELETE /filter/groups/:name → { deleted }
  Future<void> deleteGroup(String name) => _api.delete('/filter/groups/${Uri.encodeComponent(name)}');

  // GET /filter/bindings → { search, subscribe, best_version }
  Future<FilterBindings> bindings() async => FilterBindings.fromJson(await _api.get('/filter/bindings'));

  // PUT /filter/bindings (same shape) → { ok: true }
  Future<void> setBindings(FilterBindings b) => _api.put('/filter/bindings', b.toJson());

  // POST /filter/test ({rule_string, torrents:[{title,...}], media?})
  //   → { matched:[{torrent,pri_order}], dropped:[torrent], levels:[string] }
  Future<FilterTestResult> test(String ruleString, List<FilterTestTorrent> torrents) async {
    final r = await _api.post('/filter/test', {
      'rule_string': ruleString,
      'torrents': torrents.map((t) => t.toJson()).toList(),
    });
    return FilterTestResult.fromJson(r);
  }
}
