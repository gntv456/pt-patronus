import 'package:flutter/material.dart';
import 'package:ptpatronus/features/home/home_menu_button.dart';
import 'package:ptpatronus/api/site.dart';
import 'package:ptpatronus/theme/glass.dart';
import '../webview_login/webview_login_page.dart';

class SitesPage extends StatefulWidget {
  const SitesPage({super.key});
  @override
  State<SitesPage> createState() => _SitesPageState();
}

class _SitesPageState extends State<SitesPage> {
  List<Site> _sites = [];
  bool _loading = false;
  String? _error; // 加载失败时的友好文案（ApiException.toString 已友好化）；null=无错误
  int? _checkining; // 正在签到的站点 id（单站签到 loading 指示）
  bool _checkinAllLoading = false; // 一键签到 loading

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      _sites = await SiteApi().list();
    } catch (e) {
      // 保留已加载的旧数据；首次加载（_sites 空）时展示错误态 + 重试，而非空白
      if (_sites.isEmpty) {
        _error = e.toString();
      } else {
        _toast(e.toString());
      }
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  Widget _errorView() => Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.error_outline, size: 40, color: Colors.redAccent),
            const SizedBox(height: 8),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 32),
              child: Text(_error ?? '加载失败',
                  textAlign: TextAlign.center,
                  style: const TextStyle(color: Colors.redAccent, fontSize: 14)),
            ),
            const SizedBox(height: 12),
            FilledButton.tonalIcon(
              icon: const Icon(Icons.refresh, size: 18),
              label: const Text('重试'),
              onPressed: _load,
            ),
          ],
        ),
      );

  Widget _emptyView() => const Center(
        child: Text('暂无站点，点右上角 + 添加', style: TextStyle(color: Colors.grey)),
      );

  Future<void> _add() async {
    List<SiteDef> defs;
    try {
      defs = await SiteApi().definitions();
    } catch (e) {
      _toast('加载站点定义失败：$e');
      return;
    }
    if (defs.isEmpty) {
      _toast('无可用站点定义');
      return;
    }
    // 可用站在前、死站在后（死站保留可选，仅标注；排序后默认选中的 defs.first 不会是死站）。
    defs = [...defs]..sort((a, b) => a.dead == b.dead ? 0 : (a.dead ? 1 : -1));
    _showAddDialog(defs);
  }

  // 添加站点表单：选定义（据 schema 推断 platform）→ 馒头填 API Key、其余填地址。
  // 用 BottomSheet 而非 AlertDialog：手机上更贴底、键盘弹起按 viewInsets 抬高不遮挡。
  Future<void> _showAddDialog(List<SiteDef> defs) async {
    final nameCtrl = TextEditingController();
    final urlCtrl = TextEditingController();
    final keyCtrl = TextEditingController();
    SiteDef selected = defs.first;
    // 默认地址：取定义首个真实 URL（馒头 urls[0] 即 api.m-team.cc），无则回退占位。
    String defaultUrl(SiteDef d) {
      if (d.urls.isNotEmpty) return d.urls.first;
      return d.schema == 'mteam' ? 'https://api.m-team.cc' : 'https://hxpt.org';
    }
    // 名称/地址仍等于任一定义默认值（或为空）时视为「未手填」，切定义时跟随更新，不覆盖用户输入。
    final knownNames = defs.map((d) => d.name).toSet();
    final knownUrls = <String>{};
    for (final d in defs) {
      knownUrls.add(defaultUrl(d));
    }
    knownUrls.addAll(const ['https://api.m-team.cc', 'https://hxpt.org']);
    bool isDefaultName(String v) => v.isEmpty || knownNames.contains(v);
    bool isDefaultUrl(String v) => v.isEmpty || knownUrls.contains(v);
    nameCtrl.text = selected.name;
    urlCtrl.text = defaultUrl(selected);

    final ok = await showModalBottomSheet<bool>(
      context: context,
      isScrollControlled: true,
      showDragHandle: true,
      builder: (dctx) => StatefulBuilder(
        builder: (_, setSt) => Padding(
          // 键盘避让：sheet 底部按视图插入区抬高，输入框不被软键盘遮挡。
          padding: EdgeInsets.only(bottom: MediaQuery.of(dctx).viewInsets.bottom),
          child: SingleChildScrollView(
            padding: const EdgeInsets.fromLTRB(20, 0, 20, 20),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: [
                const Text('添加站点',
                    style: TextStyle(fontSize: 18, fontWeight: FontWeight.w700)),
                const SizedBox(height: 16),
                TextField(
                    controller: nameCtrl,
                    decoration: const InputDecoration(labelText: '名称', hintText: '如：好学')),
                const SizedBox(height: 12),
                DropdownButtonFormField<SiteDef>(
                  initialValue: selected,
                  isExpanded: true,
                  decoration: const InputDecoration(labelText: '定义'),
                  items: defs
                      .map((d) => DropdownMenuItem(
                            value: d,
                            child: Row(mainAxisSize: MainAxisSize.min, children: [
                              Text('${d.name}（${d.schema}）'),
                              if (d.dead) ...[
                                const SizedBox(width: 6),
                                Container(
                                  padding: const EdgeInsets.symmetric(horizontal: 5, vertical: 1),
                                  decoration: BoxDecoration(
                                    color: Colors.grey.withValues(alpha: 0.2),
                                    borderRadius: BorderRadius.circular(4),
                                  ),
                                  child: const Text('已关站',
                                      style: TextStyle(fontSize: 11, color: Colors.grey)),
                                ),
                              ],
                            ]),
                          ))
                      .toList(),
                  onChanged: (d) {
                    if (d == null) return;
                    setSt(() {
                      selected = d;
                      if (isDefaultUrl(urlCtrl.text)) urlCtrl.text = defaultUrl(d);
                      if (isDefaultName(nameCtrl.text)) nameCtrl.text = d.name;
                    });
                  },
                ),
                const SizedBox(height: 12),
                TextField(
                  controller: urlCtrl,
                  decoration: InputDecoration(labelText: '地址', hintText: defaultUrl(selected)),
                ),
                if (selected.schema == 'mteam') ...[
                  const SizedBox(height: 12),
                  TextField(
                    controller: keyCtrl,
                    obscureText: true,
                    decoration: const InputDecoration(
                      labelText: 'API Key',
                      hintText: 'M-Team x-api-key（控制台 → 实验室 → 存储令牌）',
                    ),
                  ),
                ],
                const SizedBox(height: 20),
                Row(
                  mainAxisAlignment: MainAxisAlignment.end,
                  children: [
                    TextButton(onPressed: () => Navigator.pop(dctx, false), child: const Text('取消')),
                    const SizedBox(width: 8),
                    FilledButton(onPressed: () => Navigator.pop(dctx, true), child: const Text('添加')),
                  ],
                ),
              ],
            ),
          ),
        ),
      ),
    );

    if (ok != true) return;
    final name = nameCtrl.text.trim();
    final url = urlCtrl.text.trim();
    final key = keyCtrl.text.trim();
    if (name.isEmpty || url.isEmpty) {
      _toast('名称与地址必填');
      return;
    }
    if (selected.schema == 'mteam' && key.isEmpty) {
      _toast('馒头需填写 API Key');
      return;
    }
    try {
      await SiteApi().add(
        name: name,
        platform: selected.schema,
        definition: selected.id,
        baseUrl: url,
        apiKey: selected.schema == 'mteam' ? key : null,
      );
      _toast(selected.schema == 'mteam'
          ? '已添加馒头，API Key 已保存'
          : '已添加，请点「登录」获取 cookie');
      await _load();
    } catch (e) {
      _toast('失败：$e');
    }
  }

  void _login(Site s) {
    Navigator.of(context).push(MaterialPageRoute(
      builder: (_) => WebViewLoginPage(site: s),
    ));
  }

  // 馒头走 x-api-key，无 cookie 登录概念；列表按钮改为连接测试以验证令牌。
  Future<void> _testConnection(Site s) async {
    try {
      final r = await SiteApi().test(s.id);
      if (r['ok'] == true) {
        _toast('${s.name}：连接正常，等级 ${r['level'] ?? '-'}');
      } else {
        _toast('${s.name}：${r['error'] ?? '连接失败'}');
      }
    } catch (e) {
      _toast('请求失败：$e');
    }
  }

  // 签到结果 → 友好文案（成功/已签/不支持/未登录/失败）。
  String _checkinMsg(Map<String, dynamic> r) {
    final name = r['site_name'] as String? ?? '';
    final success = r['success'] == true;
    final message = r['message'] as String? ?? '';
    final bonus = (r['bonus'] as num?)?.toDouble() ?? 0;
    final error = r['error'] as String? ?? '';
    if (success) {
      if (bonus > 0) return '$name：签到成功，获得 $bonus 魔力';
      if (message.contains('已签到')) return '$name：今天已签到';
      return '$name：${message.isNotEmpty ? message : '签到成功'}';
    }
    if (error.contains('not supported')) return '$name：该站点暂不支持签到';
    if (error.contains('not logged in') || message.contains('not logged in')) {
      return '$name：未登录，请先登录';
    }
    return '$name：${error.isNotEmpty ? error : (message.isNotEmpty ? message : '签到失败')}';
  }

  Future<void> _checkin(Site s) async {
    setState(() => _checkining = s.id);
    try {
      final r = await SiteApi().checkin(s.id);
      _toast(_checkinMsg(r));
    } catch (e) {
      _toast('${s.name}：请求失败：$e');
    } finally {
      if (mounted) setState(() => _checkining = null);
    }
  }

  Future<void> _checkinAll() async {
    setState(() => _checkinAllLoading = true);
    try {
      final results = await SiteApi().checkinAll();
      if (results.isEmpty) {
        _toast('没有启用的站点');
        return;
      }
      var ok = 0, already = 0;
      final failed = <String>[];
      for (final r in results) {
        if (r['success'] == true) {
          if ((r['message'] as String? ?? '').contains('已签到')) {
            already++;
          } else {
            ok++;
          }
        } else if (!(r['error'] as String? ?? '').contains('not supported')) {
          failed.add(r['site_name'] as String? ?? '?');
        }
      }
      final summary = '成功 $ok / 已签 $already';
      _toast(failed.isEmpty
          ? '一键签到完成：$summary'
          : '一键签到完成：$summary / 失败 ${failed.length}（${failed.join('、')}）');
    } catch (e) {
      _toast('一键签到失败：$e');
    } finally {
      if (mounted) setState(() => _checkinAllLoading = false);
    }
  }

  // 签到记录弹窗：列最近 100 条（cron 自动 + 手动），按时间倒序。
  Future<void> _showRecords() async {
    List<CheckinRecord> recs;
    try {
      recs = await SiteApi().records();
    } catch (e) {
      _toast('加载签到记录失败：$e');
      return;
    }
    if (!mounted) return;
    showDialog(
      context: context,
      builder: (dctx) => AlertDialog(
        title: const Text('签到记录'),
        content: SizedBox(
          width: double.maxFinite,
          child: recs.isEmpty
              ? const Padding(
                  padding: EdgeInsets.symmetric(vertical: 24),
                  child: Text('暂无签到记录',
                      textAlign: TextAlign.center, style: TextStyle(color: Colors.grey)),
                )
              : ListView.builder(
                  shrinkWrap: true,
                  itemCount: recs.length,
                  itemBuilder: (_, i) {
                    final r = recs[i];
                    final resultText = r.success
                        ? (r.bonus > 0
                            ? '+${r.bonus} 魔力'
                            : (r.message.isNotEmpty ? r.message : '成功'))
                        : (r.error.isNotEmpty
                            ? r.error
                            : (r.message.isNotEmpty ? r.message : '失败'));
                    return ListTile(
                      dense: true,
                      leading: Icon(
                        r.success ? Icons.check_circle_outline : Icons.error_outline,
                        size: 20,
                        color: r.success ? Colors.green : Colors.redAccent,
                      ),
                      title: Text('${r.siteName}　·　${r.source == 'cron' ? '自动' : '手动'}',
                          style: const TextStyle(fontSize: 14)),
                      subtitle: Text(resultText, style: const TextStyle(fontSize: 12)),
                      trailing: Text(
                        _fmtRecordTime(r.createdAt),
                        style: const TextStyle(fontSize: 11, color: Colors.grey),
                      ),
                    );
                  },
                ),
        ),
        actions: [
          TextButton(onPressed: () => Navigator.pop(dctx), child: const Text('关闭')),
        ],
      ),
    );
  }

  String _fmtRecordTime(String iso) {
    if (iso.isEmpty) return '';
    final d = DateTime.tryParse(iso);
    if (d == null) return iso;
    final local = d.toLocal();
    String p(int n) => n.toString().padLeft(2, '0');
    return '${local.year}-${p(local.month)}-${p(local.day)} ${p(local.hour)}:${p(local.minute)}';
  }

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  Widget build(BuildContext context) {
    return GlassBackground(
      child: SafeArea(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 12, 16, 8),
              child: Row(children: [
                const HomeMenuButton(),
                const Text('站点', style: TextStyle(fontSize: 22, fontWeight: FontWeight.w700)),
                const Spacer(),
                IconButton(
                  tooltip: '签到记录',
                  icon: const Icon(Icons.history),
                  onPressed: _showRecords,
                ),
                IconButton(
                  tooltip: '一键签到',
                  onPressed: _checkinAllLoading ? null : _checkinAll,
                  icon: _checkinAllLoading
                      ? const SizedBox(
                          width: 20, height: 20, child: CircularProgressIndicator(strokeWidth: 2))
                      : const Icon(Icons.card_giftcard),
                ),
                IconButton(icon: const Icon(Icons.refresh), onPressed: _load),
                IconButton(icon: const Icon(Icons.add), onPressed: _add),
              ]),
            ),
            if (_loading) const LinearProgressIndicator(minHeight: 2),
            Expanded(
              child: _sites.isEmpty && !_loading
                  ? (_error != null ? _errorView() : _emptyView())
                  : RefreshIndicator(
                      onRefresh: _load,
                      child: ListView.builder(
                      padding: const EdgeInsets.all(12),
                      itemCount: _sites.length,
                      itemBuilder: (_, i) {
                        final s = _sites[i];
                        return Padding(
                          padding: const EdgeInsets.only(bottom: 10),
                          child: GlassCard(
                            padding: const EdgeInsets.fromLTRB(14, 12, 12, 12),
                            child: Row(
                              children: [
                                Expanded(
                                  child: Column(
                                    crossAxisAlignment: CrossAxisAlignment.start,
                                    children: [
                                      Text(s.name, style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 16)),
                                      const SizedBox(height: 2),
                                      Text(s.baseUrl, style: const TextStyle(color: Colors.grey, fontSize: 12)),
                                    ],
                                  ),
                                ),
                                if (s.platform == 'nexusphp')
                                  IconButton(
                                    tooltip: '签到',
                                    onPressed: _checkining == s.id ? null : () => _checkin(s),
                                    icon: _checkining == s.id
                                        ? const SizedBox(
                                            width: 18,
                                            height: 18,
                                            child: CircularProgressIndicator(strokeWidth: 2))
                                        : const Icon(Icons.card_giftcard_outlined, size: 20),
                                  ),
                                if (s.platform == 'mteam')
                                  FilledButton.tonalIcon(
                                    icon: const Icon(Icons.bolt, size: 16),
                                    label: const Text('测试'),
                                    onPressed: () => _testConnection(s),
                                  )
                                else
                                  FilledButton.tonalIcon(
                                    icon: const Icon(Icons.login, size: 16),
                                    label: const Text('登录'),
                                    onPressed: () => _login(s),
                                  ),
                              ],
                            ),
                          ),
                        );
                      },
                      ),
                    ),
            ),
          ],
        ),
      ),
    );
  }
}
