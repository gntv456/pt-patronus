// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 从 rss_page.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/features/rss/rss_page.dart';

/// 新增/编辑订阅表单（自适应浮层内容）。含体积范围 min_size/max_size（修编辑丢数据 bug）。
class _RssForm extends StatefulWidget {
  final RssSubscription? editing;
  final List<Site> sites;
  final List<Downloader> downloaders;
  final List<String> filterGroups;
  final VoidCallback onSaved;
  const _RssForm({
    this.editing,
    required this.sites,
    required this.downloaders,
    required this.filterGroups,
    required this.onSaved,
  });
  @override
  State<_RssForm> createState() => _RssFormState();
}

class _RssFormState extends State<_RssForm> {
  final _api = RssApi();
  final _formKey = GlobalKey<FormState>();
  late final _name = TextEditingController(text: widget.editing?.name ?? '');
  late final _url = TextEditingController(text: widget.editing?.url ?? '');
  late final _include = TextEditingController(
    text: widget.editing?.titleInclude ?? '',
  );
  late final _exclude = TextEditingController(
    text: widget.editing?.titleExclude ?? '',
  );
  late final _tags = TextEditingController(text: widget.editing?.tags ?? '');
  late final _path = TextEditingController(
    text: widget.editing?.savePath ?? '',
  );
  late final _category = TextEditingController(
    text: widget.editing?.category ?? 'rss',
  );
  late final _minSize = TextEditingController(
    text: '${widget.editing?.minSize ?? 0}',
  );
  late final _maxSize = TextEditingController(
    text: '${widget.editing?.maxSize ?? 0}',
  );
  late final _interval = TextEditingController(
    text: '${widget.editing?.intervalMin ?? 30}',
  );
  int _siteId = 0;
  int _downloaderId = 0;
  String _resolution = '';
  bool _enabled = true;
  bool _autoDownload = false;
  bool _saving = false;
  // 选中的过滤规则组名（提交时 join('|')）。
  final _selectedGroups = <String>{};

  @override
  void initState() {
    super.initState();
    final e = widget.editing;
    if (e != null) {
      _siteId = e.siteId;
      _downloaderId = e.downloaderId;
      _resolution = e.resolution;
      _enabled = e.enabled;
      _autoDownload = e.autoDownload;
      _selectedGroups.addAll(
          e.filterGroups.split('|').map((s) => s.trim()).where((s) => s.isNotEmpty));
    }
  }

  Future<void> _save() async {
    if (!_formKey.currentState!.validate()) return;
    setState(() => _saving = true);
    final body = {
      'name': _name.text.trim(),
      'url': _url.text.trim(),
      'site_id': _siteId,
      'enabled': _enabled,
      'interval_min': int.tryParse(_interval.text.trim()) ?? 30,
      'title_include': _include.text.trim(),
      'title_exclude': _exclude.text.trim(),
      'resolution': _resolution,
      'min_size': int.tryParse(_minSize.text.trim()) ?? 0,
      'max_size': int.tryParse(_maxSize.text.trim()) ?? 0,
      'auto_download': _autoDownload,
      'downloader_id': _downloaderId,
      'save_path': _path.text.trim(),
      'category': _category.text.trim(),
      'tags': _tags.text.trim(),
      'filter_groups': _selectedGroups.join('|'),
    };
    try {
      if (widget.editing != null) {
        await _api.update(widget.editing!.id, body);
      } else {
        await _api.create(body);
      }
      widget.onSaved();
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text('$e')));
      }
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Form(
      key: _formKey,
      child: SingleChildScrollView(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            TextFormField(
              controller: _name,
              decoration: const InputDecoration(labelText: '名称'),
              validator: (v) => (v == null || v.trim().isEmpty) ? '必填' : null,
            ),
            const SizedBox(height: 10),
            TextFormField(
              controller: _url,
              decoration: const InputDecoration(
                labelText: 'URL',
                hintText: 'https://site.com/rss.xml',
              ),
              validator: (v) => (v == null || v.trim().isEmpty) ? '必填' : null,
            ),
            const SizedBox(height: 10),
            DropdownButtonFormField<int>(
              initialValue: _siteId,
              isExpanded: true,
              decoration: const InputDecoration(labelText: '关联站点'),
              items: [
                const DropdownMenuItem(value: 0, child: Text('不关联（匿名 RSS）')),
                ...widget.sites.map(
                  (s) => DropdownMenuItem(value: s.id, child: Text(s.name)),
                ),
              ],
              onChanged: (v) => setState(() => _siteId = v ?? 0),
            ),
            const SizedBox(height: 10),
            TextFormField(
              controller: _interval,
              keyboardType: TextInputType.number,
              decoration: const InputDecoration(labelText: '间隔（分钟，最小 5）'),
              validator: (v) {
                final n = int.tryParse(v ?? '');
                if (n == null || n < 5) return '最小 5 分钟';
                return null;
              },
            ),
            const SizedBox(height: 10),
            TextFormField(
              controller: _include,
              decoration: const InputDecoration(labelText: '标题包含（| 分隔）'),
            ),
            const SizedBox(height: 10),
            TextFormField(
              controller: _exclude,
              decoration: const InputDecoration(labelText: '标题排除（| 分隔）'),
            ),
            const SizedBox(height: 10),
            DropdownButtonFormField<String>(
              initialValue: _resolution,
              isExpanded: true,
              decoration: const InputDecoration(labelText: '分辨率'),
              items: const [
                DropdownMenuItem(value: '', child: Text('不限')),
                DropdownMenuItem(value: '1080p', child: Text('1080p')),
                DropdownMenuItem(value: '2160p', child: Text('2160p')),
              ],
              onChanged: (v) => setState(() => _resolution = v ?? ''),
            ),
            const SizedBox(height: 10),
            Align(
              alignment: Alignment.centerLeft,
              child: Text(
                '过滤规则组（留空=不过滤）',
                style: TextStyle(fontSize: 12, color: context.dimText),
              ),
            ),
            const SizedBox(height: 6),
            widget.filterGroups.isEmpty
                ? Padding(
                    padding: const EdgeInsets.only(bottom: 10),
                    child: Text(
                      '暂无可用规则组，可在「过滤规则」页创建',
                      style: TextStyle(fontSize: 12, color: context.dimText),
                    ),
                  )
                : Padding(
                    padding: const EdgeInsets.only(bottom: 10),
                    child: Wrap(
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
            Row(
              children: [
                Expanded(
                  child: TextFormField(
                    controller: _minSize,
                    keyboardType: TextInputType.number,
                    inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                    decoration: const InputDecoration(labelText: '最小体积'),
                  ),
                ),
                const Padding(
                  padding: EdgeInsets.symmetric(horizontal: 8),
                  child: Text('~'),
                ),
                Expanded(
                  child: TextFormField(
                    controller: _maxSize,
                    keyboardType: TextInputType.number,
                    inputFormatters: [FilteringTextInputFormatter.digitsOnly],
                    decoration: const InputDecoration(labelText: '最大体积'),
                  ),
                ),
              ],
            ),
            Padding(
              padding: const EdgeInsets.only(top: 2, bottom: 8),
              child: Text(
                '字节（0=不限）',
                style: TextStyle(fontSize: 11, color: context.dimText),
              ),
            ),
            SwitchListTile(
              contentPadding: EdgeInsets.zero,
              title: const Text('启用'),
              value: _enabled,
              onChanged: (v) => setState(() => _enabled = v),
            ),
            SwitchListTile(
              contentPadding: EdgeInsets.zero,
              title: const Text('自动下载'),
              value: _autoDownload,
              onChanged: (v) => setState(() => _autoDownload = v),
            ),
            if (_autoDownload) ...[
              DropdownButtonFormField<int>(
                initialValue: _downloaderId,
                isExpanded: true,
                decoration: const InputDecoration(labelText: '下载器'),
                items: [
                  const DropdownMenuItem(value: 0, child: Text('默认下载器')),
                  ...widget.downloaders.map(
                    (d) => DropdownMenuItem(value: d.id, child: Text(d.name)),
                  ),
                ],
                onChanged: (v) => setState(() => _downloaderId = v ?? 0),
              ),
              const SizedBox(height: 10),
              TextFormField(
                controller: _path,
                decoration: const InputDecoration(labelText: '保存路径'),
              ),
              const SizedBox(height: 10),
              TextFormField(
                controller: _category,
                decoration: const InputDecoration(labelText: '分类'),
              ),
              const SizedBox(height: 10),
              TextFormField(
                controller: _tags,
                decoration: const InputDecoration(labelText: '标签（逗号分隔）'),
              ),
            ],
            const SizedBox(height: 16),
            FilledButton(
              onPressed: _saving ? null : _save,
              child: _saving
                  ? const SizedBox(
                      height: 18,
                      width: 18,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Text('保存'),
            ),
          ],
        ),
      ),
    );
  }
}
