import 'package:flutter/material.dart';
import 'package:ptpatronus/api/site.dart';
import 'package:ptpatronus/api/downloader.dart';
import 'package:ptpatronus/api/workflow.dart';

/// 节点配置底部表单：按 ParamSchema 渲染字段，就地编辑 node.name / node.params。
/// 返回 true 表示已保存（调用方据此刷新画布）。
Future<bool> showNodeConfigSheet(
  BuildContext context,
  GraphNode node,
  NodeInfo info,
) async {
  final saved = await showModalBottomSheet<bool>(
    context: context,
    isScrollControlled: true,
    builder: (_) => _NodeConfigSheet(node: node, info: info),
  );
  return saved ?? false;
}

class _NodeConfigSheet extends StatefulWidget {
  final GraphNode node;
  final NodeInfo info;
  const _NodeConfigSheet({required this.node, required this.info});
  @override
  State<_NodeConfigSheet> createState() => _NodeConfigSheetState();
}

class _NodeConfigSheetState extends State<_NodeConfigSheet> {
  late final TextEditingController _nameCtl;
  late final Map<String, TextEditingController> _textCtls;
  List<Site> _sites = const [];
  List<Downloader> _downloaders = const [];

  @override
  void initState() {
    super.initState();
    _nameCtl = TextEditingController(text: widget.node.name);
    _textCtls = {};
    for (final f in widget.info.paramSchema) {
      if (f.type == 'text' || f.type == 'textarea' || f.type == 'kv') {
        final v = f.type == 'kv'
            ? _kvToText(widget.node.params[f.name])
            : '${widget.node.params[f.name] ?? ''}';
        _textCtls[f.name] = TextEditingController(text: v);
      }
    }
    if (widget.info.paramSchema.any((f) => f.type == 'site_select')) {
      SiteApi().list().then((l) => mounted ? setState(() => _sites = l) : null).catchError((_) {});
    }
    if (widget.info.paramSchema.any((f) => f.type == 'downloader_select')) {
      DownloaderApi()
          .list()
          .then((l) => mounted ? setState(() => _downloaders = l) : null)
          .catchError((_) {});
    }
  }

  @override
  void dispose() {
    _nameCtl.dispose();
    for (final c in _textCtls.values) {
      c.dispose();
    }
    super.dispose();
  }

  dynamic _get(String key, dynamic def) => widget.node.params[key] ?? def;

  @override
  Widget build(BuildContext context) {
    final schema = widget.info.paramSchema;
    return Padding(
      padding: EdgeInsets.fromLTRB(
          16, 12, 16, MediaQuery.of(context).viewInsets.bottom + 16),
      child: SingleChildScrollView(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text(widget.info.name,
                style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w700)),
            Text(widget.info.description,
                style: const TextStyle(fontSize: 12, color: Colors.grey)),
            const SizedBox(height: 12),
            TextField(
              controller: _nameCtl,
              decoration: const InputDecoration(
                  labelText: '节点名称', border: OutlineInputBorder(), isDense: true),
            ),
            const SizedBox(height: 12),
            for (final f in schema) ...[
              _field(f),
              const SizedBox(height: 10),
            ],
            const SizedBox(height: 6),
            FilledButton(
              onPressed: () {
                widget.node.name = _nameCtl.text.trim().isEmpty
                    ? widget.info.name
                    : _nameCtl.text.trim();
                for (final entry in _textCtls.entries) {
                  widget.node.params[entry.key] = entry.value.text;
                }
                Navigator.pop(context, true);
              },
              child: const Text('保存'),
            ),
          ],
        ),
      ),
    );
  }

  Widget _field(ParamField f) {
    switch (f.type) {
      case 'text':
      case 'textarea':
        return TextField(
          controller: _textCtls[f.name],
          maxLines: f.type == 'textarea' ? 3 : 1,
          decoration: InputDecoration(
            labelText: f.label + (f.required ? ' *' : ''),
            helperText: f.help.isEmpty ? null : f.help,
            border: const OutlineInputBorder(),
            isDense: true,
          ),
        );
      case 'number':
        return TextFormField(
          initialValue: '${_get(f.name, f.defaultValue ?? 0)}',
          keyboardType: TextInputType.number,
          decoration: InputDecoration(
            labelText: f.label,
            helperText: f.help.isEmpty ? null : f.help,
            border: const OutlineInputBorder(),
            isDense: true,
          ),
          onChanged: (v) {
            final n = num.tryParse(v);
            widget.node.params[f.name] = n?.toDouble() ?? 0;
          },
        );
      case 'switch':
        return SwitchListTile(
          contentPadding: EdgeInsets.zero,
          title: Text(f.label),
          value: _get(f.name, f.defaultValue ?? false) == true,
          onChanged: (v) => setState(() => widget.node.params[f.name] = v),
        );
      case 'select':
        return Row(
          children: [
            Text('${f.label}：'),
            Expanded(
              child: DropdownButton<dynamic>(
                isExpanded: true,
                value: widget.node.params[f.name] ?? f.defaultValue ?? (f.options.isNotEmpty ? f.options.first.value : null),
                items: f.options
                    .map((o) => DropdownMenuItem<dynamic>(
                          value: o.value,
                          child: Text(o.label),
                        ))
                    .toList(),
                onChanged: (v) => setState(() => widget.node.params[f.name] = v),
              ),
            ),
          ],
        );
      case 'site_select':
        return Row(
          children: [
            Text('${f.label}：'),
            Expanded(
              child: DropdownButton<int>(
                isExpanded: true,
                value: (_get(f.name, 0) as num?)?.toInt() ?? 0,
                items: [
                  const DropdownMenuItem(value: 0, child: Text('不关联')),
                  ..._sites.map((s) => DropdownMenuItem(value: s.id, child: Text(s.name))),
                ],
                onChanged: (v) => setState(() => widget.node.params[f.name] = v ?? 0),
              ),
            ),
          ],
        );
      case 'downloader_select':
        return Row(
          children: [
            Text('${f.label}：'),
            Expanded(
              child: DropdownButton<int>(
                isExpanded: true,
                value: (_get(f.name, 0) as num?)?.toInt() ?? 0,
                items: [
                  const DropdownMenuItem(value: 0, child: Text('默认')),
                  ..._downloaders.map((d) => DropdownMenuItem(value: d.id, child: Text(d.name))),
                ],
                onChanged: (v) => setState(() => widget.node.params[f.name] = v ?? 0),
              ),
            ),
          ],
        );
      case 'kv':
        // 简化：多行 key:value 文本
        return TextField(
          controller: _textCtls[f.name],
          maxLines: 3,
          decoration: InputDecoration(
            labelText: f.label,
            helperText: '每行 key:value',
            border: const OutlineInputBorder(),
            isDense: true,
          ),
          onChanged: (v) => widget.node.params[f.name] = _parseKv(v),
        );
      default:
        return const SizedBox.shrink();
    }
  }
}

String _kvToText(dynamic m) {
  if (m is! Map) return '';
  return m.entries.map((e) => '${e.key}:${e.value}').join('\n');
}

Map<String, dynamic> _parseKv(String text) {
  final out = <String, dynamic>{};
  for (final line in text.split('\n')) {
    final i = line.indexOf(':');
    if (i > 0) {
      out[line.substring(0, i).trim()] = line.substring(i + 1).trim();
    }
  }
  return out;
}
