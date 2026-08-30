// 从 subscriptions_page.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/features/subscriptions/subscriptions_page.dart';

// 促销类型多选对话框（CheckboxListTile 列表 + 确定/取消；空选=不限）。
class _PromoTypeDialog extends StatefulWidget {
  final List<String> initial;
  const _PromoTypeDialog({required this.initial});
  @override
  State<_PromoTypeDialog> createState() => _PromoTypeDialogState();
}

class _PromoTypeDialogState extends State<_PromoTypeDialog> {
  late final Set<String> _selected;

  @override
  void initState() {
    super.initState();
    _selected = {...widget.initial};
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: const Text('促销类型（空=不限）'),
      content: SizedBox(
        width: double.maxFinite,
        child: ListView(
          shrinkWrap: true,
          children: _promoOptions.map((o) {
            return CheckboxListTile(
              dense: true,
              contentPadding: const EdgeInsets.symmetric(horizontal: 8),
              value: _selected.contains(o.$1),
              title: Text(o.$2),
              onChanged: (v) => setState(() {
                if (v == true) {
                  _selected.add(o.$1);
                } else {
                  _selected.remove(o.$1);
                }
              }),
            );
          }).toList(),
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: const Text('取消'),
        ),
        TextButton(
          onPressed: () => Navigator.pop(context, _selected.toList()),
          child: const Text('确定'),
        ),
      ],
    );
  }
}
