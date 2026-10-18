// MedalWallPage 的 MedalWallOpsMixin mixin（含 setState 段；从 medal_wall_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/medal_wall/medal_wall_page.dart';

mixin MedalWallOpsMixin on State<MedalWallPage> {

  late final _api = MedalWallApi(frames: widget.frames);
  StreamSubscription<MedalStreamEvent>? _sub; // 流式首屏订阅（切换/刷新时 cancel）
  List<Medal> _medals = [];
  bool _loading = true;
  String? _error;
  String? _streamBanner; // done 帧账面：已有数据时的顶部常驻说明
  String _filter = 'all'; // all/owned/available/out_of_stock/wearing
  List<int> _selectedSiteIds = []; // 站点多选筛选（按 site_id），空=全部
  Future<void> _load({bool refresh = false}) {
    _sub?.cancel();
    setState(() {
      _medals = [];
      _loading = true;
      _error = null;
      _streamBanner = null;
    });
    _sub = _api.allStream(refresh: refresh).listen(
      (ev) {
        if (!mounted) return;
        setState(() {
          _medals = [..._medals, ...ev.cards];
          // 一张卡都没到却说清单读失败：别把「读不到」画成「没有勋章」。
          final notice = streamNoticeOf(
            arrived: _medals.length,
            listError: ev.listError,
          );
          _error = notice.error ?? _error;
          _streamBanner = notice.banner ?? _streamBanner;
        });
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
