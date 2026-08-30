// MedalWallPage 的 MedalWallOpsMixin mixin（含 setState 段；从 medal_wall_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/medal_wall/medal_wall_page.dart';

mixin MedalWallOpsMixin on State<MedalWallPage> {

  final _api = MedalWallApi();
  StreamSubscription<List<Medal>>? _sub; // 流式首屏订阅（切换/刷新时 cancel）
  List<Medal> _medals = [];
  bool _loading = true;
  String? _error;
  String _filter = 'all'; // all/owned/available/out_of_stock/wearing
  List<int> _selectedSiteIds = []; // 站点多选筛选（按 site_id），空=全部
  Future<void> _load({bool refresh = false}) {
    _sub?.cancel();
    setState(() {
      _medals = [];
      _loading = true;
      _error = null;
    });
    _sub = _api.allStream(refresh: refresh).listen(
      (cards) {
        if (!mounted) return;
        setState(() => _medals = [..._medals, ...cards]);
      },
      onDone: () {
        if (mounted) setState(() => _loading = false);
      },
      onError: (e) {
        if (!mounted) return;
        setState(() {
          _loading = false;
          _error = e.toString();
        });
      },
    );
    return Future.value();
  }
}
