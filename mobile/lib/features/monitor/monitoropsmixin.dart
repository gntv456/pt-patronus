// MonitorPage 的 MonitorOpsMixin mixin（含 setState 段；从 monitor_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/monitor/monitor_page.dart';

mixin MonitorOpsMixin on State<MonitorPage> {
  final _api = MonitorApi();
  List<MonitorTarget> _targets = [];
  // 每设备最新指标快照（卡片内联 CPU/内存/磁盘 用）。无批量端点，按 target 取 metrics?limit=1（同 Web loadAll）。
  final Map<int, Metrics?> _latest = {};
  List<MonitorAlert> _unread = [];
  bool _loading = false;
  String? _error;

  Timer? _timer;



}
