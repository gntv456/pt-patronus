// MonitorDetailPage 的展示层扩展（无 setState 段；从 monitor_detail_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/monitor/monitor_detail_page.dart';

extension on _MonitorDetailPageState {
  String _tempName(String raw) {
    final name = raw.trim();
    final key = name.toLowerCase();
    if (key.isEmpty) return '温度传感器';
    if (key == 'acpitz') return 'ACPI 温区';
    if (key == 'nvme_composite') return 'NVMe 综合温度';
    final nvmeSensor = RegExp(r'^nvme_sensor_(\d+)$').firstMatch(key);
    if (nvmeSensor != null) return 'NVMe 传感器 ${nvmeSensor.group(1)}';
    final nvme = RegExp(r'^nvme(?:_temp)?_?(\d+)$').firstMatch(key);
    if (nvme != null) return 'NVMe 温度 ${nvme.group(1)}';
    final pkg = RegExp(r'^coretemp_package_id_(\d+)$').firstMatch(key);
    if (pkg != null) {
      final id = pkg.group(1);
      return id == '0' ? 'CPU 封装' : 'CPU 封装 $id';
    }
    final core = RegExp(r'^coretemp_core_(\d+)$').firstMatch(key);
    if (core != null) return 'CPU 核心 ${core.group(1)}';
    if (key == 'k10temp_tctl') return 'AMD CPU 控制温度';
    if (key == 'k10temp_tccd1') return 'AMD CCD1 温度';
    if (key.startsWith('k10temp_tccd')) {
      return 'AMD CCD${key.replaceFirst('k10temp_tccd', '')} 温度';
    }
    return name.replaceAll('_', ' ');
  }

  Widget _card(String label, String value, {Color? color}) => GlassCard(
    padding: const EdgeInsets.all(14),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(label, style: TextStyle(color: context.dimText, fontSize: 12)),
        const SizedBox(height: 4),
        Text(
          value,
          style: TextStyle(
            fontSize: 20,
            fontWeight: FontWeight.w700,
            color: color,
          ),
        ),
      ],
    ),
  );

  Widget _cameraPreview() {
    if (_playerErr != null) {
      return GlassCard(
        padding: const EdgeInsets.all(16),
        child: Column(
          children: [
            Icon(Icons.videocam_off, size: 36, color: context.dimText),
            const SizedBox(height: 8),
            Text(
              _playerErr!,
              textAlign: TextAlign.center,
              style: TextStyle(color: context.dimText),
            ),
            const SizedBox(height: 12),
            OutlinedButton.icon(
              icon: const Icon(Icons.open_in_browser),
              label: const Text('在 WebView 中打开 Web 版'),
              onPressed: () => Navigator.of(context).push(
                MaterialPageRoute(
                  builder: (_) => InAppWebViewPage(initialUrl: _webStreamUrl()),
                ),
              ),
            ),
          ],
        ),
      );
    }
    if (_controller == null) {
      return AspectRatio(
        aspectRatio: 16 / 9,
        child: ColoredBox(
          color: context.mainText,
          child: Center(
            child: CircularProgressIndicator(
              strokeWidth: 2,
              color: Colors.white70,
            ),
          ),
        ),
      );
    }
    return ClipRRect(
      borderRadius: BorderRadius.circular(16),
      child: AspectRatio(
        aspectRatio: 16 / 9,
        child: Video(controller: _controller!),
      ),
    );
  }

  String _webStreamUrl() {
    // 降级：用 WebView 打开后端内嵌 Web 的摄像头流（需已登录态；粗略指向 Web 端）。
    final u = Uri.parse(_api.streamURL(widget.target.id));
    return '${u.scheme}://${u.host}/';
  }

  Widget _trend(
    String label,
    List<double> data, {
    Color color = PTColors.primary,
  }) {
    if (data.length < 2) {
      return Padding(
        padding: const EdgeInsets.symmetric(vertical: 12),
        child: Text(
          '$label 趋势：数据不足',
          style: TextStyle(color: context.dimText, fontSize: 13),
        ),
      );
    }
    final maxY =
        data.fold<double>(1, (a, b) => a > b ? a : b).clamp(1, 100) * 1.2;
    return Padding(
      padding: const EdgeInsets.only(top: 8, bottom: 4),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            label,
            style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 14),
          ),
          const SizedBox(height: 6),
          SizedBox(
            height: 90,
            child: LineChart(
              LineChartData(
                gridData: FlGridData(show: true, drawVerticalLine: false),
                borderData: FlBorderData(show: false),
                titlesData: const FlTitlesData(show: false),
                minY: 0,
                maxY: maxY,
                lineBarsData: [
                  LineChartBarData(
                    isCurved: true,
                    spots: [
                      for (var i = 0; i < data.length; i++)
                        FlSpot(i.toDouble(), data[i]),
                    ],
                    color: color,
                    barWidth: 2,
                    dotData: const FlDotData(show: false),
                    belowBarData: BarAreaData(
                      show: true,
                      color: color.withValues(alpha: 0.12),
                    ),
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }


  Future<void> _edit() async {
    final ok = await DeviceFormSheet.show(context, target: widget.target);
    if (ok == true) _load();
  }

  Future<void> _delete() async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('删除设备'),
        content: Text('确认删除「${widget.target.name}」？'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          TextButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('删除'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await _api.remove(widget.target.id);
      if (mounted) Navigator.pop(context);
    } catch (e) {
      _toast(e.toString().replaceFirst('Exception: ', ''));
    }
  }

  Future<void> _power(String action) async {
    final isShutdown = action == 'shutdown';
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: Text(isShutdown ? '确认关机' : '确认重启'),
        content: Text(
          '确认${isShutdown ? '关闭' : '重启'}「${widget.target.name}」？'
          '${isShutdown ? '关机后可能需要手动开机。' : '设备会短暂离线。'}',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          FilledButton(
            style: isShutdown
                ? FilledButton.styleFrom(backgroundColor: Colors.redAccent)
                : null,
            onPressed: () => Navigator.pop(context, true),
            child: Text(isShutdown ? '关机' : '重启'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    try {
      final r = await _api.power(widget.target.id, action);
      if (r.ok) {
        _toast(isShutdown ? '关机命令已发送' : '重启命令已发送');
      } else {
        _toast(r.error ?? '电源操作失败');
      }
    } catch (e) {
      _toast(e.toString().replaceFirst('Exception: ', ''));
    }
  }
}
