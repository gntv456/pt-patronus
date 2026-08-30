// 从 subscriptions_page.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/features/subscriptions/subscriptions_page.dart';

// 全局订阅默认设置对话框：5 项（分辨率/评分保护/洗版/促销/站点）。保存返回 GlobalDefaults。
class _GlobalDefaultsDialog extends StatefulWidget {
  final GlobalDefaults initial;
  final List<Site> sites;
  const _GlobalDefaultsDialog({required this.initial, required this.sites});
  @override
  State<_GlobalDefaultsDialog> createState() => _GlobalDefaultsDialogState();
}

class _GlobalDefaultsDialogState extends State<_GlobalDefaultsDialog> {
  late GlobalDefaults _d;
  late final TextEditingController _resCtrl;

  @override
  void initState() {
    super.initState();
    _d = widget.initial;
    _resCtrl = TextEditingController(text: _d.resolution);
  }

  @override
  void dispose() {
    _resCtrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: const Text('全局订阅默认设置'),
      content: SizedBox(
        width: double.maxFinite,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Padding(
                padding: EdgeInsets.only(bottom: 12),
                child: Text(
                  '未单独设置某项的订阅将使用此处的默认值（改后立即对所有未自定义订阅生效）。',
                  style: TextStyle(color: context.dimText, fontSize: 12),
                ),
              ),
              TextField(
                controller: _resCtrl,
                decoration: const InputDecoration(
                  isDense: true,
                  labelText: '分辨率',
                  hintText: '如 1080p/2160p，空=不限',
                ),
                onChanged: (v) => _d = _d.copyWith(resolution: v.trim()),
              ),
              SwitchListTile(
                dense: true,
                contentPadding: EdgeInsets.zero,
                title: const Text('评分保护'),
                value: _d.enableRatingGuard,
                onChanged: (v) => setState(() => _d = _d.copyWith(enableRatingGuard: v)),
              ),
              SwitchListTile(
                dense: true,
                contentPadding: EdgeInsets.zero,
                title: const Text('洗版'),
                value: _d.bestVersion,
                onChanged: (v) => setState(() => _d = _d.copyWith(bestVersion: v)),
              ),
              ListTile(
                dense: true,
                contentPadding: EdgeInsets.zero,
                title: const Text('促销类型'),
                subtitle: Text(
                  _d.promoTypes.isEmpty ? '不限' : _d.promoTypes.map(promoLabel).join('、'),
                  style: const TextStyle(fontSize: 12),
                ),
                trailing: const Icon(Icons.chevron_right, size: 20),
                onTap: () async {
                  final result = await showDialog<List<String>>(
                    context: context,
                    builder: (_) => _PromoTypeDialog(initial: _d.promoTypes),
                  );
                  if (result != null) setState(() => _d = _d.copyWith(promoTypes: result));
                },
              ),
              ListTile(
                dense: true,
                contentPadding: EdgeInsets.zero,
                title: const Text('参与站点'),
                subtitle: Text(
                  _d.siteIds.isEmpty ? '全部站点' : '${_d.siteIds.length} 个站点',
                  style: const TextStyle(fontSize: 12),
                ),
                trailing: const Icon(Icons.chevron_right, size: 20),
                onTap: () {
                  showSitePickerSheet(
                    context,
                    title: '全局默认参与站点',
                    selected: [..._d.siteIds],
                    options: widget.sites
                        .map((s) => PTFilterOption<Object?>(value: s.id, label: s.name))
                        .toList(),
                    onChanged: (v) => setState(() => _d = _d.copyWith(siteIds: v)),
                  );
                },
              ),
            ],
          ),
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: const Text('取消'),
        ),
        TextButton(
          onPressed: () => Navigator.pop(context, _d),
          child: const Text('保存'),
        ),
      ],
    );
  }
}
