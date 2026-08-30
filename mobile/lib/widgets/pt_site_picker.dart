// 从 pt_filter_sheet.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/widgets/pt_filter_sheet.dart';

class _SitePickerBody extends StatefulWidget {
  final List<int> selected;
  final List<PTFilterOption<Object?>> options;
  final ValueChanged<List<int>> onChanged;
  final bool single;
  final bool showSearch;
  const _SitePickerBody({
    required this.selected,
    required this.options,
    required this.onChanged,
    required this.single,
    required this.showSearch,
  });

  @override
  State<_SitePickerBody> createState() => _SitePickerBodyState();
}

class _SitePickerBodyState extends State<_SitePickerBody> {
  String _q = '';
  // 本地副本：实时 toggle 即回调页面，关闭 sheet 时状态已生效（与 Web 端多选一致）。
  late List<int> _selected = List.of(widget.selected);

  void _toggle(int id, bool on) {
    setState(() {
      if (on) {
        if (!_selected.contains(id)) _selected.add(id);
      } else {
        _selected.remove(id);
      }
    });
    widget.onChanged(List.of(_selected));
  }

  void _setAll(bool on) {
    setState(() {
      _selected = on ? widget.options.map((o) => o.value as int).toList() : [];
    });
    widget.onChanged(List.of(_selected));
  }

  // 站点 chip 标签：名字 + 右侧项目计数（PTFilterOption.count）。
  // >0 主色高亮（已适配），0 灰显（未适配）——一眼区分哪个站有娱乐玩法。
  Widget _chipLabel(PTFilterOption<Object?> o) {
    final n = o.count;
    if (n == null) return Text(o.label);
    return Text.rich(
      TextSpan(
        children: [
          TextSpan(text: o.label),
          TextSpan(
            text: '  $n',
            style: TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.w600,
              color: n > 0 ? PTColors.primary : context.dimText,
            ),
          ),
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final q = _q.trim().toLowerCase();
    final opts = q.isEmpty
        ? widget.options
        : widget.options
              .where((o) => o.label.toLowerCase().contains(q))
              .toList();
    // 单选模式（如娱乐页切站点）：选一项即回调并关闭，无全选/清空/完成。
    if (widget.single) {
      return Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          if (widget.showSearch)
            Padding(
              padding: const EdgeInsets.only(bottom: 10),
              child: TextField(
                onChanged: (v) => setState(() => _q = v),
                decoration: const InputDecoration(
                  isDense: true,
                  hintText: '搜索站点…',
                  prefixIcon: Icon(Icons.search, size: 20),
                  border: OutlineInputBorder(
                    borderRadius: BorderRadius.all(Radius.circular(10)),
                  ),
                ),
              ),
            ),
          Align(
            alignment: Alignment.centerLeft,
            child: Wrap(
              spacing: 8,
              runSpacing: 8,
              children: opts.map((o) {
                final id = o.value as int;
                return FilterChip(
                  label: _chipLabel(o),
                  selected: _selected.contains(id),
                  showCheckmark: false,
                  onSelected: (_) {
                    widget.onChanged([id]);
                    Navigator.of(context).pop();
                  },
                );
              }).toList(),
            ),
          ),
        ],
      );
    }
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        if (widget.showSearch)
          Padding(
            padding: const EdgeInsets.only(bottom: 8),
            child: TextField(
              onChanged: (v) => setState(() => _q = v),
              decoration: const InputDecoration(
                isDense: true,
                hintText: '搜索站点…',
                prefixIcon: Icon(Icons.search, size: 20),
                border: OutlineInputBorder(
                  borderRadius: BorderRadius.all(Radius.circular(10)),
                ),
              ),
            ),
          ),
        Row(
          children: [
            Text(
              '已选 ${_selected.length} / 共 ${widget.options.length}',
              style: TextStyle(
                fontSize: 12,
                color: Theme.of(context).colorScheme.onSurfaceVariant,
              ),
            ),
            const Spacer(),
            TextButton(onPressed: () => _setAll(true), child: const Text('全选')),
            TextButton(onPressed: () => _setAll(false), child: const Text('清空')),
          ],
        ),
        // 站点多时限制最大高度并内部滚动，避免 sheet 撑满全屏。
        ConstrainedBox(
          constraints: BoxConstraints(
            maxHeight: MediaQuery.of(context).size.height * 0.5,
          ),
          child: SingleChildScrollView(
            child: Align(
              alignment: Alignment.centerLeft,
              child: Wrap(
                spacing: 8,
                runSpacing: 8,
                children: opts.map((o) {
                  final id = o.value as int;
                  return FilterChip(
                    label: _chipLabel(o),
                    selected: _selected.contains(id),
                    showCheckmark: false,
                    onSelected: (v) => _toggle(id, v),
                  );
                }).toList(),
              ),
            ),
          ),
        ),
        const SizedBox(height: 10),
        SizedBox(
          width: double.infinity,
          child: FilledButton(
            onPressed: () => Navigator.of(context).pop(),
            child: const Text('完成'),
          ),
        ),
      ],
    );
  }
}

/// 站点筛选配置（顶栏单独按钮用）。多选：`selected` 为 siteId 列表，空=全部；
/// `options` 只含各站（不再含「全部」项，空选即全部）。`onChanged` 回调页面 typed setter。
class PTSiteFilterConfig {
  final List<int> selected;
  final List<PTFilterOption<Object?>> options;
  final ValueChanged<List<int>> onChanged;
  final bool single; // true=单选（选一项即关，如娱乐页切站点）；false=多选（默认）
  final String label; // 按钮显示的维度名前缀，默认「站点」
  final IconData icon; // 按钮图标
  const PTSiteFilterConfig({
    required this.selected,
    required this.options,
    required this.onChanged,
    this.single = false,
    this.label = '站点',
    this.icon = Icons.public,
  });

  /// 按钮上显示的选中态文案：空=「全部」/ 1 个=站名 / 多个=「N 个站点」。
  String get selectedLabel {
    if (selected.isEmpty) return '全部';
    if (selected.length == 1) {
      for (final o in options) {
        if (o.value == selected.first) return o.label;
      }
    }
    return '${selected.length} 个站点';
  }

  /// 是否处于非默认（选了具体站点）→ 按钮高亮。
  bool get active => selected.isNotEmpty;
}
