// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// _SitesPageState 的站点添加/编辑表单 mixin（从 sites_page.dart 拆出；行为不变）。
part of 'package:ptpatronus/features/sites/sites_page.dart';

mixin SitesFormMixin on SitesCheckinMixin {
  // 宿主 _SitesPageState 提供的钩子。
  Future<void> _load();
  @override
  void _toast(String m);
  // —— 统一添加/编辑表单：showAdaptiveSheet 内 StatefulBuilder ——
  Future<void> _showForm({Site? editing}) async {
    final isEdit = editing != null;
    List<SiteDef> defs = _defs;
    if (!isEdit || defs.isEmpty) {
      try {
        defs = await SiteApi().definitions();
        _defs = defs;
      } catch (e) {
        if (defs.isEmpty) {
          _toast('加载站点定义失败：$e');
          return;
        }
        _toast('站点定义刷新失败，使用当前缓存：$e');
      }
    }
    if (defs.isEmpty) {
      _toast('无可用站点定义');
      return;
    }
    // 可用站在前、死站在后。
    defs = [...defs]..sort((a, b) => a.dead == b.dead ? 0 : (a.dead ? 1 : -1));

    final nameCtrl = TextEditingController(text: isEdit ? editing.name : '');
    final urlCtrl = TextEditingController(text: isEdit ? editing.baseUrl : '');
    final keyCtrl = TextEditingController();
    final passkeyCtrl = TextEditingController();
    final cookieCtrl = TextEditingController();
    final userCtrl = TextEditingController();
    final passCtrl = TextEditingController();

    late SiteDef selected;
    bool active = isEdit ? editing.isActive : true;
    if (isEdit) {
      selected = defs.firstWhere(
        (d) => d.id == editing.definition,
        orElse: () => defs.first,
      );
    } else {
      selected = defs.firstWhere((d) => !d.dead, orElse: () => defs.first);
      nameCtrl.text = selected.name;
      urlCtrl.text = _defaultUrl(selected);
    }

    final knownNames = defs.map((d) => d.name).toSet();
    final knownUrls = <String>{'https://api.m-team.cc', 'https://www.hxpt.org'};
    for (final d in defs) {
      knownUrls.addAll(_defUrls(d));
    }

    if (!mounted) return;
    final submit = await showAdaptiveSheet<bool>(
      context,
      title: isEdit ? '编辑站点' : '添加站点',
      wideWidth: 520,
      builder: (dctx) => StatefulBuilder(
        builder: (fctx, setSt) {
          final isMteam = selected.schema == 'mteam';
          final isFlux = selected.schema == 'fluxtorrent';
          final isApiKeySite = isMteam || selected.schema == 'yemapt' || isFlux;
          final apiKeyHint = isEdit
              ? '留空不修改'
              : (isMteam
                    ? 'M-Team x-api-key（控制台 → 实验室 → 存储令牌）'
                    : isFlux
                    ? 'FluxTorrent API Token（fxo_ 开头，网页端 我的 → API Token 签发）'
                    : 'YemaPT API Auth Key（个人面板 → 详情 → 安全设定）');
          return Padding(
            padding: EdgeInsets.only(
              bottom: MediaQuery.of(fctx).viewInsets.bottom,
            ),
            child: SingleChildScrollView(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                mainAxisSize: MainAxisSize.min,
                children: [
                  TextField(
                    controller: nameCtrl,
                    decoration: const InputDecoration(
                      labelText: '名称',
                      hintText: '如：好学',
                    ),
                  ),
                  const SizedBox(height: 12),
                  if (!isEdit)
                    _DefSearchField(
                      defs: defs,
                      selected: selected,
                      onSelected: (d) {
                        setSt(() {
                          selected = d;
                          if (urlCtrl.text.isEmpty ||
                              knownUrls.contains(urlCtrl.text)) {
                            urlCtrl.text = _defaultUrl(d);
                          }
                          if (nameCtrl.text.isEmpty ||
                              knownNames.contains(nameCtrl.text)) {
                            nameCtrl.text = d.name;
                          }
                        });
                      },
                    )
                  else
                    InputDecorator(
                      decoration: const InputDecoration(labelText: '定义'),
                      child: Text('${selected.name}（${selected.schema}）'),
                    ),
                  const SizedBox(height: 12),
                  if (_defUrls(selected).length > 1) ...[
                    DropdownButtonFormField<String>(
                      initialValue: _defUrls(selected).contains(urlCtrl.text)
                          ? urlCtrl.text
                          : null,
                      isExpanded: true,
                      decoration: const InputDecoration(labelText: '域名'),
                      items: _defUrls(selected)
                          .map(
                            (u) => DropdownMenuItem<String>(
                              value: u,
                              child: Text(u, overflow: TextOverflow.ellipsis),
                            ),
                          )
                          .toList(),
                      onChanged: (v) {
                        if (v == null) return;
                        setSt(() => urlCtrl.text = v);
                      },
                    ),
                    const SizedBox(height: 12),
                  ],
                  TextField(
                    controller: urlCtrl,
                    decoration: InputDecoration(
                      labelText: '地址',
                      hintText: _defaultUrl(selected),
                    ),
                  ),
                  if (isApiKeySite) ...[
                    const SizedBox(height: 12),
                    TextField(
                      controller: keyCtrl,
                      obscureText: true,
                      decoration: InputDecoration(
                        labelText: 'API Key',
                        hintText: apiKeyHint,
                      ),
                    ),
                    // FluxTorrent：passkey 用于下载直链，留空则首次用户信息抓取时自动获取回写
                    if (isFlux) ...[
                      const SizedBox(height: 12),
                      TextField(
                        controller: passkeyCtrl,
                        obscureText: true,
                        decoration: InputDecoration(
                          labelText: 'Passkey',
                          hintText: isEdit ? '留空不修改' : '留空则自动从站点获取',
                        ),
                      ),
                    ],
                  ] else ...[
                    const SizedBox(height: 12),
                    TextField(
                      controller: cookieCtrl,
                      maxLines: 3,
                      decoration: InputDecoration(
                        labelText: 'Cookie',
                        hintText: isEdit
                            ? '留空不修改（或用「登录」WebView 自动取）'
                            : '粘贴站点 cookie（k=v; k2=v2），也可稍后用「登录」自动取',
                      ),
                    ),
                    const SizedBox(height: 12),
                    TextField(
                      controller: userCtrl,
                      decoration: const InputDecoration(
                        labelText: '账号',
                        hintText: '站点用户名（选填）',
                      ),
                    ),
                    const SizedBox(height: 12),
                    TextField(
                      controller: passCtrl,
                      obscureText: true,
                      decoration: InputDecoration(
                        labelText: '密码',
                        hintText: isEdit ? '留空不修改' : '站点密码（选填，加密存储）',
                      ),
                    ),
                  ],
                  SwitchListTile(
                    value: active,
                    onChanged: (v) => setSt(() => active = v),
                    title: const Text('启用'),
                    contentPadding: EdgeInsets.zero,
                    dense: true,
                  ),
                  const SizedBox(height: 16),
                  Row(
                    mainAxisAlignment: MainAxisAlignment.end,
                    children: [
                      TextButton(
                        onPressed: () => Navigator.pop(dctx, false),
                        child: const Text('取消'),
                      ),
                      const SizedBox(width: 8),
                      FilledButton(
                        onPressed: () => Navigator.pop(dctx, true),
                        child: Text(isEdit ? '保存' : '添加'),
                      ),
                    ],
                  ),
                ],
              ),
            ),
          );
        },
      ),
    );

    if (submit != true) return;
    final name = nameCtrl.text.trim();
    final url = urlCtrl.text.trim();
    if (name.isEmpty || url.isEmpty) {
      _toast('名称与地址必填');
      return;
    }
    final isMteam = selected.schema == 'mteam';
    final isFlux = selected.schema == 'fluxtorrent';
    final isApiKeySite = isMteam || selected.schema == 'yemapt' || isFlux;
    if (!isEdit && isApiKeySite && keyCtrl.text.trim().isEmpty) {
      _toast('${selected.name}需填写 API Key');
      return;
    }
    try {
      if (isEdit) {
        await SiteApi().update(
          editing.id,
          name: name,
          platform: editing.platform,
          baseUrl: url,
          apiKey: isApiKeySite && keyCtrl.text.trim().isNotEmpty
              ? keyCtrl.text.trim()
              : null,
          passkey: isFlux && passkeyCtrl.text.trim().isNotEmpty
              ? passkeyCtrl.text.trim()
              : null,
          cookie: !isApiKeySite && cookieCtrl.text.trim().isNotEmpty
              ? cookieCtrl.text.trim()
              : null,
          username: !isApiKeySite && userCtrl.text.trim().isNotEmpty
              ? userCtrl.text.trim()
              : null,
          password: !isApiKeySite && passCtrl.text.trim().isNotEmpty
              ? passCtrl.text.trim()
              : null,
          isActive: active,
        );
        _toast('已更新');
      } else {
        await SiteApi().add(
          name: name,
          platform: selected.schema,
          definition: selected.id,
          baseUrl: url,
          apiKey: isApiKeySite ? keyCtrl.text.trim() : null,
          passkey: isFlux && passkeyCtrl.text.trim().isNotEmpty
              ? passkeyCtrl.text.trim()
              : null,
          cookie: !isApiKeySite && cookieCtrl.text.trim().isNotEmpty
              ? cookieCtrl.text.trim()
              : null,
          username: !isApiKeySite && userCtrl.text.trim().isNotEmpty
              ? userCtrl.text.trim()
              : null,
          password: !isApiKeySite && passCtrl.text.trim().isNotEmpty
              ? passCtrl.text.trim()
              : null,
          isActive: active,
        );
        _toast(
          isFlux
              ? '已添加，API Token 已保存（passkey 将自动获取）'
              : isApiKeySite
              ? '已添加，API Key 已保存'
              : '已添加，请点「登录」获取 cookie',
        );
      }
      await _load();
    } catch (e) {
      _toast('失败：$e');
    }
  }

  String _defaultUrl(SiteDef d) {
    final urls = _defUrls(d);
    if (urls.isNotEmpty) return urls.first;
    return d.schema == 'mteam'
        ? 'https://api.m-team.cc'
        : 'https://www.hxpt.org';
  }

  List<String> _defUrls(SiteDef d) {
    final seen = <String>{};
    final out = <String>[];
    for (final raw in d.urls) {
      final u = raw.trim();
      if (u.isEmpty || !seen.add(u)) continue;
      out.add(u);
    }
    return out;
  }
}
