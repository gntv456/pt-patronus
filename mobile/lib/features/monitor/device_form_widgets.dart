// DeviceFormSheet 的展示层扩展（无 setState 段；从 device_form_sheet.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/monitor/device_form_sheet.dart';

extension on _DeviceFormSheetState {
  String _defaultProtocol(String type) => switch (type) {
    'host' => 'local',
    'nas' => 'local',
    'camera' => 'onvif',
    _ => '',
  };

  String _inferredProtocol(MonitorTarget t) {
    if (t.protocol.isNotEmpty) return t.protocol;
    if (t.type == 'host') return t.host.isEmpty ? 'local' : 'ssh';
    if (t.type == 'nas') return t.host.isEmpty ? 'local' : 'api';
    return _defaultProtocol(t.type);
  }

  List<String> _protocols(String type) => switch (type) {
    'host' => ['local', 'ssh', 'agent'],
    'nas' => ['local', 'agent', 'api', 'snmp'],
    'camera' => ['onvif', 'rtsp'],
    _ => [''],
  };

  String _protocolLabel(String protocol) => switch (protocol) {
    'local' => '本机',
    'ssh' => 'SSH 远程',
    'agent' => 'Agent 上报',
    'api' => '群晖 / TrueNAS API',
    'snmp' => '通用 SNMP',
    'onvif' => 'ONVIF',
    'rtsp' => 'RTSP 直连',
    _ => protocol,
  };


  Widget _field(
    String label,
    TextEditingController ctrl, {
    String hint = '',
    bool number = false,
  }) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 4),
      child: Row(
        children: [
          SizedBox(
            width: 72,
            child: Text(label, style: const TextStyle(fontSize: 14)),
          ),
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

}
