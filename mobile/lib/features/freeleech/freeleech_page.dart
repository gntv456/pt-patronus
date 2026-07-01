import 'package:flutter/material.dart';
import 'package:ptpatronus/features/home/home_menu_button.dart';
import 'package:ptpatronus/api/freeleech.dart';
import 'package:ptpatronus/theme/glass.dart';

/// 站免页：汇总各站全站免费/促销状态。下拉刷新；单站可独立刷新。
class FreeleechPage extends StatefulWidget {
  const FreeleechPage({super.key});
  @override
  State<FreeleechPage> createState() => _FreeleechPageState();
}

class _FreeleechPageState extends State<FreeleechPage> {
  final _api = FreeleechApi();
  List<FreeleechResult> _results = [];
  bool _loading = true;
  String? _error;
  int? _syncingId;

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
      _results = await _api.all(refresh: refresh);
    } catch (e) {
      _error = e.toString();
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _syncOne(FreeleechResult r) async {
    setState(() => _syncingId = r.siteId);
    try {
      final one = await _api.syncOne(r.siteId);
      final i = _results.indexWhere((x) => x.siteId == r.siteId);
      if (i >= 0) _results[i] = one;
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('$e')));
      }
    } finally {
      if (mounted) setState(() => _syncingId = null);
    }
  }

  Color _statusColor(FreeleechStatus s) => switch (s) {
        FreeleechStatus.active => const Color(0xFF2ECC71),
        FreeleechStatus.upcoming => const Color(0xFF3498DB),
        FreeleechStatus.failed => const Color(0xFFF56C6C),
        FreeleechStatus.none => const Color(0xFF95A5A6),
      };

  String _statusLabel(FreeleechStatus s) => switch (s) {
        FreeleechStatus.active => '正在站免',
        FreeleechStatus.upcoming => '即将站免',
        FreeleechStatus.failed => '读取失败',
        FreeleechStatus.none => '暂无站免',
      };

  @override
  Widget build(BuildContext context) {
    return GlassBackground(
      child: Scaffold(
        backgroundColor: Colors.transparent,
        appBar: AppBar(
          leading: const HomeMenuButton(),
          title: const Text('站免'),
          backgroundColor: Colors.transparent,
          elevation: 0,
          actions: [
            IconButton(
              icon: const Icon(Icons.refresh),
              onPressed: _loading ? null : () => _load(refresh: true),
            ),
          ],
        ),
        body: RefreshIndicator(
          onRefresh: () => _load(refresh: true),
          child: _loading
              ? const Center(child: CircularProgressIndicator())
              : _error != null
                  ? _Empty(text: _error!, onRetry: _load)
                  : _results.isEmpty
                      ? const _Empty(text: '暂无数据，请先添加并登录站点')
                      : ListView.builder(
                          padding: const EdgeInsets.all(12),
                          itemCount: _results.length,
                          itemBuilder: (_, i) => _siteCard(_results[i]),
                        ),
        ),
      ),
    );
  }

  Widget _siteCard(FreeleechResult r) {
    final top = r.notices.isNotEmpty ? r.notices.first : null;
    final color = top != null ? _statusColor(top.status) : const Color(0xFF95A5A6);
    return GlassCard(
      padding: const EdgeInsets.all(14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(children: [
            Text(r.siteName, style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 15)),
            const SizedBox(width: 8),
            if (top != null)
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
                decoration: BoxDecoration(color: color.withValues(alpha: 0.18), borderRadius: BorderRadius.circular(999)),
                child: Text(_statusLabel(top.status), style: TextStyle(color: color, fontSize: 12)),
              ),
            const Spacer(),
            if (_syncingId == r.siteId)
              const Padding(padding: EdgeInsets.all(8), child: SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2)))
            else
              IconButton(
                icon: const Icon(Icons.refresh, size: 18),
                onPressed: () => _syncOne(r),
                visualDensity: VisualDensity.compact,
              ),
          ]),
          if (r.error.isNotEmpty)
            Padding(padding: const EdgeInsets.only(top: 6), child: Text(r.error, style: const TextStyle(color: Color(0xFFF56C6C), fontSize: 13)))
          else if (r.notices.isEmpty)
            const Padding(padding: EdgeInsets.only(top: 6), child: Text('该站点未开启站免检测', style: TextStyle(color: Colors.grey, fontSize: 13)))
          else
            ...r.notices.map((n) => Padding(
                  padding: const EdgeInsets.only(top: 8),
                  child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                    Text(n.title.isNotEmpty ? n.title : _statusLabel(n.status), style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 14)),
                    if (n.content.isNotEmpty || n.error.isNotEmpty)
                      Padding(padding: const EdgeInsets.only(top: 2), child: Text(n.content.isNotEmpty ? n.content : n.error, style: const TextStyle(fontSize: 13))),
                  ]),
                )),
        ],
      ),
    );
  }
}

class _Empty extends StatelessWidget {
  final String text;
  final VoidCallback? onRetry;
  const _Empty({required this.text, this.onRetry});
  @override
  Widget build(BuildContext context) {
    return ListView(
      children: [
        const SizedBox(height: 120),
        Center(child: Padding(padding: const EdgeInsets.all(24), child: Text(text, textAlign: TextAlign.center, style: const TextStyle(color: Colors.grey)))),
        if (onRetry != null) Center(child: ElevatedButton(onPressed: onRetry, child: const Text('重试'))),
      ],
    );
  }
}
