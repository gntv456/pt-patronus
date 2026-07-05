import 'package:flutter/material.dart';
import 'package:ptpatronus/api/site.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';
import '../webview_login/webview_login_page.dart';

/// 站点页：宽屏数据表格（名称/定义/平台/地址/状态/操作）+ 窄屏卡片，对齐 Web Sites.vue 的 el-table。
/// cookie 走 WebView 登录（移动端更优）；表单也支持手填 cookie/账号/密码（对齐 Web 添加表单能力）。
class SitesPage extends StatefulWidget {
  const SitesPage({super.key});
  @override
  State<SitesPage> createState() => _SitesPageState();
}

class _SitesPageState extends State<SitesPage> {
  List<Site> _sites = [];
  List<SiteDef> _defs = []; // 添加表单下拉项缓存
  bool _loading = true; // 首访即显示加载态，避免短暂空白像页面坏了
  String? _error;
  int? _checkining; // 正在签到的站点 id
  int? _testing; // 正在连接测试的站点 id
  bool _checkinAllLoading = false;

  bool get _isAdmin => ApiClient().isAdmin;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final results = await Future.wait([
        SiteApi().list(),
        _defs.isEmpty ? SiteApi().definitions() : Future.value(_defs),
      ]);
      _sites = results[0] as List<Site>;
      _defs = results[1] as List<SiteDef>;
    } catch (e) {
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

  String _defaultUrl(SiteDef d) {
    if (d.urls.isNotEmpty) return d.urls.first;
    return d.schema == 'mteam' ? 'https://api.m-team.cc' : 'https://hxpt.org';
  }

  void _login(Site s) {
    Navigator.of(
      context,
    ).push(MaterialPageRoute(builder: (_) => WebViewLoginPage(site: s)));
  }

  // —— 连接测试 / 签到 / 一键签到（业务逻辑保持不变）——
  Future<void> _testConnection(Site s) async {
    setState(() => _testing = s.id);
    try {
      final r = await SiteApi().test(s.id);
      if (r['ok'] == true) {
        _toast('${s.name}：连接正常，等级 ${r['level'] ?? '-'}');
      } else {
        _toast('${s.name}：${r['error'] ?? '连接失败'}');
      }
    } catch (e) {
      _toast('请求失败：$e');
    } finally {
      if (mounted) setState(() => _testing = null);
    }
  }

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
      _toast(
        failed.isEmpty
            ? '一键签到完成：$summary'
            : '一键签到完成：$summary / 失败 ${failed.length}（${failed.join('、')}）',
      );
    } catch (e) {
      _toast('一键签到失败：$e');
    } finally {
      if (mounted) setState(() => _checkinAllLoading = false);
    }
  }

  Future<void> _deleteSite(Site s) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('删除站点'),
        content: Text('确认删除站点「${s.name}」？相关配置将被移除。'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: const Text('取消'),
          ),
          TextButton(
            onPressed: () => Navigator.pop(ctx, true),
            style: TextButton.styleFrom(foregroundColor: Colors.redAccent),
            child: const Text('删除'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await SiteApi().remove(s.id);
      _toast('已删除');
      await _load();
    } catch (e) {
      _toast('失败：$e');
    }
  }

  String _fmtRecordTime(String iso) {
    if (iso.isEmpty) return '';
    final d = DateTime.tryParse(iso);
    if (d == null) return iso;
    final local = d.toLocal();
    String p(int n) => n.toString().padLeft(2, '0');
    return '${local.year}-${p(local.month)}-${p(local.day)} ${p(local.hour)}:${p(local.minute)}';
  }

  // —— 签到记录：自适应浮层（宽屏对话框 / 窄屏底栏），对齐 Web el-drawer ——
  Future<void> _showRecords() async {
    List<CheckinRecord> recs;
    try {
      recs = await SiteApi().records();
    } catch (e) {
      _toast('加载签到记录失败：$e');
      return;
    }
    if (!mounted) return;
    await showAdaptiveSheet(
      context,
      title: '签到记录',
      wideWidth: 640,
      builder: (_) => recs.isEmpty
          ? const PTEmptyState(text: '暂无签到记录')
          : PTAdaptiveTable<CheckinRecord>(
              items: recs,
              columns: [
                PTColumn(
                  label: '时间',
                  flex: 2,
                  cell: (_, r) => Text(
                    _fmtRecordTime(r.createdAt),
                    style: const TextStyle(fontSize: 12),
                  ),
                ),
                PTColumn(
                  label: '站点',
                  flex: 1,
                  cell: (_, r) =>
                      Text(r.siteName, style: const TextStyle(fontSize: 12)),
                ),
                PTColumn(
                  label: '结果',
                  flex: 2,
                  cell: (_, r) => _recordResult(r),
                ),
                PTColumn(
                  label: '来源',
                  flex: 1,
                  cell: (_, r) => Text(
                    r.source == 'cron' ? '自动' : '手动',
                    style: const TextStyle(fontSize: 12),
                  ),
                ),
              ],
              emptyText: '暂无签到记录',
              narrowCard: (_, r) => GlassCard(
                padding: const EdgeInsets.all(12),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        Expanded(
                          child: Text(
                            '${r.siteName} · ${r.source == 'cron' ? '自动' : '手动'}',
                            style: const TextStyle(
                              fontWeight: FontWeight.w600,
                              fontSize: 13,
                            ),
                          ),
                        ),
                        Text(
                          _fmtRecordTime(r.createdAt),
                          style: const TextStyle(
                            fontSize: 11,
                            color: Colors.grey,
                          ),
                        ),
                      ],
                    ),
                    const SizedBox(height: 4),
                    Text(
                      r.success
                          ? (r.bonus > 0
                                ? '+${r.bonus} 魔力'
                                : (r.message.isNotEmpty ? r.message : '成功'))
                          : (r.error.isNotEmpty
                                ? r.error
                                : (r.message.isNotEmpty ? r.message : '失败')),
                      style: TextStyle(
                        fontSize: 12,
                        color: r.success ? PTColors.success : PTColors.danger,
                      ),
                    ),
                  ],
                ),
              ),
            ),
    );
  }

  Widget _recordResult(CheckinRecord r) {
    if (r.success) {
      return Container(
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
        decoration: BoxDecoration(
          color: PTColors.success.withValues(alpha: 0.16),
          borderRadius: BorderRadius.circular(999),
        ),
        child: Text(
          r.bonus > 0
              ? '+${r.bonus} 魔力'
              : (r.message.isNotEmpty ? r.message : '成功'),
          style: const TextStyle(fontSize: 12, color: PTColors.success),
        ),
      );
    }
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
      decoration: BoxDecoration(
        color: PTColors.danger.withValues(alpha: 0.14),
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(
        r.error.isNotEmpty
            ? r.error
            : (r.message.isNotEmpty ? r.message : '失败'),
        style: const TextStyle(fontSize: 12, color: PTColors.danger),
      ),
    );
  }

  // —— 统一添加/编辑表单：showAdaptiveSheet 内 StatefulBuilder ——
  Future<void> _showForm({Site? editing}) async {
    final isEdit = editing != null;
    List<SiteDef> defs = _defs;
    if (!isEdit && defs.isEmpty) {
      try {
        defs = await SiteApi().definitions();
        _defs = defs;
      } catch (e) {
        _toast('加载站点定义失败：$e');
        return;
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
    final knownUrls = <String>{'https://api.m-team.cc', 'https://hxpt.org'};
    for (final d in defs) {
      knownUrls.add(_defaultUrl(d));
    }

    if (!mounted) return;
    final submit = await showAdaptiveSheet<bool>(
      context,
      title: isEdit ? '编辑站点' : '添加站点',
      wideWidth: 520,
      builder: (dctx) => StatefulBuilder(
        builder: (fctx, setSt) {
          final isMteam = selected.schema == 'mteam';
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
                  TextField(
                    controller: urlCtrl,
                    decoration: InputDecoration(
                      labelText: '地址',
                      hintText: _defaultUrl(selected),
                    ),
                  ),
                  if (isMteam) ...[
                    const SizedBox(height: 12),
                    TextField(
                      controller: keyCtrl,
                      obscureText: true,
                      decoration: InputDecoration(
                        labelText: 'API Key',
                        hintText: isEdit
                            ? '留空不修改'
                            : 'M-Team x-api-key（控制台 → 实验室 → 存储令牌）',
                      ),
                    ),
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
    if (!isEdit && isMteam && keyCtrl.text.trim().isEmpty) {
      _toast('馒头需填写 API Key');
      return;
    }
    try {
      if (isEdit) {
        await SiteApi().update(
          editing.id,
          name: name,
          baseUrl: url,
          apiKey: isMteam && keyCtrl.text.trim().isNotEmpty
              ? keyCtrl.text.trim()
              : null,
          cookie: !isMteam && cookieCtrl.text.trim().isNotEmpty
              ? cookieCtrl.text.trim()
              : null,
          username: !isMteam && userCtrl.text.trim().isNotEmpty
              ? userCtrl.text.trim()
              : null,
          password: !isMteam && passCtrl.text.trim().isNotEmpty
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
          apiKey: isMteam ? keyCtrl.text.trim() : null,
          cookie: !isMteam && cookieCtrl.text.trim().isNotEmpty
              ? cookieCtrl.text.trim()
              : null,
          username: !isMteam && userCtrl.text.trim().isNotEmpty
              ? userCtrl.text.trim()
              : null,
          password: !isMteam && passCtrl.text.trim().isNotEmpty
              ? passCtrl.text.trim()
              : null,
          isActive: active,
        );
        _toast(isMteam ? '已添加馒头，API Key 已保存' : '已添加，请点「登录」获取 cookie');
      }
      await _load();
    } catch (e) {
      _toast('失败：$e');
    }
  }

  @override
  Widget build(BuildContext context) {
    return PTPageScaffold(
      title: '站点',
      actions: [
        PTBarAction(icon: Icons.refresh, label: '刷新', onPressed: _load),
        PTBarAction(
          icon: Icons.add,
          label: '添加站点',
          onPressed: () => _showForm(),
        ),
        PTBarAction(
          icon: Icons.history,
          label: '签到记录',
          onPressed: _showRecords,
          overflow: true,
        ),
        PTBarAction(
          icon: Icons.card_giftcard,
          label: '一键签到',
          onPressed: _checkinAllLoading ? null : _checkinAll,
          overflow: true,
        ),
      ],
      body: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          if (_loading && _sites.isEmpty)
            const LinearProgressIndicator(minHeight: 2),
          Expanded(child: _listBody()),
        ],
      ),
    );
  }

  Widget _listBody() {
    if (_sites.isEmpty && !_loading) {
      return RefreshIndicator(
        onRefresh: _load,
        child: _error != null
            ? PTEmptyState(
                icon: Icons.error_outline,
                text: '加载失败',
                description: _error,
                onRetry: _load,
              )
            : PTEmptyState(
                icon: Icons.language,
                text: '暂无站点',
                description: '添加你的第一个 PT 站点，开始跨站统计与签到',
                actionLabel: '添加站点',
                onAction: () => _showForm(),
              ),
      );
    }
    return RefreshIndicator(
      onRefresh: _load,
      child: PTAdaptiveTable<Site>(
        items: _sites,
        columns: [
          PTColumn(
            label: '名称',
            flex: 2,
            cell: (_, s) => Text(
              s.name,
              style: const TextStyle(fontWeight: FontWeight.w600),
            ),
          ),
          PTColumn(
            label: '定义',
            flex: 2,
            cell: (_, s) => Text(
              s.definition,
              style: const TextStyle(fontSize: 13, color: Colors.grey),
            ),
          ),
          PTColumn(
            label: '平台',
            flex: 1,
            cell: (_, s) => Text(
              s.platform.isNotEmpty ? s.platform : '-',
              style: const TextStyle(fontSize: 13),
            ),
          ),
          PTColumn(
            label: '地址',
            flex: 3,
            cell: (_, s) => Text(
              s.baseUrl,
              style: const TextStyle(fontSize: 12),
              overflow: TextOverflow.ellipsis,
            ),
          ),
          PTColumn(
            label: '状态',
            flex: 1,
            cell: (_, s) => _statusPill(s.isActive),
          ),
          PTColumn(label: '操作', flex: 3, cell: (ctx, s) => _rowActions(ctx, s)),
        ],
        emptyText: '暂无站点',
        narrowCard: (ctx, s) => GlassCard(
          padding: const EdgeInsets.fromLTRB(14, 12, 8, 12),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  Expanded(
                    child: Text(
                      s.name,
                      style: const TextStyle(
                        fontWeight: FontWeight.w700,
                        fontSize: 16,
                      ),
                    ),
                  ),
                  _statusPill(s.isActive),
                ],
              ),
              const SizedBox(height: 2),
              Text(
                s.baseUrl,
                style: const TextStyle(color: Colors.grey, fontSize: 12),
                overflow: TextOverflow.ellipsis,
              ),
              const SizedBox(height: 4),
              Text(
                '${s.definition} · ${s.platform.isNotEmpty ? s.platform : '-'}',
                style: const TextStyle(color: Colors.grey, fontSize: 12),
              ),
              const SizedBox(height: 8),
              _rowActions(ctx, s),
            ],
          ),
        ),
      ),
    );
  }

  Widget _statusPill(bool on) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 2),
      decoration: BoxDecoration(
        color: (on ? PTColors.success : PTColors.danger).withValues(
          alpha: on ? 0.2 : 0.15,
        ),
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(
        on ? '启用' : '停用',
        style: TextStyle(
          fontSize: 12,
          color: on ? PTColors.success : PTColors.danger,
          fontWeight: FontWeight.w600,
        ),
      ),
    );
  }

  Widget _rowActions(BuildContext context, Site s) {
    final isNexus = s.platform == 'nexusphp';
    final isMteam = s.platform == 'mteam';
    return Wrap(
      spacing: 4,
      runSpacing: 4,
      crossAxisAlignment: WrapCrossAlignment.center,
      children: [
        if (isNexus)
          _actBtn(
            icon: Icons.card_giftcard_outlined,
            label: '签到',
            loading: _checkining == s.id,
            onPressed: _checkining == s.id ? null : () => _checkin(s),
          ),
        if (isMteam)
          _actBtn(
            icon: Icons.bolt_outlined,
            label: '测试',
            loading: _testing == s.id,
            onPressed: _testing == s.id ? null : () => _testConnection(s),
          )
        else
          _actBtn(icon: Icons.login, label: '登录', onPressed: () => _login(s)),
        _actBtn(
          icon: Icons.edit_outlined,
          label: '编辑',
          onPressed: () => _showForm(editing: s),
        ),
        if (_isAdmin)
          _actBtn(
            icon: Icons.delete_outline,
            label: '删除',
            danger: true,
            onPressed: () => _deleteSite(s),
          ),
      ],
    );
  }

  Widget _actBtn({
    required IconData icon,
    required String label,
    bool danger = false,
    bool loading = false,
    VoidCallback? onPressed,
  }) {
    final color = danger ? PTColors.danger : PTColors.primary;
    if (loading) {
      return const Padding(
        padding: EdgeInsets.all(8),
        child: SizedBox(
          width: 16,
          height: 16,
          child: CircularProgressIndicator(strokeWidth: 2),
        ),
      );
    }
    return TextButton.icon(
      onPressed: onPressed,
      icon: Icon(icon, size: 16, color: color),
      label: Text(label, style: TextStyle(fontSize: 12, color: color)),
      style: TextButton.styleFrom(
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
        visualDensity: VisualDensity.compact,
      ),
    );
  }
}

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
    final q = _ctrl.text.trim().toLowerCase();
    final matches = q.isEmpty
        ? const <SiteDef>[]
        : widget.defs
              .where((d) =>
                  '${d.name} ${d.schema} ${d.urls.join(' ')} ${d.aka.join(' ')} ${d.description} ${d.tags.join(' ')}'
                      .toLowerCase()
                      .contains(q))
              .take(50)
              .toList();
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
              hintText: '输入名称搜索…',
              prefixIcon: Icon(Icons.search, size: 18),
              isDense: true,
              border: OutlineInputBorder(),
            ),
            onChanged: (_) => setState(() {}),
          ),
        ),
        if (q.isEmpty)
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 24),
            child: Center(
              child: Text(
                '共 ${widget.defs.length} 个定义，输入名称搜索',
                style: const TextStyle(color: Colors.grey, fontSize: 13),
              ),
            ),
          )
        else if (matches.isEmpty)
          const Padding(
            padding: EdgeInsets.symmetric(vertical: 24),
            child: Center(
              child: Text('无匹配', style: TextStyle(color: Colors.grey, fontSize: 13)),
            ),
          )
        else
          ListView(
            shrinkWrap: true,
            physics: const NeverScrollableScrollPhysics(),
            padding: EdgeInsets.zero,
            children: [
              for (final d in matches)
                ListTile(
                  title: Text('${d.name}（${d.schema}）'),
                  subtitle: (d.aka.isNotEmpty || d.dead)
                      ? Text(
                          [...d.aka, if (d.dead) '已关站'].join(' · '),
                          style: TextStyle(fontSize: 11, color: d.dead ? Colors.grey : Colors.black54),
                        )
                      : null,
                  trailing: d.id == widget.selected.id
                      ? const Icon(Icons.check_circle, color: PTColors.primary, size: 20)
                      : null,
                  onTap: () => Navigator.of(context).pop(d),
                ),
            ],
          ),
      ],
    );
  }
}
