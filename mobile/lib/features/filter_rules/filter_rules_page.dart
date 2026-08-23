import 'package:flutter/material.dart';
import 'package:ptpatronus/api/filter_rules.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';

import 'package:ptpatronus/widgets/pt_motion.dart';
import 'widgets/filter_test_sheet.dart';
import 'widgets/group_form.dart';
import 'widgets/rule_form.dart';

/// 资源过滤规则页（对标 MoviePilot-2 filter 模块，对齐 Web FilterRules）。
///
/// 三 Tab：
///  - 内置规则：只读列表（22 条，展示 id / 名称 / include / exclude / 体积·做种·发布约束）。
///  - 自定义规则：CRUD。
///  - 规则组：CRUD + 三个全局绑定（搜索 / 订阅 / 洗版）。
/// 顶栏「规则测试」：粘贴 rule_string + 种子标题 → POST /filter/test → 命中/剔除/层级。
class FilterRulesPage extends StatefulWidget {
  const FilterRulesPage({super.key});
  @override
  State<FilterRulesPage> createState() => _FilterRulesPageState();
}

class _FilterRulesPageState extends State<FilterRulesPage> with SingleTickerProviderStateMixin {
  final _api = FilterRuleApi();
  late final TabController _tab;

  List<FilterRule> _builtin = [];
  List<FilterRule> _custom = [];
  List<FilterRuleGroup> _groups = [];
  FilterBindings _bindings = const FilterBindings();

  bool _loadingRules = true;
  bool _loadingGroups = true;
  String? _rulesError;
  String? _groupsError;

  @override
  void initState() {
    super.initState();
    _tab = TabController(length: 3, vsync: this)..addListener(() {
        if (!_tab.indexIsChanging) setState(() {}); // 切 Tab → 刷新 FAB
      });
    _loadRules();
    _loadGroups();
  }

  @override
  void dispose() {
    _tab.dispose();
    super.dispose();
  }

  Future<void> _loadRules() async {
    setState(() {
      _loadingRules = true;
      _rulesError = null;
    });
    try {
      final results = await Future.wait([_api.builtin(), _api.custom()]);
      if (!mounted) return;
      setState(() {
        _builtin = results[0];
        _custom = results[1];
      });
    } catch (e) {
      if (mounted) setState(() => _rulesError = '$e');
    } finally {
      if (mounted) setState(() => _loadingRules = false);
    }
  }

  Future<void> _loadGroups() async {
    setState(() {
      _loadingGroups = true;
      _groupsError = null;
    });
    try {
      // 并发取规则组与全局绑定（类型不同，分两条 await 更清晰，避免 Future.wait 混类型 cast）。
      final groups = await _api.groups();
      final bindings = await _api.bindings();
      if (!mounted) return;
      setState(() {
        _groups = groups;
        _bindings = bindings;
      });
    } catch (e) {
      if (mounted) setState(() => _groupsError = '$e');
    } finally {
      if (mounted) setState(() => _loadingGroups = false);
    }
  }

  void _toast(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(content: Text(msg), duration: const Duration(seconds: 2)),
    );
  }

  // —— 自定义规则 CRUD ——
  Future<void> _openCreateCustom() async {
    final saved = await showAdaptiveSheet<bool>(
      context,
      title: '新增自定义规则',
      builder: (_) => FilterRuleForm(
        onSave: (input) async {
          await _api.createCustom(input);
          _toast('已新增规则 ${input.id}');
          return true;
        },
      ),
    );
    if (saved == true) _loadRules();
  }

  Future<void> _openEditCustom(FilterRule r) async {
    final saved = await showAdaptiveSheet<bool>(
      context,
      title: '编辑规则 ${r.id}',
      builder: (_) => FilterRuleForm(
        initial: FilterCustomRuleInput.fromRule(r),
        onSave: (input) async {
          await _api.updateCustom(r.id, input);
          _toast('已保存');
          return true;
        },
      ),
    );
    if (saved == true) _loadRules();
  }

  Future<void> _deleteCustom(FilterRule r) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('删除规则'),
        content: Text('删除自定义规则「${r.id}」？引用它的规则组将出现悬空 id。'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('取消')),
          TextButton(onPressed: () => Navigator.pop(context, true), child: const Text('删除')),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await _api.deleteCustom(r.id);
      _toast('已删除 ${r.id}');
      _loadRules();
    } catch (e) {
      _toast('删除失败：$e');
    }
  }

  // —— 规则组 CRUD ——
  Future<void> _openCreateGroup() async {
    final saved = await showAdaptiveSheet<bool>(
      context,
      title: '新增规则组',
      builder: (_) => FilterGroupForm(
        onSave: (input) async {
          await _api.createGroup(input);
          _toast('已新增规则组 ${input.name}');
          return true;
        },
      ),
    );
    if (saved == true) _loadGroups();
  }

  Future<void> _openEditGroup(FilterRuleGroup g) async {
    final saved = await showAdaptiveSheet<bool>(
      context,
      title: '编辑规则组 ${g.name}',
      builder: (_) => FilterGroupForm(
        initial: FilterRuleGroupInput(
          name: g.name,
          ruleString: g.ruleString,
          mediaType: g.mediaType,
          category: g.category,
        ),
        onSave: (input) async {
          await _api.updateGroup(g.name, input);
          _toast('已保存');
          return true;
        },
      ),
    );
    if (saved == true) _loadGroups();
  }

  Future<void> _deleteGroup(FilterRuleGroup g) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('删除规则组'),
        content: Text('删除规则组「${g.name}」？全局绑定中对它的引用会被级联清理。'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('取消')),
          TextButton(onPressed: () => Navigator.pop(context, true), child: const Text('删除')),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await _api.deleteGroup(g.name);
      _toast('已删除 ${g.name}');
      _loadGroups();
    } catch (e) {
      _toast('删除失败：$e');
    }
  }

  // —— 全局绑定多选 ——
  List<String> _currentList(_BindingKind k) {
    switch (k) {
      case _BindingKind.search:
        return _bindings.search;
      case _BindingKind.subscribe:
        return _bindings.subscribe;
      case _BindingKind.bestVersion:
        return _bindings.bestVersion;
    }
  }

  FilterBindings _withList(_BindingKind k, List<String> v) => FilterBindings(
        search: k == _BindingKind.search ? v : _bindings.search,
        subscribe: k == _BindingKind.subscribe ? v : _bindings.subscribe,
        bestVersion: k == _BindingKind.bestVersion ? v : _bindings.bestVersion,
      );

  Future<void> _editBinding(String label, _BindingKind kind) async {
    final all = _groups.map((g) => g.name).toList();
    final picked = List<String>.from(_currentList(kind));
    final ok = await showAdaptiveSheet<bool>(
      context,
      title: '$label绑定',
      wideWidth: 460,
      builder: (_) => StatefulBuilder(builder: (ctx, setS) {
        return Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            if (all.isEmpty)
              const Padding(padding: EdgeInsets.symmetric(vertical: 16), child: Text('暂无规则组，请先创建', textAlign: TextAlign.center))
            else
              ...all.map((name) => CheckboxListTile(
                    dense: true,
                    contentPadding: const EdgeInsets.symmetric(horizontal: 4),
                    value: picked.contains(name),
                    title: Text(name, style: const TextStyle(fontSize: 14)),
                    onChanged: (v) => setS(() {
                      if (v == true) {
                        picked.add(name);
                      } else {
                        picked.remove(name);
                      }
                    }),
                  )),
            const SizedBox(height: 8),
            if (all.isNotEmpty)
              FilledButton(
                onPressed: () => Navigator.pop(ctx, true),
                child: Text('确定（${picked.length}）'),
              ),
          ],
        );
      }),
    );
    if (ok == true) {
      final next = _withList(kind, picked);
      try {
        await _api.setBindings(next);
        if (mounted) setState(() => _bindings = next);
        _toast('$label绑定已更新');
      } catch (e) {
        _toast('保存失败：$e');
      }
    }
  }

  // —— 规则测试 ——
  void _openTest() {
    final prefill = (_tab.index == 2 && _groups.isNotEmpty) ? _groups.first.ruleString : '';
    showAdaptiveSheet(
      context,
      title: '规则测试',
      wideWidth: 620,
      builder: (_) => FilterTestSheet(api: _api, initialRule: prefill),
    );
  }

  @override
  Widget build(BuildContext context) {
    return PTPageScaffold(
      title: const Text('过滤规则', style: TextStyle(fontSize: 17, fontWeight: FontWeight.w700)),
      actions: [
        PTBarAction(
          icon: Icons.refresh,
          label: '刷新',
          loading: _loadingRules || _loadingGroups,
          onPressed: (_loadingRules || _loadingGroups) ? null : () { _loadRules(); _loadGroups(); },
        ),
        PTBarAction(icon: Icons.science_outlined, label: '规则测试', onPressed: _openTest),
      ],
      body: Column(children: [
        Material(
          color: Colors.transparent,
          child: TabBar(
            controller: _tab,
            tabs: const [
              Tab(text: '内置规则'),
              Tab(text: '自定义规则'),
              Tab(text: '规则组'),
            ],
          ),
        ),
        Expanded(
          child: TabBarView(
            controller: _tab,
            children: [_builtinTab(), _customTab(), _groupsTab()],
          ),
        ),
      ]),
      floatingActionButton: _fab(),
    );
  }

  Widget? _fab() {
    if (_tab.index == 1) {
      return FloatingActionButton(
        heroTag: 'filter_add_custom',
        onPressed: _openCreateCustom,
        tooltip: '新增自定义规则',
        child: const Icon(Icons.add),
      );
    }
    if (_tab.index == 2) {
      return FloatingActionButton(
        heroTag: 'filter_add_group',
        onPressed: _openCreateGroup,
        tooltip: '新增规则组',
        child: const Icon(Icons.add),
      );
    }
    return null;
  }

  // —— Tab: 内置规则（只读） ——
  Widget _builtinTab() {
    if (_loadingRules && _builtin.isEmpty) return const PTLoadingView();
    if (_rulesError != null && _builtin.isEmpty) {
      return PTEmptyState(icon: Icons.error_outline, text: '加载失败', description: _rulesError, onRetry: _loadRules);
    }
    if (_builtin.isEmpty) {
      return const PTEmptyState(icon: Icons.rule_folder_outlined, text: '暂无内置规则');
    }
    return RefreshIndicator(
      onRefresh: _loadRules,
      child: ListView.separated(
        padding: const EdgeInsets.fromLTRB(12, 8, 12, 16),
        itemCount: _builtin.length,
        separatorBuilder: (_, _) => const SizedBox(height: 8),
        itemBuilder: (_, i) => PTListItemEntrance(index: i, child: _ruleCard(_builtin[i], readonly: true)),
      ),
    );
  }

  // —— Tab: 自定义规则 ——
  Widget _customTab() {
    if (_loadingRules && _custom.isEmpty) return const PTLoadingView();
    if (_rulesError != null && _custom.isEmpty) {
      return PTEmptyState(icon: Icons.error_outline, text: '加载失败', description: _rulesError, onRetry: _loadRules);
    }
    if (_custom.isEmpty) {
      return PTEmptyState(
        icon: Icons.edit_note,
        text: '暂无自定义规则',
        description: '点右下角 + 新增；规则 id 将作为规则组表达式中的 token',
        actionLabel: '新增规则',
        onAction: _openCreateCustom,
      );
    }
    return RefreshIndicator(
      onRefresh: _loadRules,
      child: ListView.separated(
        padding: const EdgeInsets.fromLTRB(12, 8, 12, 80),
        itemCount: _custom.length,
        separatorBuilder: (_, _) => const SizedBox(height: 8),
        itemBuilder: (_, i) => _ruleCard(_custom[i], readonly: false, onEdit: () => _openEditCustom(_custom[i]), onDelete: () => _deleteCustom(_custom[i])),
      ),
    );
  }

  Widget _ruleCard(FilterRule r, {required bool readonly, VoidCallback? onEdit, VoidCallback? onDelete}) {
    return GlassCard(
      padding: const EdgeInsets.fromLTRB(12, 10, 10, 10),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Row(children: [
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
            decoration: BoxDecoration(color: PTColors.primary.withValues(alpha: 0.14), borderRadius: BorderRadius.circular(4)),
            child: Text(r.id, style: const TextStyle(fontSize: 11, fontWeight: FontWeight.w700, fontFamily: 'monospace', color: PTColors.primary)),
          ),
          const SizedBox(width: 8),
          Expanded(child: Text(r.name.isEmpty ? '(未命名)' : r.name, style: const TextStyle(fontWeight: FontWeight.w600), maxLines: 1, overflow: TextOverflow.ellipsis)),
          if (!readonly) ...[
            IconButton(icon: const Icon(Icons.edit_outlined, size: 18), tooltip: '编辑', onPressed: onEdit),
            IconButton(icon: const Icon(Icons.delete_outline, size: 18), tooltip: '删除', onPressed: onDelete),
          ],
        ]),
        const SizedBox(height: 6),
        Wrap(spacing: 6, runSpacing: 4, children: [
          ...r.include.map((p) => _tag('含: $p', PTColors.success)),
          ...r.exclude.map((p) => _tag('排: $p', PTColors.danger)),
          if (r.sizeRange.isNotEmpty) _tag('体积 ${r.sizeRange}${_mbUnit(r.sizeRange)}', PTColors.warn),
          if (r.seeders > 0) _tag('做种≥${r.seeders}', context.dimText),
          if (r.publishTime.isNotEmpty) _tag('发布 ${r.publishTime}分', context.dimText),
          ...r.match.map((m) => _tag('匹配:$m', context.dimText)),
          for (final entry in r.tmdb.entries) _tag('tmdb:${entry.key}=${entry.value}', context.dimText),
        ]),
      ]),
    );
  }

  String _mbUnit(String v) => v.toLowerCase().endsWith('mb') || v.toLowerCase().endsWith('gb') ? '' : 'MB';

  Widget _tag(String t, Color c) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(color: c.withValues(alpha: 0.12), borderRadius: BorderRadius.circular(4)),
      child: Text(t, style: TextStyle(fontSize: 10, color: c)),
    );
  }

  // —— Tab: 规则组 + 全局绑定 ——
  Widget _groupsTab() {
    if (_loadingGroups && _groups.isEmpty) {
      return const PTLoadingView();
    }
    if (_groupsError != null && _groups.isEmpty) {
      return PTEmptyState(icon: Icons.error_outline, text: '加载失败', description: _groupsError, onRetry: _loadGroups);
    }
    return RefreshIndicator(
      onRefresh: _loadGroups,
      child: ListView(
        padding: const EdgeInsets.fromLTRB(12, 8, 12, 80),
        children: [
          const _SectionLabel('规则组'),
          if (_groups.isEmpty)
            Padding(padding: EdgeInsets.symmetric(vertical: 8), child: Text('暂无规则组，点右下角 + 新增', style: TextStyle(fontSize: 12, color: context.dimText)))
          else
            ..._groups.map((g) => _groupCard(g)),
          const SizedBox(height: 16),
          const _SectionLabel('全局绑定'),
          _bindingRow(context, '搜索', _bindings.search, () => _editBinding('搜索', _BindingKind.search)),
          _bindingRow(context, '订阅', _bindings.subscribe, () => _editBinding('订阅', _BindingKind.subscribe)),
          _bindingRow(context, '洗版', _bindings.bestVersion, () => _editBinding('洗版', _BindingKind.bestVersion)),
          const SizedBox(height: 16),
          const _HelpCard(),
        ],
      ),
    );
  }

  Widget _groupCard(FilterRuleGroup g) {
    return GlassCard(
      padding: const EdgeInsets.fromLTRB(12, 10, 10, 10),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Row(children: [
          Expanded(child: Text(g.name, style: const TextStyle(fontWeight: FontWeight.w600))),
          if (g.mediaType.isNotEmpty) _tag(g.mediaType, PTColors.primary),
          if (g.category.isNotEmpty) ...[
            const SizedBox(width: 4),
            _tag(g.category, context.dimText),
          ],
          IconButton(icon: const Icon(Icons.edit_outlined, size: 18), tooltip: '编辑', onPressed: () => _openEditGroup(g)),
          IconButton(icon: const Icon(Icons.delete_outline, size: 18), tooltip: '删除', onPressed: () => _deleteGroup(g)),
        ]),
        const SizedBox(height: 4),
        Container(
          width: double.infinity,
          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 6),
          decoration: BoxDecoration(color: Theme.of(context).colorScheme.surfaceContainerHighest.withValues(alpha: 0.5), borderRadius: BorderRadius.circular(6)),
          child: Text(g.ruleString, style: const TextStyle(fontSize: 12, fontFamily: 'monospace')),
        ),
      ]),
    );
  }

  Widget _bindingRow(BuildContext context, String label, List<String> names, VoidCallback onTap) {
    return GlassCard(
      padding: const EdgeInsets.fromLTRB(12, 8, 8, 8),
      onTap: onTap,
      child: Row(children: [
        SizedBox(width: 48, child: Text(label, style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w600))),
        const SizedBox(width: 8),
        Expanded(
          child: names.isEmpty
              ? Text('未绑定', style: TextStyle(fontSize: 12, color: context.dimText))
              : Wrap(spacing: 6, runSpacing: 4, children: names.map((n) => _tag(n, PTColors.primary)).toList()),
        ),
        Icon(Icons.chevron_right, size: 18, color: context.dimText),
      ]),
    );
  }
}

/// 三个全局绑定的槽位（搜索 / 订阅 / 洗版）。
enum _BindingKind { search, subscribe, bestVersion }

/// 小节标题。
class _SectionLabel extends StatelessWidget {
  final String text;
  const _SectionLabel(this.text);
  @override
  Widget build(BuildContext context) =>
      Padding(padding: const EdgeInsets.fromLTRB(2, 6, 0, 6), child: Text(text, style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w700, color: PTColors.primary)));
}

/// rule_string 语法帮助卡。
class _HelpCard extends StatelessWidget {
  const _HelpCard();
  @override
  Widget build(BuildContext context) {
    return GlassCard(
      padding: const EdgeInsets.all(12),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Row(children: [
          const Icon(Icons.lightbulb_outline, size: 16, color: PTColors.warn),
          const SizedBox(width: 6),
          const Text('rule_string 语法', style: TextStyle(fontSize: 12, fontWeight: FontWeight.w700)),
        ]),
        const SizedBox(height: 6),
        Text(
          '> 分优先级层（左=最高优先），每层用 ! & | () 连接规则 id；'
          '种子按 first-match-wins 赋 pri_order，未命中任何层即被剔除。'
          '内置规则如 4K / 1080P / BLU / H265 等可直接引用。',
          style: TextStyle(fontSize: 11, color: Theme.of(context).hintColor, height: 1.5),
        ),
        const SizedBox(height: 6),
        Container(
          width: double.infinity,
          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
          decoration: BoxDecoration(color: PTColors.primary.withValues(alpha: 0.10), borderRadius: BorderRadius.circular(6)),
          child: const Text('例：4K & !BLU > 1080P', style: TextStyle(fontSize: 11, fontFamily: 'monospace', color: PTColors.primary)),
        ),
      ]),
    );
  }
}
