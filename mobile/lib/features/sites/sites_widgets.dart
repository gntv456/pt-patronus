// SitesPage 的展示层扩展（无 setState 段；从 sites_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/sites/sites_page.dart';

extension on _SitesPageState {
  String _fmtRecordTime(String iso) {
    if (iso.isEmpty) return '';
    final d = DateTime.tryParse(iso);
    if (d == null) return iso;
    final local = d.toLocal();
    String p(int n) => n.toString().padLeft(2, '0');
    return '${local.year}-${p(local.month)}-${p(local.day)} ${p(local.hour)}:${p(local.minute)}';
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

  Future<void> _batchDelete() async {
    if (_sites.isEmpty) {
      _toast('暂无站点');
      return;
    }
    final picked = <Site>[];
    final ok = await showAdaptiveSheet<bool>(
      context,
      title: '批量删除站点',
      wideWidth: 460,
      builder: (_) => StatefulBuilder(builder: (ctx, setS) {
        return Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            ..._sites.map((s) => CheckboxListTile(
                  dense: true,
                  contentPadding: const EdgeInsets.symmetric(horizontal: 4),
                  value: picked.contains(s),
                  title: Text(s.name, style: const TextStyle(fontSize: 14)),
                  onChanged: (v) => setS(() {
                    if (v == true) {
                      picked.add(s);
                    } else {
                      picked.remove(s);
                    }
                  }),
                )),
            const SizedBox(height: 8),
            FilledButton(
              onPressed: picked.isEmpty ? null : () => Navigator.pop(ctx, true),
              style: FilledButton.styleFrom(backgroundColor: Colors.redAccent),
              child: Text('删除选中（${picked.length}）'),
            ),
          ],
        );
      }),
    );
    if (ok != true || picked.isEmpty) return;
    if (!mounted) return;
    final confirm = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('批量删除'),
        content: Text('确认删除选中的 ${picked.length} 个站点？相关配置将被移除。'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('取消')),
          TextButton(
            onPressed: () => Navigator.pop(ctx, true),
            style: TextButton.styleFrom(foregroundColor: Colors.redAccent),
            child: const Text('删除'),
          ),
        ],
      ),
    );
    if (confirm != true) return;
    try {
      final res = await SiteApi().batchDelete(picked.map((s) => s.id).toList());
      final deleted = res['deleted'] as int? ?? 0;
      final failed = (res['failed'] as List?)?.length ?? 0;
      _toast(failed > 0 ? '已删除 $deleted 个，失败 $failed 个' : '已删除 $deleted 个站点');
      await _load();
    } catch (e) {
      _toast('失败：$e');
    }
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
    final isApiKeySite = s.platform == 'mteam' || s.platform == 'yemapt';
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
        if (isApiKeySite)
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

  void _login(Site s) {
    Navigator.of(
      context,
    ).push(MaterialPageRoute(builder: (_) => WebViewLoginPage(site: s)));
  }

}
