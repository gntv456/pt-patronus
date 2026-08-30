// _FileManagePageState 的展示层：toast/父路径/后端下拉/路径栏/条目行/错误视图 + 页面构建。
// （无 setState 段；从 file_manage_page.dart 拆出，行为不变。）
part of 'package:ptpatronus/features/file_manage/file_manage_page.dart';

extension on _FileManagePageState {

  String _parentOf(String p) {
    final segs = p.split('/').where((s) => s.isNotEmpty).toList();
    if (segs.isEmpty) return '/';
    segs.removeLast();
    return segs.isEmpty ? '/' : '/${segs.join('/')}';
  }

  Widget _backendDropdown(BuildContext context) {
    if (_backends.isEmpty) {
      return Text(
        '无存储后端',
        style: TextStyle(color: context.dimText, fontSize: 13),
      );
    }
    return DropdownButton<_Backend>(
      value: _cur,
      underline: const SizedBox(),
      hint: const Text('选择存储'),
      items: _backends
          .map(
            (b) => DropdownMenuItem(
              value: b,
              child: Text(
                  '${_statusDot(b.key)}${b.kind == 'cloud' ? '☁ ' : ''}${b.name}（${b.typeName}）'),
            ),
          )
          .toList(),
      onChanged: (v) {
        if (v != null && v != _cur) _select(v);
      },
    );
  }

  Widget _pathBar() {
    final segs = _path.split('/').where((s) => s.isNotEmpty).toList();
    // 单行横向滚动面包屑，深路径永不折行（取代旧 Wrap 折多行吃竖向空间）。
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 4, 12, 6),
      child: SizedBox(
        height: 38,
        child: SingleChildScrollView(
          scrollDirection: Axis.horizontal,
          child: Row(
            children: [
              InkWell(
                onTap: segs.isEmpty ? null : () => _cd(_parentOf(_path)),
                borderRadius: BorderRadius.circular(8),
                child: const Padding(
                  padding: EdgeInsets.symmetric(horizontal: 6, vertical: 8),
                  child: Icon(Icons.arrow_upward, size: 18),
                ),
              ),
              ActionChip(label: const Text('根'), onPressed: () => _cd('/')),
              ...segs.asMap().entries.map(
                (e) => ActionChip(
                  label: Text(e.value),
                  onPressed: () =>
                      _cd('/${segs.sublist(0, e.key + 1).join('/')}'),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _entryRow(BuildContext context, FileEntry e) => Padding(
    padding: const EdgeInsets.only(bottom: 10),
    child: GlassCard(
      padding: const EdgeInsets.fromLTRB(14, 10, 8, 10),
      onTap: e.isDir ? () => _cd(e.path) : null,
      child: Row(
        children: [
          Icon(
            e.isDir ? Icons.folder : Icons.insert_drive_file,
            size: 24,
            color: e.isDir ? PTColors.primary2 : context.dimText,
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  e.name,
                  style: const TextStyle(
                    fontWeight: FontWeight.w600,
                    fontSize: 15,
                  ),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
                const SizedBox(height: 2),
                Text(
                  e.isDir ? '文件夹' : _fmtBytes(e.size),
                  style: TextStyle(color: context.dimText, fontSize: 12),
                ),
              ],
            ),
          ),
          if (!e.isDir && !_readOnly)
            IconButton(
              icon: const Icon(Icons.download, size: 20),
              tooltip: '下载',
              onPressed: _busy ? null : () => _download(e),
            ),
          if (_readOnly && e.isDir)
            IconButton(
              icon: const Icon(Icons.link, size: 20),
              tooltip: '生成 STRM',
              onPressed: _busy ? null : () => _strm(e.path),
            ),
          if (!_readOnly)
            PopupMenuButton<String>(
              icon: const Icon(Icons.more_vert, size: 20),
              tooltip: '更多',
              enabled: !_busy,
              onSelected: (cmd) {
                if (cmd == 'rename') {
                  _rename(e);
                } else if (cmd == 'move') {
                  _moveCopy(e, 'move');
                } else if (cmd == 'copy') {
                  _moveCopy(e, 'copy');
                } else if (cmd == 'delete') {
                  _delete(e);
                }
              },
              itemBuilder: (_) => const [
                PopupMenuItem(value: 'rename', child: Text('重命名')),
                PopupMenuItem(value: 'move', child: Text('移动')),
                PopupMenuItem(value: 'copy', child: Text('复制')),
                PopupMenuItem(value: 'delete', child: Text('删除')),
              ],
            ),
        ],
      ),
    ),
  );

  Widget _errorView() => Center(
    child: Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        const Icon(Icons.error_outline, size: 40, color: Colors.redAccent),
        const SizedBox(height: 8),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 32),
          child: Text(
            _error ?? '加载失败',
            textAlign: TextAlign.center,
            style: const TextStyle(color: Colors.redAccent, fontSize: 14),
          ),
        ),
        const SizedBox(height: 12),
        FilledButton.tonalIcon(
          icon: const Icon(Icons.refresh, size: 18),
          label: const Text('重试'),
          onPressed: _loadEntries,
        ),
      ],
    ),
  );

  Widget _buildScaffold(BuildContext context) {
    final b = _cur;
    final hasStorage = b != null;
    final readOnly = b?.readOnly ?? false;
    return PTPageScaffold(
      title: '文件管理',
      actions: [
        PTBarAction(
          icon: Icons.refresh,
          label: '刷新',
          onPressed: hasStorage ? _loadEntries : null,
        ),
        // 云盘只读模式：对当前目录（含根目录，根无列表行无操作入口）生成 STRM。
        if (readOnly)
          PTBarAction(
            icon: Icons.link,
            label: '生成 STRM',
            onPressed: _busy ? null : () => _strm(_path),
          ),
        PTBarAction(
          icon: Icons.create_new_folder,
          label: '新建文件夹',
          onPressed: (hasStorage && !_busy && !readOnly) ? _mkdir : null,
          overflow: true,
        ),
        PTBarAction(
          icon: Icons.add,
          label: '添加存储',
          onPressed: _addStorage,
          overflow: true,
        ),
        PTBarAction(
          icon: Icons.network_check,
          label: '测试连接',
          onPressed: hasStorage ? _testCurrent : null,
          overflow: true,
        ),
        PTBarAction(
          icon: Icons.edit,
          label: '编辑当前存储',
          onPressed: (hasStorage && b.fs != null) ? _editStorage : null,
          overflow: true,
        ),
        PTBarAction(
          icon: Icons.delete,
          label: '删除当前存储',
          onPressed: (hasStorage && b.fs != null) ? _deleteStorage : null,
          overflow: true,
        ),
      ],
      floatingActionButton: (!hasStorage || readOnly)
          ? null
          : FloatingActionButton(
              tooltip: '上传文件',
              onPressed: _busy ? null : _upload,
              child: const Icon(Icons.upload_file),
            ),
      body: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 4, 16, 0),
            child: Row(
              children: [
                _backendDropdown(context),
                const Spacer(),
                if (_busy)
                  const SizedBox(
                    width: 18,
                    height: 18,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  ),
              ],
            ),
          ),
          if (hasStorage) _pathBar(),
          if (_loading) const LinearProgressIndicator(minHeight: 2),
          Expanded(
            child: _backends.isEmpty && !_loading
                ? Center(
                    child: Text(
                      '暂无存储后端，请点右上角更多添加 本机 / WebDAV / Alist / SMB / Agent（云盘在「云盘」页管理）',
                      style: TextStyle(color: context.dimText),
                    ),
                  )
                : !hasStorage
                ? const SizedBox.shrink()
                : (_error != null && _entries.isEmpty
                      ? _errorView()
                      : _entries.isEmpty && !_loading
                      ? Center(
                          child: Text(
                            '空目录',
                            style: TextStyle(color: context.dimText),
                          ),
                        )
                      : ListView.builder(
                          padding: const EdgeInsets.fromLTRB(12, 4, 12, 80),
                          itemCount: _entries.length,
                          itemBuilder: (ctx, i) => _entryRow(ctx, _entries[i]),
                        )),
          ),
        ],
      ),
    );
  }
}
