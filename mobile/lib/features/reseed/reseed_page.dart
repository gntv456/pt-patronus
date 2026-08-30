import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:ptpatronus/api/downloader.dart';
import 'package:ptpatronus/api/reseed.dart';
import 'package:ptpatronus/api/site.dart';
import 'package:ptpatronus/core/responsive.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_filter_sheet.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';
import 'package:url_launcher/url_launcher.dart';

part 'reseedopsmixin.dart';
part 'reseed_task_cards.dart';
part 'reseed_widgets.dart';

part 'missing_check_sheet.dart';

/// 辅种页（IYUU 精确匹配，对齐 Web Reseed.vue）：配置 token → 选下载器开始辅种 →
/// 候选任务审阅（宽屏表格 / 窄屏卡片）→ 添加辅种/跳过。复用后端 /keepseed/*。
class ReseedPage extends StatefulWidget {
  const ReseedPage({super.key});
  @override
  State<ReseedPage> createState() => _ReseedPageState();
}

class _ReseedPageState extends State<ReseedPage> with ReseedOpsMixin {

  @override
  void dispose() {
    _tokenCtrl.dispose();
    _savePathCtrl.dispose();
    super.dispose();
  }

  @override
  void _toast(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
  }
  @override
  void initState() {
    super.initState();
    _loadAll();
  }

  Future<void> _loadAll() async {
    await Future.wait([_loadConfig(), _loadDownloaders(), _loadTasks()]);
  }

  Color _statusColor(String s) => switch (s) {
    'added' => PTColors.success,
    'failed' => PTColors.danger,
    'pending' => PTColors.warn,
    _ => context.dimText,
  };

  String _statusLabel(String s) => switch (s) {
    'added' => '已添加',
    'failed' => '失败',
    'pending' => '待处理',
    'skipped' => '已跳过',
    _ => s,
  };

  @override
  Widget build(BuildContext context) {
    return PTPageScaffold(
      title: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          const Text(
            '辅种',
            style: TextStyle(fontSize: 17, fontWeight: FontWeight.w700),
          ),
          const SizedBox(width: 8),
          _iyuuBadge(),
        ],
      ),
      actions: [
        PTBarAction(
          icon: Icons.refresh,
          label: '刷新',
          loading: _loading,
          onPressed: _loading ? null : _loadTasks,
        ),
        PTBarAction(
          icon: Icons.open_in_new,
          label: 'iyuu.cn',
          onPressed: _openIyuu,
          overflow: true,
        ),
      ],
      filterDimensions: _filterDimensions(),
      filterOnClearAll: () {
        setState(() => _status = '');
        _loadTasks();
      },
      body: Padding(
        padding: const EdgeInsets.fromLTRB(14, 4, 14, 2),
        child: isWideScreen(context) ? _wideBody() : _narrowBody(),
      ),
    );
  }

  Widget _reseedCard() {
    return GlassCard(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text(
            '开始辅种',
            style: TextStyle(fontWeight: FontWeight.w600, fontSize: 15),
          ),
          const SizedBox(height: 8),
          Row(
            children: [
              Expanded(
                child: DropdownButton<int>(
                  value: _selectedDl,
                  isExpanded: true,
                  hint: const Text('选择下载器'),
                  items: _downloaders
                      .map(
                        (d) =>
                            DropdownMenuItem(value: d.id, child: Text(d.name)),
                      )
                      .toList(),
                  onChanged: (v) {
                    setState(() => _selectedDl = v);
                    if (v != null) _loadSetting();
                  },
                ),
              ),
              const SizedBox(width: 10),
              FilledButton.icon(
                onPressed: _reseeding ? null : _startReseed,
                icon: _reseeding
                    ? const SizedBox(
                        width: 16,
                        height: 16,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : const Icon(Icons.eco_outlined, size: 18),
                label: const Text('开始辅种'),
              ),
            ],
          ),
          const SizedBox(height: 10),
          // 未辅种检查：对当前下载器的做种，按目标站逐站比对 IYUU 目录，列出未辅种种子。
          Align(
            alignment: Alignment.centerLeft,
            child: FilledButton.tonalIcon(
              onPressed: _selectedDl == null ? null : _openMissingCheck,
              icon: const Icon(Icons.search_off_outlined, size: 18),
              label: const Text('未辅种检查'),
            ),
          ),
          const SizedBox(height: 4),
          _buildAutoRow(),
        ],
      ),
    );
  }

  // 状态筛选维度（收进顶栏筛选按钮）：选状态 → 重新拉该状态任务。
  List<PTFilterDimension> _filterDimensions() {
    return [
      PTFilterDimension.choice(
        label: '状态',
        selected: _status,
        isDefault: _status.isEmpty,
        onChanged: (v) {
          setState(() => _status = (v as String?) ?? '');
          _loadTasks();
        },
        options: const [
          PTFilterOption<Object?>(value: '', label: '全部'),
          PTFilterOption<Object?>(value: 'pending', label: '待处理'),
          PTFilterOption<Object?>(value: 'added', label: '已添加'),
          PTFilterOption<Object?>(value: 'skipped', label: '已跳过'),
          PTFilterOption<Object?>(value: 'failed', label: '失败'),
        ],
      ),
    ];
  }

}
