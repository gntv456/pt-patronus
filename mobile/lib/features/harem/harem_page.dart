import 'package:flutter/material.dart';
import 'package:ptpatronus/api/harem.dart';
import 'package:ptpatronus/theme/glass.dart';

class HaremPage extends StatefulWidget {
  const HaremPage({super.key});
  @override
  State<HaremPage> createState() => _HaremPageState();
}

class _HaremPageState extends State<HaremPage> {
  final _api = HaremApi();
  HaremResult? _data;
  bool _loading = true;
  String? _error;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load({bool refresh = false}) async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      _data = await _api.all(refresh: refresh);
    } catch (e) {
      _error = e.toString();
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _export() async {
    try {
      final text = await _api.exportText();
      if (!mounted) return;
      showModalBottomSheet(
        context: context,
        builder: (_) => Padding(
          padding: const EdgeInsets.all(16),
          child: SingleChildScrollView(child: Text(text, style: const TextStyle(fontFamily: 'monospace', fontSize: 13))),
        ),
      );
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('$e')));
    }
  }

  @override
  Widget build(BuildContext context) {
    final ov = _data?.overview;
    return GlassBackground(
      child: Scaffold(
        backgroundColor: Colors.transparent,
        appBar: AppBar(
          title: const Text('后宫'),
          backgroundColor: Colors.transparent,
          elevation: 0,
          actions: [
            IconButton(onPressed: _export, icon: const Icon(Icons.receipt_long)),
            IconButton(icon: const Icon(Icons.refresh), onPressed: _loading ? null : () => _load(refresh: true)),
          ],
        ),
        body: RefreshIndicator(
          onRefresh: () => _load(refresh: true),
          child: _loading
              ? const Center(child: CircularProgressIndicator())
              : _error != null
                  ? _Empty(text: _error!, onRetry: _load)
                  : _data == null
                      ? const _Empty(text: '无数据')
                      : ListView(
                          padding: const EdgeInsets.all(12),
                          children: [
                            if (ov != null) _overview(ov),
                            const SizedBox(height: 8),
                            ..._data!.prescriptions.map((p) => _prescription(p)),
                          ],
                        ),
        ),
      ),
    );
  }

  Widget _overview(HaremOverview ov) {
    final items = [
      ('${ov.totalSites}', '站点'),
      ('${ov.totalInvitees}', '成员'),
      ('${ov.totalPermInvites}', '永邀'),
      ('${ov.totalTempInvites}', '临邀'),
      ('${ov.totalLowRatio}', '低分享率'),
      ('${ov.totalBanned}', '已禁用'),
    ];
    return SizedBox(
      height: 60,
      child: ListView(
        scrollDirection: Axis.horizontal,
        children: items
            .map((e) => Container(
                  width: 76,
                  margin: const EdgeInsets.only(right: 6),
                  decoration: BoxDecoration(color: const Color(0xFF6C5CE7).withValues(alpha: 0.08), borderRadius: BorderRadius.circular(10)),
                  alignment: Alignment.center,
                  child: Column(mainAxisSize: MainAxisSize.min, children: [
                    Text(e.$1, style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w700)),
                    Text(e.$2, style: const TextStyle(fontSize: 11, color: Colors.grey)),
                  ]),
                ))
            .toList(),
      ),
    );
  }

  Widget _prescription(PrescriptionItem p) {
    return GlassCard(
      padding: const EdgeInsets.all(14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(children: [
            Expanded(child: Text(p.siteName, style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 15))),
            Text('现药 ${p.remain}', style: const TextStyle(fontSize: 13)),
            const SizedBox(width: 10),
            Text('可买 ${p.canBuy}', style: const TextStyle(fontSize: 13)),
            const SizedBox(width: 10),
            Text(p.canInvite ? '权限 ✓' : '权限 ✗', style: TextStyle(color: p.canInvite ? const Color(0xFF2ECC71) : const Color(0xFFF56C6C), fontSize: 13)),
          ]),
          const SizedBox(height: 6),
          if (p.error.isNotEmpty)
            Text(p.error, style: const TextStyle(color: Color(0xFFF56C6C), fontSize: 13))
          else
            Text('后宫 ${p.haremCount} 人${p.deadCount > 0 ? "（禁用 ${p.deadCount}）" : ""}', style: const TextStyle(color: Colors.grey, fontSize: 13)),
          if (p.invitees.isNotEmpty)
            TextButton(
              onPressed: () => _showInvitees(p),
              child: const Text('查看花名册'),
            ),
        ],
      ),
    );
  }

  void _showInvitees(PrescriptionItem p) {
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      builder: (_) => SizedBox(
        height: MediaQuery.of(context).size.height * 0.7,
        child: Column(
          children: [
            AppBar(title: Text('${p.siteName} 后宫成员'), automaticallyImplyLeading: false, actions: [IconButton(onPressed: () => Navigator.pop(context), icon: const Icon(Icons.close))]),
            Expanded(child: ListView(children: p.invitees.map(_inviteeRow).toList())),
          ],
        ),
      ),
    );
  }

  Widget _inviteeRow(InviteeInfo iv) {
    final dead = iv.status == 'disabled';
    return ListTile(
      title: Text(iv.username, style: TextStyle(color: dead ? const Color(0xFFF56C6C) : null)),
      subtitle: Text('状态 ${iv.status} · 分享率 ${iv.ratio}'),
      trailing: Text(iv.uploaded > 0 ? '${(iv.uploaded / 1073741824).toStringAsFixed(1)}G' : '—', style: const TextStyle(color: Colors.grey, fontSize: 12)),
    );
  }
}

class _Empty extends StatelessWidget {
  final String text;
  final VoidCallback? onRetry;
  const _Empty({required this.text, this.onRetry});
  @override
  Widget build(BuildContext context) {
    return ListView(children: [
      const SizedBox(height: 120),
      Center(child: Padding(padding: const EdgeInsets.all(24), child: Text(text, textAlign: TextAlign.center, style: const TextStyle(color: Colors.grey)))),
      if (onRetry != null) Center(child: ElevatedButton(onPressed: onRetry, child: const Text('重试'))),
    ]);
  }
}
