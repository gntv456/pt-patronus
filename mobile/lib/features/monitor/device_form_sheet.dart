import 'package:flutter/material.dart';
import 'package:ptpatronus/api/monitor.dart';

/// 设备新增/编辑表单（底部 sheet）。type 驱动 protocol/vendor 可选项；凭据编辑留空=不改。
class DeviceFormSheet extends StatefulWidget {
  final MonitorTarget? target; // null=新建
  const DeviceFormSheet({super.key, this.target});

  static Future<bool?> show(BuildContext context, {MonitorTarget? target}) {
    return showModalBottomSheet<bool>(
      context: context,
      isScrollControlled: true,
      builder: (bc) => Padding(
        padding: EdgeInsets.only(bottom: MediaQuery.of(bc).viewInsets.bottom),
        child: DeviceFormSheet(target: target),
      ),
    );
  }

  @override
  State<DeviceFormSheet> createState() => _DeviceFormSheetState();
}

class _DeviceFormSheetState extends State<DeviceFormSheet> {
  final _api = MonitorApi();
  final _name = TextEditingController();
  final _host = TextEditingController();
  final _port = TextEditingController();
  final _username = TextEditingController();
  final _password = TextEditingController();
  final _privateKey = TextEditingController();
  final _rtsp = TextEditingController();
  final _interval = TextEditingController();
  final _timeout = TextEditingController();

  late String _type;
  String _protocol = '';
  String _vendor = '';
  bool _enabled = true;
  bool _hasPrivateKey = false;
  bool _saving = false;
  bool _dirty = false; // 表单是否有未保存改动：返回键 / 下滑关闭时拦截确认（避免丢 SSH 私钥/地址等）
  void _markDirty() => _dirty = true;

  @override
  void initState() {
    super.initState();
    final t = widget.target;
    _type = t?.type ?? 'host';
    _protocol = t?.protocol ?? _defaultProtocol(_type);
    _vendor = t?.vendor ?? (_type == 'nas' ? 'synology' : '');
    _enabled = t?.enabled ?? true;
    _name.text = t?.name ?? '';
    _host.text = t?.host ?? '';
    _port.text = (t?.port ?? 0) == 0 ? '' : '${t!.port}';
    _username.text = t?.host == null ? '' : ''; // 凭据不回传
    _hasPrivateKey = t?.hasPrivateKey ?? false;
    _rtsp.text = t?.rtspUrl ?? '';
    _interval.text = '${t?.intervalSec ?? 60}';
    _timeout.text = '${t?.timeoutSec ?? 15}';
    for (final c in [_name, _host, _port, _username, _password, _privateKey, _rtsp, _interval, _timeout]) {
      c.addListener(_markDirty);
    }
  }

  String _defaultProtocol(String type) => switch (type) {
        'host' => 'local',
        'nas' => 'api',
        'camera' => 'onvif',
        _ => '',
      };

  List<String> _protocols(String type) => switch (type) {
        'host' => ['local', 'ssh'],
        'nas' => ['api', 'snmp'],
        'camera' => ['onvif', 'rtsp'],
        _ => [''],
      };

  @override
  void dispose() {
    for (final c in [_name, _host, _port, _username, _password, _privateKey, _rtsp, _interval, _timeout]) {
      c.dispose();
    }
    super.dispose();
  }

  Future<void> _submit() async {
    if (_name.text.trim().isEmpty) {
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('名称必填')));
      return;
    }
    setState(() => _saving = true);
    try {
      final input = TargetInput(
        name: _name.text.trim(),
        type: _type,
        host: _host.text.trim(),
        port: int.tryParse(_port.text.trim()) ?? 0,
        vendor: _type == 'nas' ? _vendor : '',
        protocol: _protocol,
        rtspUrl: _type == 'camera' ? _rtsp.text.trim() : '',
        intervalSec: int.tryParse(_interval.text.trim()) ?? 60,
        timeoutSec: int.tryParse(_timeout.text.trim()) ?? 15,
        username: _username.text.trim(),
        password: _password.text.trim(),
        privateKey: (_type == 'host' && _protocol == 'ssh') ? _privateKey.text.trim() : '',
        enabled: _enabled,
      );
      if (widget.target == null) {
        await _api.create(input);
      } else {
        await _api.update(widget.target!.id, input);
      }
      if (mounted) Navigator.pop(context, true);
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text(e.toString().replaceFirst('Exception: ', ''))));
      }
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  Widget _field(String label, TextEditingController ctrl, {String hint = '', bool number = false}) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 4),
      child: Row(
        children: [
          SizedBox(width: 72, child: Text(label, style: const TextStyle(fontSize: 14))),
          Expanded(
            child: TextField(
              controller: ctrl,
              keyboardType: number ? TextInputType.number : TextInputType.text,
              decoration: InputDecoration(hintText: hint, isDense: true),
            ),
          ),
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final isEdit = widget.target != null;
    final showHost = !(_type == 'host' && _protocol == 'local');
    return PopScope(
      canPop: !_dirty,
      onPopInvokedWithResult: _confirmExit,
      child: Padding(
      padding: const EdgeInsets.fromLTRB(20, 16, 20, 24),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(isEdit ? '编辑设备' : '添加设备',
              style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w700)),
          const SizedBox(height: 12),
          _field('名称', _name, hint: '如：客厅摄像头'),
          Row(
            children: [
              const SizedBox(width: 72, child: Text('类型', style: TextStyle(fontSize: 14))),
              Expanded(
                child: DropdownButton<String>(
                  value: _type,
                  isExpanded: true,
                  onChanged: isEdit
                      ? null
                      : (v) {
                          if (v == null) return;
                          setState(() {
                            _type = v;
                            _protocol = _defaultProtocol(v);
                            if (v == 'nas') _vendor = 'synology';
                          });
                        },
                  items: const [
                    DropdownMenuItem(value: 'host', child: Text('服务器')),
                    DropdownMenuItem(value: 'nas', child: Text('NAS')),
                    DropdownMenuItem(value: 'camera', child: Text('摄像头')),
                  ],
                ),
              ),
            ],
          ),
          if (showHost) _field('地址', _host, hint: '192.168.1.10'),
          if (showHost) _field('端口', _port, hint: '22 / 5000 …', number: true),
          Row(
            children: [
              const SizedBox(width: 72, child: Text('协议', style: TextStyle(fontSize: 14))),
              Expanded(
                child: DropdownButton<String>(
                  value: _protocol,
                  isExpanded: true,
                  items: _protocols(_type)
                      .map((p) => DropdownMenuItem(value: p, child: Text(p)))
                      .toList(),
                  onChanged: (v) => setState(() => _protocol = v ?? _protocol),
                ),
              ),
            ],
          ),
          if (_type == 'nas')
            Row(
              children: [
                const SizedBox(width: 72, child: Text('厂商', style: TextStyle(fontSize: 14))),
                Expanded(
                  child: DropdownButton<String>(
                    value: _vendor,
                    isExpanded: true,
                    items: const [
                      DropdownMenuItem(value: 'synology', child: Text('群晖')),
                      DropdownMenuItem(value: 'truenas', child: Text('TrueNAS')),
                      DropdownMenuItem(value: 'snmp', child: Text('通用 SNMP')),
                    ],
                    onChanged: (v) => setState(() => _vendor = v ?? _vendor),
                  ),
                ),
              ],
            ),
          if (_type == 'camera' && _protocol == 'rtsp')
            _field('RTSP', _rtsp, hint: 'rtsp://user:pass@ip/stream'),
          if (showHost || _type == 'nas') _field('账号', _username),
          if (showHost || _type == 'nas')
            _field('密码', _password, hint: isEdit ? '留空不改' : ''),
          if (_type == 'host' && _protocol == 'ssh')
            Padding(
              padding: const EdgeInsets.symmetric(vertical: 4),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text('私钥', style: TextStyle(fontSize: 14)),
                  const SizedBox(height: 4),
                  TextField(
                    controller: _privateKey,
                    maxLines: 6,
                    minLines: 4,
                    decoration: InputDecoration(
                      isDense: true,
                      hintText: _hasPrivateKey
                          ? '已配置私钥，留空保留；粘贴新私钥则覆盖'
                          : '粘贴 SSH 私钥 PEM',
                      border: const OutlineInputBorder(),
                    ),
                  ),
                ],
              ),
            ),
          _field('间隔(秒)', _interval, number: true),
          _field('超时(秒)', _timeout, number: true),
          SwitchListTile(
            contentPadding: EdgeInsets.zero,
            title: const Text('启用'),
            value: _enabled,
            onChanged: (v) => setState(() => _enabled = v),
          ),
          const SizedBox(height: 8),
          FilledButton(
            onPressed: _saving ? null : _submit,
            child: Text(_saving ? '保存中…' : (isEdit ? '保存' : '添加')),
          ),
        ],
      ),
      ),
    );
  }

  Future<void> _confirmExit(bool didPop, Object? result) async {
    if (didPop) return;
    final navigator = Navigator.of(context);
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('放弃修改？'),
        content: const Text('当前表单有未保存的改动，确定离开吗？'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('继续编辑')),
          TextButton(onPressed: () => Navigator.pop(ctx, true), child: const Text('放弃')),
        ],
      ),
    );
    if (ok != true) return;
    navigator.pop();
  }
}
