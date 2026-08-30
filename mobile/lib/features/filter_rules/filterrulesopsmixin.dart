// FilterRulesPage 的 FilterRulesOpsMixin mixin（含 setState 段；从 filter_rules_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/filter_rules/filter_rules_page.dart';

mixin FilterRulesOpsMixin on State<FilterRulesPage> {

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

  // 宿主提供的钩子。
  void _toast(String msg);
  List<String> _currentList(_BindingKind k);
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

}
