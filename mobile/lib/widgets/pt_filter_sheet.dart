import 'package:flutter/material.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';

// ─────────────────────────────────────────────────────────────────────────
// 多维度筛选弹窗：把一页里散落的多个 PTFilterPill（每个开一个 showFilterSheet）
// 收进一个筛选按钮 → 点按弹出一个包含全部维度的 sheet。每维一节：choice=候选
// 列表（同 showFilterSheet 的 ListTile 布局），toggle=Switch。底部可选「清除全部」。
//
// 设计要点：
//  - 异构维度共享一个 List：PTFilterDimension 把泛型擦除到 Object?，类型安全留在
//    各页 onChanged 闭包里（页面已有 typed setter，如 setState(() => _status = v as String)）。
//  - 多选不即关：sheet 自持本地选中态 _sel，点选项更新本地（checkmark 跟手）+
//    回调页 onChanged（页面 setState 同步）；用户下拉/返回关闭。
//  - 激活徽标：bar 用 dimension.isDefault 判定；tab-like 结构维度（邮箱/设备类型）
//    置 isStructural=true，恒不计入徽标（否则常亮）。
//  - 复用 showAdaptiveSheet（宽屏对话框 / 窄屏底栏）+ PTFilterOption，不另造容器。
// ─────────────────────────────────────────────────────────────────────────

enum PTFilterKind { choice, toggle }

/// 一个筛选维度。`.choice` 单选（候选列表）/ `.toggle` 开关（二元）。
class PTFilterDimension {
  final String label; // 节标题（如「状态」「近期站庆」）
  final PTFilterKind kind;
  final Object? selected; // choice: 当前值（含 null="全部"）；toggle: bool
  final List<PTFilterOption<Object?>> options; // 仅 choice；toggle 传 const []
  final bool isDefault; // true=处于默认值，不计入「筛选激活」徽标
  final bool isStructural; // true=结构性维度（邮箱/设备类型 Tab 折叠而来），恒不计入徽标
  final ValueChanged<Object?> onChanged; // 点选/拨开关 → 页面 typed setter

  /// 单选维度。`selected` 为当前值，`options` 含「全部」(null) 在内的候选；
  /// `isDefault` 通常 = (selected == 默认值)。
  const PTFilterDimension.choice({
    required this.label,
    required this.selected,
    required this.options,
    required this.onChanged,
    this.isDefault = false,
    this.isStructural = false,
  }) : kind = PTFilterKind.choice;

  /// 开关维度。`value` 为当前开/关；默认关（isDefault = !value）。
  /// 非 const：初始化器要把 bool 回调包成 Object? 回调（_boolWrap 是运行时函数）。
  PTFilterDimension.toggle({
    required this.label,
    required bool value,
    required ValueChanged<bool> onChanged,
    this.isStructural = false,
  }) : selected = value,
       kind = PTFilterKind.toggle,
       options = const [],
       isDefault = !value,
       onChanged = _boolWrap(onChanged);

  /// 是否计入顶栏筛选按钮的激活徽标。
  bool get contributesToBadge => !isDefault && !isStructural;
}

ValueChanged<Object?> _boolWrap(ValueChanged<bool> cb) =>
    (Object? v) => cb(v == true);

/// 弹出多维度筛选面板（宽屏居中对话框 / 窄屏底栏，复用 showAdaptiveSheet）。
///
/// [dimensions] 每次构建传新 List（廉价值对象，勿缓存进 state）——页面状态变了
/// 就重建传新值，bar 徽标 / sheet 初始态才准。sheet 内部交互（点选/拨开关）由
/// sheet 自持本地态驱动，不依赖页面重建。
void showMultiFilterSheet(
  BuildContext context, {
  required String title,
  required List<PTFilterDimension> dimensions,
  bool showClearAll = true,
  VoidCallback? onClearAll,
  double wideWidth = 480,
}) {
  showAdaptiveSheet(
    context,
    title: title,
    wideWidth: wideWidth,
    builder: (_) => _MultiFilterBody(
      dimensions: dimensions,
      showClearAll: showClearAll,
      onClearAll: onClearAll,
    ),
  );
}

class _MultiFilterBody extends StatefulWidget {
  final List<PTFilterDimension> dimensions;
  final bool showClearAll;
  final VoidCallback? onClearAll;
  const _MultiFilterBody({
    required this.dimensions,
    required this.showClearAll,
    required this.onClearAll,
  });

  @override
  State<_MultiFilterBody> createState() => _MultiFilterBodyState();
}

class _MultiFilterBodyState extends State<_MultiFilterBody> {
  // 本地选中态：点选项时同步更新本地（checkmark 跟手）+ 回调页面。
  late final List<Object?> _sel = widget.dimensions
      .map((d) => d.selected)
      .toList();

  void _onChoice(int i, Object? value) {
    setState(() => _sel[i] = value);
    widget.dimensions[i].onChanged(value);
  }

  void _onToggle(int i, bool value) {
    setState(() => _sel[i] = value);
    widget.dimensions[i].onChanged(value);
  }

  @override
  Widget build(BuildContext context) {
    final dims = widget.dimensions;
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        for (int i = 0; i < dims.length; i++) ...[
          _sectionHeader(dims[i].label),
          if (dims[i].kind == PTFilterKind.choice)
            for (final opt in dims[i].options)
              ListTile(
                dense: true,
                contentPadding: const EdgeInsets.symmetric(horizontal: 8),
                title: Text(opt.label, style: const TextStyle(fontSize: 14)),
                trailing: opt.value == _sel[i]
                    ? const Icon(Icons.check, color: PTColors.primary, size: 20)
                    : null,
                onTap: () => _onChoice(i, opt.value),
              )
          else
            SwitchListTile(
              dense: true,
              contentPadding: const EdgeInsets.symmetric(horizontal: 8),
              title: Text(dims[i].label, style: const TextStyle(fontSize: 14)),
              value: _sel[i] == true,
              onChanged: (v) => _onToggle(i, v),
            ),
          if (i < dims.length - 1) const Divider(height: 1, indent: 12),
        ],
        if (widget.showClearAll && widget.onClearAll != null) ...[
          const SizedBox(height: 4),
          Padding(
            padding: const EdgeInsets.only(top: 4, bottom: 2),
            child: Align(
              alignment: Alignment.centerRight,
              child: TextButton.icon(
                onPressed: () {
                  widget.onClearAll!();
                  Navigator.of(context).pop(); // 清空后关 sheet，重开看新态
                },
                icon: const Icon(Icons.clear_all, size: 18),
                label: const Text('清除全部'),
              ),
            ),
          ),
        ],
      ],
    );
  }

  Widget _sectionHeader(String text) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(8, 10, 8, 2),
      child: Text(
        text,
        style: const TextStyle(
          fontSize: 12,
          fontWeight: FontWeight.w700,
          color: PTColors.primary,
        ),
      ),
    );
  }
}

// ─────────────────────────────────────────────────────────────────────────
// 站点筛选：单独一个按钮 + 紧凑弹窗。
// 站点数量多，不适合 showMultiFilterSheet 的「一排一个 ListTile」——这里用 Wrap
// 的 FilterChip 紧凑排列（每行多个），站点多时顶部加搜索框过滤候选。
// 与 showMultiFilterSheet 区别：站点是「单独按钮」不和其他筛选混一个弹窗；
// 单选即关（选了就 pop），符合站点切换的即时预期。
// ─────────────────────────────────────────────────────────────────────────

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

/// 弹出站点多选面板（紧凑 Wrap of FilterChip + 搜索 + 全选/清空）。宽屏对话框 / 窄屏底栏。
/// `options` 只含各站（value 为 siteId）。实时 toggle 回调；点「完成」或遮罩关闭。
void showSitePickerSheet(
  BuildContext context, {
  required String title,
  required List<int> selected,
  required List<PTFilterOption<Object?>> options,
  required ValueChanged<List<int>> onChanged,
  bool single = false,
  bool showSearch = true,
  double wideWidth = 480,
}) {
  showAdaptiveSheet(
    context,
    title: title,
    wideWidth: wideWidth,
    builder: (_) => _SitePickerBody(
      selected: selected,
      options: options,
      onChanged: onChanged,
      single: single,
      showSearch: showSearch && options.length > 8,
    ),
  );
}

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
