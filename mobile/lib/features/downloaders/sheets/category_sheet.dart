// 分类管理弹窗：新建/编辑/删除。
// （从 downloaders_page.dart 拆出，part of 主库；行为不变。）
part of 'package:ptpatronus/features/downloaders/downloaders_page.dart';

// 分类管理（qB）：列表 + 新建（含保存路径）/删除。
class _CategorySheet extends StatefulWidget {
  final int id;
  const _CategorySheet({required this.id});
  @override
  State<_CategorySheet> createState() => _CategorySheetState();
}

class _CategorySheetState extends State<_CategorySheet> {
  final _api = DownloaderApi();
  final _nameCtrl = TextEditingController();
  final _pathCtrl = TextEditingController();
  List<String> _cats = [];
  bool _loading = true;
  String _error = '';

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    try {
      final c = await _api.categories(widget.id);
      if (mounted) {
        setState(() {
          _cats = c;
          _loading = false;
          _error = '';
        });
      }
    } catch (e) {
      if (mounted) setState(() { _loading = false; _error = e.toString(); });
    }
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  Future<void> _add() async {
    final name = _nameCtrl.text.trim();
    if (name.isEmpty) return;
    try {
      await _api.createCategory(widget.id, name, _pathCtrl.text.trim());
      _nameCtrl.clear();
      _pathCtrl.clear();
      _toast('已添加');
      _load();
    } catch (e) {
      _toast(e.toString().replaceFirst('Exception: ', ''));
    }
  }

  Future<void> _del(String name) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('删除分类'),
        content: Text('确认删除分类「$name」？该分类下任务将变为无分类。'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: const Text('取消'),
          ),
          TextButton(
            onPressed: () => Navigator.pop(ctx, true),
            child: const Text('删除'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await _api.deleteCategories(widget.id, [name]);
      _toast('已删除');
      _load();
    } catch (e) {
      _toast(e.toString().replaceFirst('Exception: ', ''));
    }
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        const Padding(
          padding: EdgeInsets.all(14),
          child: Text(
            '分类管理',
            style: TextStyle(fontSize: 16, fontWeight: FontWeight.w600),
          ),
        ),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 14),
          child: Row(
            children: [
              Expanded(
                child: TextField(
                  controller: _nameCtrl,
                  decoration: const InputDecoration(
                    hintText: '新分类名',
                    isDense: true,
                    border: OutlineInputBorder(),
                  ),
                  onSubmitted: (_) => _add(),
                ),
              ),
              const SizedBox(width: 8),
              Expanded(
                child: TextField(
                  controller: _pathCtrl,
                  decoration: const InputDecoration(
                    hintText: '保存路径（可空）',
                    isDense: true,
                    border: OutlineInputBorder(),
                  ),
                ),
              ),
              const SizedBox(width: 8),
              IconButton.filled(tooltip: '添加分类', icon: const Icon(Icons.add), onPressed: _add),
            ],
          ),
        ),
        const Divider(height: 16),
        Expanded(
          child: _loading
              ? const PTLoadingView()
              : _error.isNotEmpty
              ? _sheetErrorView(_error, _load)
              : _cats.isEmpty
              ? Center(
                  child: Text('无分类', style: TextStyle(color: context.dimText)),
                )
              : ListView.builder(
                  itemCount: _cats.length,
                  itemBuilder: (_, i) {
                    final c = _cats[i];
                    return ListTile(
                      dense: true,
                      title: Text(c, style: const TextStyle(fontSize: 13)),
                      trailing: IconButton(
                        tooltip: '删除该分类',
                        icon: const Icon(Icons.delete_outline, size: 20),
                        onPressed: () => _del(c),
                      ),
                    );
                  },
                ),
        ),
      ],
    );
  }
}
