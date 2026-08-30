// SaveptPage 的 SaveptOpsMixin mixin（含 setState 段；从 savept_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/savept/savept_page.dart';

mixin SaveptOpsMixin on State<SaveptPage> {

  // ── 派生数据 ──
  List<String> get _yearOptions {
    final years = <String>{};
    for (final c in _data?.cards ?? const <SaveptSiteCard>[]) {
      if (c.year.isNotEmpty && c.year != '未知') years.add(c.year);
    }
    final list = ['全部', ...years.toList()..sort()];
    if ((_data?.cards ?? const []).any(
          (c) => c.year.isEmpty || c.year == '未知',
        ) &&
        !list.contains('未知')) {
      list.add('未知');
    }
    return list;
  }

  final _api = SaveptApi();
  final _searchCtrl = TextEditingController();
  final ScrollController _scroll = ScrollController();
  late final AnimationController _pulse;
  SaveptResult? _data;
  bool _loading = true;
  String? _error;
  bool _showBackToTop = false;

  // 筛选
  String _keyword = '';
  String _year = '全部';
  String _mpStatus = '全部'; // 全部/owned/available/unsupported
  String _status = '全部'; // 全部/健康/病危/死亡
  String _type = '全部'; // 全部/内站/外站
  bool _annivOnly = false;

  Future<void> _load({bool refresh = false}) async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      _data = await _api.get(refresh: refresh);
      if (refresh && mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(const SnackBar(content: Text('数据已刷新')));
      }
    } catch (e) {
      _error = e.toString();
      if (refresh && mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text('刷新失败：$e')));
      }
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  void _clearFilters() {
    _searchCtrl.clear();
    setState(() {
      _keyword = '';
      _year = '全部';
      _mpStatus = '全部';
      _status = '全部';
      _type = '全部';
      _annivOnly = false;
    });
  }

  // 体征点按 = 快捷筛选：点某项设对应维度，重点同一项或点「监护中」清空。
  void _toggleStatus(String v) =>
      setState(() => _status = (_status == v) ? '全部' : v);
  void _toggleType(String v) => setState(() => _type = (_type == v) ? '全部' : v);
  void _toggleMp(String v) =>
      setState(() => _mpStatus = (_mpStatus == v) ? '全部' : v);

  Widget _vitals(SaveptSummary s) {
    // 「监护中」高亮 ⇔ 当前无任何筛选（年份/关键词不计，因其另有入口）。
    final noFilter =
        _status == '全部' &&
        _type == '全部' &&
        _mpStatus == '全部' &&
        _year == '全部' &&
        !_annivOnly;
    final items = <PTStat>[
      PTStat(
        value: '${s.total}',
        label: '监护中',
        color: PTColors.primary,
        selected: noFilter,
        onTap: () => setState(() {
          _status = '全部';
          _type = '全部';
          _mpStatus = '全部';
          _year = '全部';
          _annivOnly = false;
        }),
      ),
      PTStat(
        value: '${s.healthy}',
        label: '健康',
        color: const Color(0xFF22C55E),
        selected: _status == '健康',
        onTap: () => _toggleStatus('健康'),
      ),
      PTStat(
        value: '${s.mpOwned}',
        label: '已拥有',
        color: const Color(0xFF16A34A),
        selected: _mpStatus == 'owned',
        onTap: () => _toggleMp('owned'),
      ),
      PTStat(
        value: '${s.critical}',
        label: '病危',
        color: const Color(0xFFEF4444),
        selected: _status == '病危',
        onTap: () => _toggleStatus('病危'),
      ),
      PTStat(
        value: '${s.closed}',
        label: '已关站',
        color: const Color(0xFF94A3B8),
        selected: _status == '死亡',
        onTap: () => _toggleStatus('死亡'),
      ),
      PTStat(
        value: '${s.todayAnniv}',
        label: '站庆',
        color: const Color(0xFFF59E0B),
        selected: _annivOnly,
        onTap: () => setState(() => _annivOnly = !_annivOnly),
      ),
      PTStat(
        value: '${s.internal}',
        label: '内站',
        color: const Color(0xFF3B82F6),
        selected: _type == '内站',
        onTap: () => _toggleType('内站'),
      ),
      PTStat(
        value: '${s.external}',
        label: '外站',
        color: const Color(0xFF8B5CF6),
        selected: _type == '外站',
        onTap: () => _toggleType('外站'),
      ),
    ];
    // 单行指标条：宽屏 Row 均匀分布 + 竖分隔（监护仪风格）/ 窄屏横向滚动，永不折行。
    return GlassCard(
      padding: const EdgeInsets.symmetric(vertical: 6, horizontal: 4),
      child: PTStatStrip(
        items: items,
        dividers: isWideScreen(context),
        height: 44,
      ),
    );
  }

  List<PTFilterDimension> _filterDimensions() {
    String mpLabel(String v) => switch (v) {
      'owned' => '已拥有',
      'available' => '可添加',
      'unsupported' => '未收录',
      _ => '全部',
    };
    return [
      PTFilterDimension.choice(
        label: '健康状态',
        selected: _status,
        isDefault: _status == '全部',
        onChanged: (v) => setState(() => _status = (v as String?) ?? '全部'),
        options: const [
          PTFilterOption<Object?>(value: '全部', label: '全部'),
          PTFilterOption<Object?>(value: '健康', label: '健康'),
          PTFilterOption<Object?>(value: '病危', label: '病危'),
          PTFilterOption<Object?>(value: '死亡', label: '死亡'),
        ],
      ),
      PTFilterDimension.choice(
        label: '站点类型',
        selected: _type,
        isDefault: _type == '全部',
        onChanged: (v) => setState(() => _type = (v as String?) ?? '全部'),
        options: const [
          PTFilterOption<Object?>(value: '全部', label: '全部'),
          PTFilterOption<Object?>(value: '内站', label: '内站'),
          PTFilterOption<Object?>(value: '外站', label: '外站'),
        ],
      ),
      PTFilterDimension.toggle(
        label: '仅看近期站庆',
        value: _annivOnly,
        onChanged: (v) => setState(() => _annivOnly = v),
      ),
      PTFilterDimension.choice(
        label: '建站年份',
        selected: _year,
        isDefault: _year == '全部',
        onChanged: (v) => setState(() => _year = (v as String?) ?? '全部'),
        options: _yearOptions
            .map((y) => PTFilterOption<Object?>(value: y, label: y))
            .toList(),
      ),
      PTFilterDimension.choice(
        label: 'MP 状态',
        selected: _mpStatus,
        isDefault: _mpStatus == '全部',
        onChanged: (v) => setState(() => _mpStatus = (v as String?) ?? '全部'),
        options: [
          const PTFilterOption<Object?>(value: '全部', label: '全部'),
          PTFilterOption<Object?>(value: 'owned', label: mpLabel('owned')),
          PTFilterOption<Object?>(
            value: 'available',
            label: mpLabel('available'),
          ),
          PTFilterOption<Object?>(
            value: 'unsupported',
            label: mpLabel('unsupported'),
          ),
        ],
      ),
    ];
  }
}
