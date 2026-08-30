// Peers 弹窗：实时列表 + flagship 图标。
// （从 downloaders_page.dart 拆出，part of 主库；行为不变。）
part of 'package:ptpatronus/features/downloaders/downloaders_page.dart';

// Peers 列表。
class _PeersSheet extends StatefulWidget {
  final int id;
  final String hash;
  const _PeersSheet({required this.id, required this.hash});
  @override
  State<_PeersSheet> createState() => _PeersSheetState();
}

class _PeersSheetState extends State<_PeersSheet> {
  final _api = DownloaderApi();
  List<Peer> _peers = [];
  bool _loading = true;
  String _error = '';

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    try {
      final p = await _api.peers(widget.id, widget.hash);
      if (mounted) {
        setState(() {
          _peers = p;
          _loading = false;
          _error = '';
        });
      }
    } catch (e) {
      if (mounted) setState(() { _loading = false; _error = e.toString(); });
    }
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        Padding(
          padding: const EdgeInsets.all(14),
          child: Row(
            children: [
              Text(
                'Peers',
                style: TextStyle(fontSize: 16, fontWeight: FontWeight.w600),
              ),
              const Spacer(),
              Text(
                '${_peers.length}',
                style: TextStyle(color: context.dimText, fontSize: 13),
              ),
              IconButton(
                tooltip: '刷新 Peers 列表',
                icon: const Icon(Icons.refresh, size: 20),
                onPressed: _load,
              ),
            ],
          ),
        ),
        const Divider(height: 1),
        Expanded(
          child: _loading
              ? const PTLoadingView()
              : _error.isNotEmpty
              ? _sheetErrorView(_error, _load)
              : _peers.isEmpty
              ? Center(
                  child: Text('无连接的对等体', style: TextStyle(color: context.dimText)),
                )
              : ListView.builder(
                  itemCount: _peers.length,
                  itemBuilder: (_, i) {
                    final p = _peers[i];
                    return ListTile(
                      dense: true,
                      title: Text(p.ip, style: const TextStyle(fontSize: 13)),
                      subtitle: Text(
                        '${p.client.isEmpty ? '-' : p.client} · ${(p.progress * 100).toStringAsFixed(0)}% · ↓${_fmtSpeed(p.dlRate)} ↑${_fmtSpeed(p.upRate)}',
                        style: const TextStyle(fontSize: 11),
                      ),
                      trailing: Text(
                        p.flags,
                        style: TextStyle(
                          fontSize: 11,
                          color: context.dimText,
                        ),
                      ),
                    );
                  },
                ),
        ),
      ],
    );
  }
}
