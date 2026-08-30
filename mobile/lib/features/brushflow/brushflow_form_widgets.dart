// _BrushFlowFormState 的展示层扩展：字段行/开关行/解析工具/toast。
// （从 brushflow_form.dart 拆出；行为不变。）
part of 'package:ptpatronus/features/brushflow/brushflow_page.dart';

extension on _BrushFlowFormState {
  /// 统一的「标签 + 控件」字段行。
  Widget _field(String label, Widget child) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            label,
            style: TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.w600,
              color: context.dimText,
            ),
          ),
          const SizedBox(height: 6),
          child,
        ],
      ),
    );
  }

  /// 统一的开关行。
  Widget _switchRow(String label, bool value, ValueChanged<bool> onChanged) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 4),
      child: Row(
        children: [
          Expanded(child: Text(label, style: const TextStyle(fontSize: 13))),
          Switch(value: value, onChanged: onChanged),
        ],
      ),
    );
  }

  int _parseInt(String s, [int def = 0]) => int.tryParse(s.trim()) ?? def;
  double _parseDouble(String s, [double def = 0]) =>
      double.tryParse(s.trim()) ?? def;

  void _toast(String msg) {
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
  }

}
