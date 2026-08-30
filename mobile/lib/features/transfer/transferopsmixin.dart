// TransferPage 的 TransferOpsMixin mixin（含 setState 段；从 transfer_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/transfer/transfer_page.dart';

mixin TransferOpsMixin on State<TransferPage> {

  final _api = TransferApi();
  final _dlApi = DownloaderApi();
  List<TransferHistory> _list = [];
  bool _loading = true;
  final String _status = ''; // '' | success | failed
  final _redoing = <int>{};

  // 宿主提供的钩子。
  void _toast(String msg);
  Future<void> _load() async {
    setState(() => _loading = true);
    try {
      _list = await _api.history(_status, 200);
    } catch (_) {
      _toast('加载失败');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _redo(TransferHistory h) async {
    setState(() => _redoing.add(h.id));
    try {
      final r = await _api.redo(h.id);
      final ok = r?['Success'] == true;
      _toast(ok ? '重做完成' : '重做未成功：${r?['ErrMsg'] ?? '未知'}');
      _load();
    } catch (_) {
      _toast('重做失败');
    } finally {
      if (mounted) setState(() => _redoing.remove(h.id));
    }
  }
}
