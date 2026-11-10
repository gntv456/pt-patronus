// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 站点定义选择器（从 sites_page.dart 拆出，part of 主库；行为不变）。
part of 'package:ptpatronus/features/sites/sites_page.dart';

/// 站点定义选择器（添加站点用）。点按「定义」栏 → 打开独立搜索选择面板（选择框）：
/// 面板内输入检索 + 候选列表，选完回填。286 个定义需检索；与表单隔离（独立 sheet），
/// 避免在表单内联列表导致的输入/滚动/焦点冲突。
class _DefSearchField extends StatelessWidget {
  final List<SiteDef> defs;
  final SiteDef selected;
  final ValueChanged<SiteDef> onSelected;
  const _DefSearchField({
    required this.defs,
    required this.selected,
    required this.onSelected,
  });

  @override
  Widget build(BuildContext context) {
    return InkWell(
      onTap: () async {
        final picked = await showAdaptiveSheet<SiteDef>(
          context,
          title: '选择站点定义',
          wideWidth: 480,
          builder: (_) => _DefPickerBody(defs: defs, selected: selected),
        );
        if (picked != null) onSelected(picked);
      },
      child: InputDecorator(
        decoration: const InputDecoration(
          labelText: '定义',
          suffixIcon: Icon(Icons.arrow_drop_down),
        ),
        child: Text(
          '${selected.name}（${selected.schema}）',
          overflow: TextOverflow.ellipsis,
        ),
      ),
    );
  }
}

/// 定义搜索选择面板内容：搜索框（自动聚焦）+ 候选列表。空查询显提示（不一次渲染 286 条）。
class _DefPickerBody extends StatefulWidget {
  final List<SiteDef> defs;
  final SiteDef selected;
  const _DefPickerBody({required this.defs, required this.selected});
  @override
  State<_DefPickerBody> createState() => _DefPickerBodyState();
}

class _DefPickerBodyState extends State<_DefPickerBody> {
  final _ctrl = TextEditingController();

  @override
  void dispose() {
    _ctrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final q = _ctrl.text.trim();
    final matches = q.isEmpty
        ? filterSiteDefsBySearch(widget.defs, '', limit: null)
        : filterSiteDefsBySearch(widget.defs, q, limit: 80);
    final listHeight = (MediaQuery.of(context).size.height * 0.55)
        .clamp(220.0, 460.0)
        .toDouble();
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Padding(
          padding: const EdgeInsets.only(bottom: 8),
          child: TextField(
            controller: _ctrl,
            autofocus: true,
            textInputAction: TextInputAction.search,
            decoration: const InputDecoration(
              hintText: '输入名称 / 域名搜索…',
              prefixIcon: Icon(Icons.search, size: 18),
              isDense: true,
              border: OutlineInputBorder(),
            ),
            onChanged: (_) => setState(() {}),
          ),
        ),
        if (matches.isEmpty)
          Padding(
            padding: EdgeInsets.symmetric(vertical: 24),
            child: Center(
              child: Text(
                '无匹配',
                style: TextStyle(color: context.dimText, fontSize: 13),
              ),
            ),
          )
        else
          SizedBox(
            height: listHeight,
            child: ListView.builder(
              padding: EdgeInsets.zero,
              itemCount: matches.length,
              itemBuilder: (_, i) {
                final d = matches[i];
                return ListTile(
                  title: Text('${d.name}（${d.schema}）'),
                  subtitle: (_candidateSubtitle(d).isNotEmpty)
                      ? Text(
                          _candidateSubtitle(d),
                          style: TextStyle(
                            fontSize: 11,
                            color: d.dead ? context.dimText : context.mainText,
                          ),
                        )
                      : null,
                  trailing: d.id == widget.selected.id
                      ? const Icon(
                          Icons.check_circle,
                          color: PTColors.primary,
                          size: 20,
                        )
                      : null,
                  onTap: () => Navigator.of(context).pop(d),
                );
              },
            ),
          ),
      ],
    );
  }

  String _candidateSubtitle(SiteDef d) {
    return [...d.urls, ...d.aka, if (d.dead) '已关站'].join(' · ');
  }
}
