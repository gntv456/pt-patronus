// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter/material.dart';
import 'package:ptpatronus/api/plugin.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';

/// 移动端插件页：查看与启停已安装插件；复杂配置/动作/市场明确引导到 Web。
class PluginPage extends StatefulWidget {
  const PluginPage({super.key});
  @override
  State<PluginPage> createState() => _PluginPageState();
}

class _PluginPageState extends State<PluginPage> {
  final _api = PluginApi();
  List<PluginInfo> _items = [];
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() => _loading = true);
    try {
      _items = await _api.list();
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('$e')));
      }
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _toggle(PluginInfo p) async {
    try {
      if (p.enabled) {
        await _api.disable(p);
      } else {
        await _api.enable(p);
      }
      await _load();
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('$e')));
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    return PTPageScaffold(
      title: '插件',
      body: _loading
          ? const Center(child: CircularProgressIndicator())
          : ListView(
              padding: const EdgeInsets.all(12),
              children: [
                GlassCard(
                  child: Padding(
                    padding: const EdgeInsets.all(12),
                    child: Text(
                      '插件配置、动作执行与市场安装请在 Web 端「插件」页完成；移动端可查看状态并启停。',
                      style: TextStyle(color: context.dimText, fontSize: 12),
                    ),
                  ),
                ),
                const SizedBox(height: 12),
                ..._items.map((p) => GlassCard(
                      child: ListTile(
                        title: Text(p.name),
                        subtitle: Text('v${p.version} · ${p.description}', maxLines: 2),
                        trailing: Switch(
                          value: p.enabled,
                          onChanged: (_) => _toggle(p),
                        ),
                      ),
                    )),
              ],
            ),
    );
  }
}
