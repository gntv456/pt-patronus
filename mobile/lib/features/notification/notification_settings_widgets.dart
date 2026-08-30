// NotificationSettingsPage 的展示层扩展（无 setState 段；从 notification_settings_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/notification/notification_settings_page.dart';

extension on _NotificationSettingsPageState {
  Widget _group(
    String title,
    String subtitle,
    bool enabled,
    ValueChanged<bool> onToggle,
    List<Widget> children,
  ) {
    return Padding(
      padding: const EdgeInsets.only(top: 12),
      child: GlassCard(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        title,
                        style: const TextStyle(
                          fontSize: 15,
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                      const SizedBox(height: 2),
                      Text(
                        subtitle,
                        style: TextStyle(
                          fontSize: 12,
                          color: context.dimText,
                        ),
                      ),
                    ],
                  ),
                ),
                Switch(value: enabled, onChanged: onToggle),
              ],
            ),
            const SizedBox(height: 12),
            ...children,
          ],
        ),
      ),
    );
  }

  Widget _secretField(
    String label,
    TextEditingController ctrl,
    bool configured,
    String masked,
  ) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Expanded(
                child: Text(
                  label,
                  style: const TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w600,
                  ),
                ),
              ),
              _statusChip(configured, configured ? '已配置 $masked' : '未配置'),
            ],
          ),
          const SizedBox(height: 6),
          TextField(
            controller: ctrl,
            obscureText: true,
            decoration: InputDecoration(
              hintText: configured ? '留空不修改' : '请输入',
              isDense: true,
              border: OutlineInputBorder(
                borderRadius: BorderRadius.circular(12),
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _plainField(
    String label,
    TextEditingController ctrl, {
    String hint = '',
    String help = '',
  }) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            label,
            style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w600),
          ),
          const SizedBox(height: 6),
          TextField(
            controller: ctrl,
            keyboardType: TextInputType.url,
            decoration: InputDecoration(
              hintText: hint,
              isDense: true,
              border: OutlineInputBorder(
                borderRadius: BorderRadius.circular(12),
              ),
            ),
          ),
          if (help.isNotEmpty)
            Padding(
              padding: const EdgeInsets.only(top: 4),
              child: Text(
                help,
                style: TextStyle(fontSize: 11, color: context.dimText),
              ),
            ),
        ],
      ),
    );
  }

  Widget _statusChip(bool configured, String text) {
    final color = configured ? PTColors.success : PTColors.warn;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 9, vertical: 2),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.16),
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(
        text,
        style: TextStyle(
          color: color,
          fontSize: 11,
          fontWeight: FontWeight.w600,
        ),
      ),
    );
  }

}
