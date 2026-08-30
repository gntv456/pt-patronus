// 存储挂载添加/编辑表单弹窗。
// （从 file_manage_page.dart 拆出，part of 主库；行为不变。）
part of 'package:ptpatronus/features/file_manage/file_manage_page.dart';

/// 存储后端新建/编辑表单：name/type(webdav/alist/smb/agent/local)/agent选择器/url/user/pass/base_path。
/// 编辑态密码留空=不修改。type=agent 用 Agent 选择器替代地址凭据；type=local 仅需根路径（本机物理目录）（对齐 Web FileManager.vue）。
class _StorageFormSheet extends StatefulWidget {
  final FileStorage? editing; // null=新建
  const _StorageFormSheet({this.editing});
  @override
  State<_StorageFormSheet> createState() => _StorageFormSheetState();
}

class _StorageFormSheetState extends State<_StorageFormSheet> {
  final _api = FileStorageApi();
  final _agentApi = AgentApi();
  final _name = TextEditingController();
  final _url = TextEditingController();
  final _username = TextEditingController();
  final _password = TextEditingController();
  final _basePath = TextEditingController(text: '/');
  String _type = 'webdav';
  List<Agent> _agents = const [];
  int _agentId = 0;
  bool _saving = false;
  bool _loadingAgents = true;

  bool get _isEdit => widget.editing != null;

  @override
  void initState() {
    super.initState();
    final e = widget.editing;
    if (e != null) {
      _name.text = e.name;
      _type = e.type.isEmpty ? 'webdav' : e.type;
      _url.text = e.url;
      _username.text = e.username;
      _basePath.text = e.basePath.isEmpty ? '/' : e.basePath;
      _agentId = e.agentId;
    }
    _agentApi
        .list()
        .then((l) {
          if (mounted) {
            setState(() {
              _agents = l;
              _loadingAgents = false;
            });
          }
        })
        .catchError((_) {
          if (mounted) setState(() => _loadingAgents = false);
        });
  }

  @override
  void dispose() {
    _name.dispose();
    _url.dispose();
    _username.dispose();
    _password.dispose();
    _basePath.dispose();
    super.dispose();
  }

  void _toast(String s) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(s)));
  }

  Future<void> _submit() async {
    final name = _name.text.trim();
    if (name.isEmpty) {
      _toast('请填写名称');
      return;
    }
    if (_type == 'agent') {
      if (_agentId == 0) {
        _toast('请选择 Agent');
        return;
      }
    } else if (_type == 'rclone') {
      if (_username.text.trim().isEmpty) {
        _toast('请填写 Remote 名');
        return;
      }
    } else if (_type != 'local' && _url.text.trim().isEmpty) {
      _toast('请填写地址');
      return;
    }
    setState(() => _saving = true);
    try {
      final in_ = FileStorageInput(
        name: name,
        type: _type,
        url: _url.text.trim(),
        username: _username.text.trim(),
        password: _password.text,
        basePath: _basePath.text.trim().isEmpty ? '/' : _basePath.text.trim(),
        agentId: _agentId,
      );
      if (_isEdit) {
        await _api.update(widget.editing!.id, in_);
        _toast('已更新');
      } else {
        await _api.create(in_);
        _toast('已添加');
      }
      if (mounted) Navigator.pop(context, true);
    } catch (e) {
      _toast(e.toString().replaceFirst('Exception: ', ''));
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        TextField(
          controller: _name,
          decoration: const InputDecoration(labelText: '名称'),
        ),
        const SizedBox(height: 12),
        DropdownButtonFormField<String>(
          initialValue: _type,
          decoration: const InputDecoration(labelText: '类型'),
          items: const [
            DropdownMenuItem(value: 'local', child: Text('本机')),
            DropdownMenuItem(value: 'webdav', child: Text('WebDAV')),
            DropdownMenuItem(value: 'alist', child: Text('Alist')),
            DropdownMenuItem(value: 'smb', child: Text('SMB')),
            DropdownMenuItem(value: 'agent', child: Text('Agent')),
            DropdownMenuItem(value: 'rclone', child: Text('Rclone')),
          ],
          onChanged: (v) => setState(() => _type = v ?? 'webdav'),
        ),
        if (_type == 'agent') ...[
          const SizedBox(height: 12),
          // agents 异步加载：加载完再建下拉，initialValue 才能正确预选编辑态的 agent。
          _loadingAgents
              ? const Padding(
                  padding: EdgeInsets.symmetric(vertical: 8),
                  child: Align(
                    alignment: Alignment.centerLeft,
                    child: SizedBox(
                      width: 16,
                      height: 16,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    ),
                  ),
                )
              : DropdownButtonFormField<int>(
                  initialValue:
                      (_agentId == 0 || _agents.any((a) => a.id == _agentId))
                      ? _agentId
                      : 0,
                  decoration: const InputDecoration(labelText: 'Agent'),
                  items: [
                    const DropdownMenuItem(value: 0, child: Text('选择 Agent')),
                    ..._agents.map(
                      (a) => DropdownMenuItem(
                        value: a.id,
                        child: Text('${a.name}${a.online ? '' : '（离线）'}'),
                      ),
                    ),
                  ],
                  onChanged: (v) => setState(() => _agentId = v ?? 0),
                ),
          const SizedBox(height: 8),
          Text(
            '选择已部署 ptpatronus-agent 的设备（飞牛/绿联等），根路径填 agent 机器上的绝对路径。设备离线或版本过低（需 0.2.0+）时文件操作不可用。',
            style: TextStyle(fontSize: 11, color: context.dimText, height: 1.5),
          ),
        ] else if (_type == 'local') ...[
          const SizedBox(height: 12),
          Text(
            '直接读写 PTPatronus 服务所在机器的本地目录，无需开网络服务。根路径填本机绝对路径（如 Windows D:\\Media 或 Linux /mnt/nas）。后端进程拥有该目录完整权限，请仅配置可信目录。',
            style: TextStyle(fontSize: 11, color: context.dimText, height: 1.5),
          ),
        ] else if (_type == 'rclone') ...[
          const SizedBox(height: 12),
          TextField(
            controller: _url,
            decoration: const InputDecoration(
              labelText: 'rclone 二进制路径',
              hintText: '留空=用 PATH 中的 rclone',
            ),
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _username,
            decoration: const InputDecoration(
              labelText: 'Remote 名',
              hintText: 'rclone.conf 中的 [section]，如 myremote',
            ),
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _password,
            minLines: 6,
            maxLines: 12,
            decoration: InputDecoration(
              labelText: 'rclone.conf 内容',
              hintText: _isEdit ? '留空不修改' : '粘贴 rclone.conf INI 全文',
              alignLabelWithHint: true,
            ),
          ),
          const SizedBox(height: 8),
          Text(
            '通过 rclone CLI 访问远端（非挂载）。后端把 conf 临时写盘 + --config 调用；操作需服务端机器装有 rclone。根路径填 remote 内的子路径。',
            style: TextStyle(fontSize: 11, color: context.dimText, height: 1.5),
          ),
        ] else ...[
          const SizedBox(height: 12),
          TextField(
            controller: _url,
            decoration: const InputDecoration(
              labelText: '地址',
              hintText: 'http://host:port',
            ),
            keyboardType: TextInputType.url,
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _username,
            decoration: const InputDecoration(labelText: '用户名'),
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _password,
            obscureText: true,
            decoration: InputDecoration(
              labelText: '密码',
              hintText: _isEdit ? '留空不修改' : null,
            ),
          ),
        ],
        const SizedBox(height: 12),
        TextField(
          controller: _basePath,
          decoration: const InputDecoration(labelText: '根路径'),
        ),
        const SizedBox(height: 20),
        FilledButton(
          onPressed: _saving ? null : _submit,
          child: _saving
              ? const SizedBox(
                  width: 16,
                  height: 16,
                  child: CircularProgressIndicator(
                    strokeWidth: 2,
                    color: Colors.white,
                  ),
                )
              : Text(_isEdit ? '保存' : '添加'),
        ),
      ],
    );
  }
}
