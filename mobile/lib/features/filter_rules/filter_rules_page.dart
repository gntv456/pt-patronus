import 'package:flutter/material.dart';
import 'package:ptpatronus/api/filter_rules.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';

import 'package:ptpatronus/widgets/pt_motion.dart';
import 'widgets/filter_test_sheet.dart';
import 'widgets/group_form.dart';
import 'widgets/rule_form.dart';

part 'filter_rules_help.dart';

part 'filterrulesopsmixin.dart';
part 'filter_rules_widgets.dart';

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

class _FilterRulesPageState extends State<FilterRulesPage> with FilterRulesOpsMixin, SingleTickerProviderStateMixin {

  @override
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

  @override
  void _toast(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(content: Text(msg), duration: const Duration(seconds: 2)),
    );
  }
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

}

/// 三个全局绑定的槽位（搜索 / 订阅 / 洗版）。
enum _BindingKind { search, subscribe, bestVersion }
