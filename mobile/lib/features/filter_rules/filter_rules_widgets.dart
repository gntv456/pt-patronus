// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// FilterRulesPage 的展示层扩展（无 setState 段；从 filter_rules_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/filter_rules/filter_rules_page.dart';

extension on _FilterRulesPageState {

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
