// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 从 brushflow_page.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/features/brushflow/brushflow_page.dart';

/// 新建 / 编辑刷流任务的表单（放进 showAdaptiveSheet）。
/// 自持所有 controller / 下拉 / 开关状态；[onSubmit] 返回 true 时表单自关闭。
class _BrushFlowForm extends StatefulWidget {
  final List<Site> sites;
  final List<Downloader> downloaders;
  final List<String> filterGroups;
  final BrushFlowTask? initial;
  final Future<bool> Function(Map<String, dynamic> body) onSubmit;

  const _BrushFlowForm({
    required this.sites,
    required this.downloaders,
    required this.filterGroups,
    required this.onSubmit,
    this.initial,
  });

  @override
  State<_BrushFlowForm> createState() => _BrushFlowFormState();
}

class _BrushFlowFormState extends State<_BrushFlowForm> {
  // 文本字段控制器（initial 非空 = 编辑，预填）。
  late final _nameCtrl = TextEditingController(text: widget.initial?.name ?? '');
  late final _keywordCtrl =
      TextEditingController(text: widget.initial?.keyword ?? '');
  late final _savePathCtrl =
      TextEditingController(text: widget.initial?.savePath ?? '');
  late final _categoryDlCtrl =
      TextEditingController(text: widget.initial?.categoryDl ?? '');
  late final _minSizeCtrl = TextEditingController(
      text: widget.initial == null ? '' : '${widget.initial!.minSizeMb}');
  late final _maxSizeCtrl = TextEditingController(
      text: widget.initial == null ? '' : '${widget.initial!.maxSizeMb}');
  late final _minSeedersCtrl = TextEditingController(
      text: widget.initial == null ? '' : '${widget.initial!.minSeeders}');
  late final _maxSeedersCtrl = TextEditingController(
      text: widget.initial == null ? '' : '${widget.initial!.maxSeeders}');
  late final _delRatioCtrl = TextEditingController(
      text: widget.initial == null ? '' : '${widget.initial!.deleteRatio}');
  late final _delHoursCtrl = TextEditingController(
      text: widget.initial == null ? '' : '${widget.initial!.deleteSeedHours}');
  late final _delUpMbCtrl = TextEditingController(
      text: widget.initial == null ? '' : '${widget.initial!.deleteUploadMb}');
  late final _maxSeedsCtrl = TextEditingController(
      text: widget.initial == null ? '' : '${widget.initial!.maxSeeds}');

  // 下拉 / 开关状态。
  int? _siteId;
  int? _dlId;
  String _category = '综合';
  bool _enabled = true;
  bool _freeOnly = false;
  bool _paused = false;
  bool _deleteFiles = false;
  bool _saving = false;
  // 选中的过滤规则组名（提交时 join('|')）。
  final _selectedGroups = <String>{};

  static const _categories = ['综合', '电影', '电视剧', '音乐'];

  @override
  void initState() {
    super.initState();
    final ini = widget.initial;
    if (ini != null) {
      _siteId = widget.sites.any((s) => s.id == ini.siteId) ? ini.siteId : null;
      _dlId = widget.downloaders.any((d) => d.id == ini.downloaderId)
          ? ini.downloaderId
          : null;
      _category =
          ini.category.isNotEmpty ? ini.category : '综合';
      _enabled = ini.enabled;
      _freeOnly = ini.freeOnly;
      _paused = ini.paused;
      _deleteFiles = ini.deleteFiles;
      _selectedGroups.addAll(
          ini.filterGroups.split('|').map((s) => s.trim()).where((s) => s.isNotEmpty));
    } else {
      _siteId = widget.sites.isNotEmpty ? widget.sites.first.id : null;
      _dlId =
          widget.downloaders.isNotEmpty ? widget.downloaders.first.id : null;
    }
  }

  @override
  void dispose() {
    _nameCtrl.dispose();
    _keywordCtrl.dispose();
    _savePathCtrl.dispose();
    _categoryDlCtrl.dispose();
    _minSizeCtrl.dispose();
    _maxSizeCtrl.dispose();
    _minSeedersCtrl.dispose();
    _maxSeedersCtrl.dispose();
    _delRatioCtrl.dispose();
    _delHoursCtrl.dispose();
    _delUpMbCtrl.dispose();
    _maxSeedsCtrl.dispose();
    super.dispose();
  }


  Future<void> _submit() async {
    if (_nameCtrl.text.trim().isEmpty) {
      _toast('请输入任务名称');
      return;
    }
    if (_siteId == null) {
      _toast('请选择站点');
      return;
    }
    if (_dlId == null) {
      _toast('请选择下载器');
      return;
    }
    setState(() => _saving = true);
    final body = <String, dynamic>{
      'name': _nameCtrl.text.trim(),
      'site_id': _siteId,
      'downloader_id': _dlId,
      'enabled': _enabled,
      'keyword': _keywordCtrl.text.trim(),
      'category': _category,
      'min_size_mb': _parseInt(_minSizeCtrl.text),
      'max_size_mb': _parseInt(_maxSizeCtrl.text),
      'min_seeders': _parseInt(_minSeedersCtrl.text),
      'max_seeders': _parseInt(_maxSeedersCtrl.text),
      'free_only': _freeOnly,
      'paused': _paused,
      'delete_ratio': _parseDouble(_delRatioCtrl.text),
      'delete_seed_hours': _parseDouble(_delHoursCtrl.text),
      'delete_upload_mb': _parseInt(_delUpMbCtrl.text),
      'delete_files': _deleteFiles,
      'max_seeds': _parseInt(_maxSeedsCtrl.text),
      'save_path': _savePathCtrl.text.trim(),
      'category_dl': _categoryDlCtrl.text.trim(),
      'filter_groups': _selectedGroups.join('|'),
    };
    final ok = await widget.onSubmit(body);
    if (!mounted) return;
    setState(() => _saving = false);
    if (ok) Navigator.of(context).pop();
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      mainAxisSize: MainAxisSize.min,
      children: [
        _field('任务名称', TextField(
          controller: _nameCtrl,
          decoration: const InputDecoration(
            isDense: true,
            hintText: '如：馒头刷流',
            border: OutlineInputBorder(),
          ),
        )),
        _field(
          '站点',
          DropdownButton<int>(
            value: _siteId,
            isExpanded: true,
            hint: const Text('选择站点'),
            items: widget.sites
                .map((s) => DropdownMenuItem(value: s.id, child: Text(s.name)))
                .toList(),
            onChanged: (v) => setState(() => _siteId = v),
          ),
        ),
        _field(
          '下载器',
          DropdownButton<int>(
            value: _dlId,
            isExpanded: true,
            hint: const Text('选择下载器'),
            items: widget.downloaders
                .map((d) => DropdownMenuItem(value: d.id, child: Text(d.name)))
                .toList(),
            onChanged: (v) => setState(() => _dlId = v),
          ),
        ),
        _field(
          '分类',
          DropdownButton<String>(
            value: _category,
            isExpanded: true,
            items: _categories
                .map((c) => DropdownMenuItem(value: c, child: Text(c)))
                .toList(),
            onChanged: (v) => setState(() => _category = v ?? '综合'),
          ),
        ),
        _field('关键字', TextField(
          controller: _keywordCtrl,
          decoration: const InputDecoration(
            isDense: true,
            hintText: '标题关键字（留空=不限）',
            border: OutlineInputBorder(),
          ),
        )),
        _field(
          '大小范围（MB，留空=不限）',
          Row(
            children: [
              Expanded(
                child: TextField(
                  controller: _minSizeCtrl,
                  keyboardType: TextInputType.number,
                  decoration: const InputDecoration(
                    isDense: true,
                    hintText: '最小',
                    border: OutlineInputBorder(),
                  ),
                ),
              ),
              const Padding(
                padding: EdgeInsets.symmetric(horizontal: 8),
                child: Text('—'),
              ),
              Expanded(
                child: TextField(
                  controller: _maxSizeCtrl,
                  keyboardType: TextInputType.number,
                  decoration: const InputDecoration(
                    isDense: true,
                    hintText: '最大',
                    border: OutlineInputBorder(),
                  ),
                ),
              ),
            ],
          ),
        ),
        _field(
          '做种人数范围（留空=不限）',
          Row(
            children: [
              Expanded(
                child: TextField(
                  controller: _minSeedersCtrl,
                  keyboardType: TextInputType.number,
                  decoration: const InputDecoration(
                    isDense: true,
                    hintText: '最少',
                    border: OutlineInputBorder(),
                  ),
                ),
              ),
              const Padding(
                padding: EdgeInsets.symmetric(horizontal: 8),
                child: Text('—'),
              ),
              Expanded(
                child: TextField(
                  controller: _maxSeedersCtrl,
                  keyboardType: TextInputType.number,
                  decoration: const InputDecoration(
                    isDense: true,
                    hintText: '最多',
                    border: OutlineInputBorder(),
                  ),
                ),
              ),
            ],
          ),
        ),
        _switchRow('启用', _enabled, (v) => setState(() => _enabled = v)),
        _switchRow('仅免费', _freeOnly, (v) => setState(() => _freeOnly = v)),
        _field(
          '过滤规则组（留空=不过滤）',
          widget.filterGroups.isEmpty
              ? Text('暂无可用规则组，可在「过滤规则」页创建',
                  style: TextStyle(fontSize: 12, color: context.dimText))
              : Wrap(
                  spacing: 8,
                  runSpacing: 4,
                  children: [
                    for (final g in widget.filterGroups)
                      FilterChip(
                        label: Text(g),
                        selected: _selectedGroups.contains(g),
                        onSelected: (v) => setState(() => v
                            ? _selectedGroups.add(g)
                            : _selectedGroups.remove(g)),
                      ),
                  ],
                ),
        ),
        _switchRow(
            '添加后暂停', _paused, (v) => setState(() => _paused = v)),
        const Divider(height: 20),
        const Text(
          '删种规则',
          style: TextStyle(fontWeight: FontWeight.w600, fontSize: 13),
        ),
        const SizedBox(height: 8),
        _field(
          '分享率上限 / 保种小时 / 上传 MB（留空=不限）',
          Row(
            children: [
              Expanded(
                child: TextField(
                  controller: _delRatioCtrl,
                  keyboardType: const TextInputType.numberWithOptions(decimal: true),
                  decoration: const InputDecoration(
                    isDense: true,
                    hintText: '分享率',
                    border: OutlineInputBorder(),
                  ),
                ),
              ),
              const SizedBox(width: 8),
              Expanded(
                child: TextField(
                  controller: _delHoursCtrl,
                  keyboardType: const TextInputType.numberWithOptions(decimal: true),
                  decoration: const InputDecoration(
                    isDense: true,
                    hintText: '小时',
                    border: OutlineInputBorder(),
                  ),
                ),
              ),
              const SizedBox(width: 8),
              Expanded(
                child: TextField(
                  controller: _delUpMbCtrl,
                  keyboardType: TextInputType.number,
                  decoration: const InputDecoration(
                    isDense: true,
                    hintText: '上传MB',
                    border: OutlineInputBorder(),
                  ),
                ),
              ),
            ],
          ),
        ),
        _switchRow('删种同时删除文件', _deleteFiles,
            (v) => setState(() => _deleteFiles = v)),
        _field('最大做种数（留空=不限）', TextField(
          controller: _maxSeedsCtrl,
          keyboardType: TextInputType.number,
          decoration: const InputDecoration(
            isDense: true,
            hintText: '如：20',
            border: OutlineInputBorder(),
          ),
        )),
        _field('保存路径（留空=下载器默认）', TextField(
          controller: _savePathCtrl,
          decoration: const InputDecoration(
            isDense: true,
            hintText: '/downloads/brush',
            border: OutlineInputBorder(),
          ),
        )),
        _field('下载器分类（留空=不限）', TextField(
          controller: _categoryDlCtrl,
          decoration: const InputDecoration(
            isDense: true,
            hintText: '如：brush',
            border: OutlineInputBorder(),
          ),
        )),
        const SizedBox(height: 12),
        FilledButton.icon(
          onPressed: _saving ? null : _submit,
          icon: _saving
              ? const SizedBox(
                  width: 16,
                  height: 16,
                  child: CircularProgressIndicator(strokeWidth: 2),
                )
              : const Icon(Icons.save_outlined, size: 18),
          label: Text(widget.initial == null ? '创建任务' : '保存修改'),
        ),
      ],
    );
  }


}
