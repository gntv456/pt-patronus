import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:ptpatronus/api/monitor.dart';

part 'deviceformopsmixin.dart';
part 'device_form_widgets.dart';

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

class _DeviceFormSheetState extends State<DeviceFormSheet> with DeviceFormOpsMixin {

  @override
  void _normalizeNasFields() {
    if (_type != 'nas') return;
    if (_protocol == 'local' || _protocol == 'agent') {
      _vendor = '';
    } else if (_protocol == 'snmp') {
      _vendor = 'snmp';
    } else if (_vendor.isEmpty || _vendor == 'snmp') {
      _vendor = 'synology';
    }
  }
  @override
  void dispose() {
    for (final c in [
      _name,
      _host,
      _port,
      _username,
      _password,
      _privateKey,
      _rtsp,
      _interval,
      _timeout,
      _diskPaths,
    ]) {
      c.dispose();
    }
    super.dispose();
  }

  @override
  Future<void> _showAgentSecret(String secret) async {
    await showDialog<void>(
      context: context,
      barrierDismissible: false,
      builder: (ctx) => AlertDialog(
        title: const Text('Agent Secret（仅显示一次）'),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Text('请立即复制保存，用于 Agent 上报鉴权。关闭后无法再次查看。'),
            const SizedBox(height: 12),
            SelectableText(secret),
          ],
        ),
        actions: [
          TextButton.icon(
            onPressed: () async {
              await Clipboard.setData(ClipboardData(text: secret));
              if (!ctx.mounted) return;
              ScaffoldMessenger.of(
                ctx,
              ).showSnackBar(const SnackBar(content: Text('Secret 已复制')));
            },
            icon: const Icon(Icons.copy, size: 18),
            label: const Text('复制'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(ctx),
            child: const Text('关闭'),
          ),
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final isEdit = widget.target != null;
    final showHost = _protocol != 'local' && _protocol != 'agent';
    final lockAgentProtocol = isEdit && widget.target!.protocol == 'agent';
    return PopScope(
      canPop: !_dirty,
      onPopInvokedWithResult: _confirmExit,
      child: Padding(
        padding: const EdgeInsets.fromLTRB(20, 16, 20, 24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text(
              isEdit ? '编辑设备' : '添加设备',
              style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w700),
            ),
            const SizedBox(height: 12),
            _field('名称', _name, hint: '如：客厅摄像头'),
            Row(
              children: [
                const SizedBox(
                  width: 72,
                  child: Text('类型', style: TextStyle(fontSize: 14)),
                ),
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
                              if (v == 'nas') _normalizeNasFields();
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
            if (showHost)
              _field('端口', _port, hint: '22 / 5000 …', number: true),
            Row(
              children: [
                const SizedBox(
                  width: 72,
                  child: Text('采集方式', style: TextStyle(fontSize: 14)),
                ),
                Expanded(
                  child: DropdownButton<String>(
                    value: _protocol,
                    isExpanded: true,
                    items: _protocols(_type)
                        .map(
                          (p) => DropdownMenuItem(
                            value: p,
                            child: Text(_protocolLabel(p)),
                          ),
                        )
                        .toList(),
                    onChanged: lockAgentProtocol
                        ? null
                        : (v) => setState(() {
                            _protocol = v ?? _protocol;
                            _normalizeNasFields();
                          }),
                  ),
                ),
              ],
            ),
            if ((_type == 'host' || _type == 'nas') && _protocol == 'local')
              Row(
                children: [
                  const SizedBox(
                    width: 72,
                    child: Text('磁盘路径', style: TextStyle(fontSize: 14)),
                  ),
                  Expanded(
                    child: TextField(
                      controller: _diskPaths,
                      maxLines: 2,
                      decoration: const InputDecoration(
                        hintText: '每行一个容器内路径，如 /mnt/nas',
                        isDense: true,
                      ),
                    ),
                  ),
                ],
              ),
            if (_type == 'nas' && _protocol == 'api')
              Row(
                children: [
                  const SizedBox(
                    width: 72,
                    child: Text('厂商', style: TextStyle(fontSize: 14)),
                  ),
                  Expanded(
                    child: DropdownButton<String>(
                      value: _vendor,
                      isExpanded: true,
                      items: const [
                        DropdownMenuItem(value: 'synology', child: Text('群晖')),
                        DropdownMenuItem(
                          value: 'truenas',
                          child: Text('TrueNAS'),
                        ),
                      ],
                      onChanged: (v) => setState(() => _vendor = v ?? _vendor),
                    ),
                  ),
                ],
              ),
            if (_type == 'camera' && _protocol == 'rtsp')
              _field('RTSP', _rtsp, hint: 'rtsp://user:pass@ip/stream'),
            if (showHost) _field('账号', _username),
            if (showHost) _field('密码', _password, hint: isEdit ? '留空不改' : ''),
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
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: const Text('继续编辑'),
          ),
          TextButton(
            onPressed: () => Navigator.pop(ctx, true),
            child: const Text('放弃'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    navigator.pop();
  }

  void _markDirty() => _dirty = true;

  @override
  void initState() {
    super.initState();
    final t = widget.target;
    _type = t?.type ?? 'host';
    _protocol = t == null ? _defaultProtocol(_type) : _inferredProtocol(t);
    _vendor = t?.vendor ?? (_type == 'nas' ? 'synology' : '');
    if (_type == 'nas' && _vendor == 'snmp' && _protocol != 'local') {
      _protocol = 'snmp';
    }
    _normalizeNasFields();
    _enabled = t?.enabled ?? true;
    _name.text = t?.name ?? '';
    _host.text = t?.host ?? '';
    _port.text = (t?.port ?? 0) == 0 ? '' : '${t!.port}';
    _username.text = t?.host == null ? '' : ''; // 凭据不回传
    _hasPrivateKey = t?.hasPrivateKey ?? false;
    _rtsp.text = t?.rtspUrl ?? '';
    _interval.text = '${t?.intervalSec ?? 60}';
    _timeout.text = '${t?.timeoutSec ?? 15}';
    _diskPaths.text = t?.diskPaths ?? '';
    for (final c in [
      _name,
      _host,
      _port,
      _username,
      _password,
      _privateKey,
      _rtsp,
      _interval,
      _timeout,
      _diskPaths,
    ]) {
      c.addListener(_markDirty);
    }
  }
}
