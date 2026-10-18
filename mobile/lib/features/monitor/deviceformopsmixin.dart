// DeviceFormSheet 的 DeviceFormOpsMixin mixin（含 setState 段；从 device_form_sheet.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/monitor/device_form_sheet.dart';

mixin DeviceFormOpsMixin on State<DeviceFormSheet> {
  // 宿主提供的钩子。
  void _normalizeNasFields();
  Future<void> _showAgentSecret(String secret);


  final _api = MonitorApi();
  final _agentApi = AgentApi();
  final _name = TextEditingController();
  final _host = TextEditingController();
  final _port = TextEditingController();
  final _username = TextEditingController();
  final _password = TextEditingController();
  final _privateKey = TextEditingController();
  final _rtsp = TextEditingController();
  final _interval = TextEditingController();
  final _timeout = TextEditingController();
  final _diskPaths = TextEditingController();
  late String _type;
  String _protocol = '';
  String _vendor = '';
  bool _enabled = true;
  bool _hasPrivateKey = false;
  bool _saving = false;
  bool _dirty = false; // 表单是否有未保存改动：返回键 / 下滑关闭时拦截确认（避免丢 SSH 私钥/地址等）
  Future<void> _submit() async {
    if (_name.text.trim().isEmpty) {
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(const SnackBar(content: Text('名称必填')));
      return;
    }
    setState(() => _saving = true);
    try {
      _normalizeNasFields();
      final input = TargetInput(
        name: _name.text.trim(),
        type: _type,
        host: _protocol == 'local' || _protocol == 'agent'
            ? ''
            : _host.text.trim(),
        port: _protocol == 'local' || _protocol == 'agent'
            ? 0
            : int.tryParse(_port.text.trim()) ?? 0,
        vendor: _type == 'nas' ? _vendor : '',
        protocol: _protocol,
        // PUT 是全量替换：这两项表单没有编辑入口，必须回读原值，
        // 否则「在手机上改个名字」就会抹掉告警规则/脚本命令并解绑凭据（后端照单存）。
        credentialId: widget.target?.credentialId ?? 0,
        thresholds: widget.target?.thresholds ?? '',
        rtspUrl: _type == 'camera' ? _rtsp.text.trim() : '',
        intervalSec: int.tryParse(_interval.text.trim()) ?? 60,
        timeoutSec: int.tryParse(_timeout.text.trim()) ?? 15,
        username: _username.text.trim(),
        password: _password.text.trim(),
        privateKey: (_type == 'host' && _protocol == 'ssh')
            ? _privateKey.text.trim()
            : '',
        enabled: _enabled,
        diskPaths: _diskPaths.text.trim(),
      );
      if (widget.target == null) {
        if (_protocol == 'agent') {
          final result = await _agentApi.create(input.name, type: _type);
          if (!mounted) return;
          await _showAgentSecret(result.plainSecret);
        } else {
          await _api.create(input);
        }
      } else {
        await _api.update(widget.target!.id, input);
      }
      if (mounted) Navigator.pop(context, true);
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(e.toString().replaceFirst('Exception: ', ''))),
        );
      }
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }
}
