// AssistantPage 的 AssistantOpsMixin mixin（含 setState 段；从 assistant_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/assistant/assistant_page.dart';

mixin AssistantOpsMixin on State<AssistantPage> {

  final _api = AgentApi();
  List<AgentConversation> _convs = [];
  // 会话下拉的取数窗口提示（单页 200 个封顶）。
  String _convsNotice = '';
  int? _activeId;
  List<AgentMessage> _messages = [];
  final _inputCtrl = TextEditingController();
  final _scrollCtrl = ScrollController();
  bool _sending = false;
  // 会话记录读取账面：读不出来时不能画成「这个会话还没说过话」。
  String _messagesNotice = '';
  // 「停止」是用户自己的动作，不该按「发送失败」出声。
  bool _userStopped = false;
  // 实时累积
  String _liveContent = '';
  String _liveReasoning = '';
  bool _hasLive = false;
  List<Map<String, dynamic>> _liveTools = [];

  // 宿主提供的钩子。
  void _toast(String msg);
  void _scrollToBottom();
  Future<void> _handleConfirm(Map<String, dynamic> d);
  Future<void> _loadConversations() async {
    try {
      final page = await _api.conversationsPage();
      _convs = page.items;
      _convsNotice = cappedListNotice(
        shown: page.items.length,
        total: page.total,
        truncated: page.truncated,
        unknownTotal: (n) => '已列出最近 $n 个会话，但总数读取失败——可能还有更多',
        knownTotal: (n, total) => '仅列出最近 $n 个会话（共 $total 个，更早的未加载）',
      );
      if (mounted && _activeId == null && _convs.isNotEmpty) {
        await _select(_convs.first.id);
      }
      setState(() {});
    } catch (e) {
      // 静默吞掉会让「没拉到」与「没有会话」长得一样，用户只会以为自己没建过对话。
      _convsNotice = '';
      _toast('加载会话列表失败：$e');
      if (mounted) setState(() {});
    }
  }

  Future<void> _select(int id) async {
    setState(() { _activeId = id; _hasLive = false; _messagesNotice = ''; });
    try {
      _messages = await _api.messages(id);
    } catch (e) {
      // 旧行为：清空 + 不吭声，界面回到「和助手对话」的空态——
      // 用户以为会话丢了，其实只是一次读取失败。
      _messages = [];
      _messagesNotice = '这个会话的记录读不出来：$e';
    } finally {
      if (mounted) setState(() {});
      _scrollToBottom();
    }
  }

  void _newChat() {
    _api.cancel();
    setState(() { _activeId = null; _messages = []; _hasLive = false; _messagesNotice = ''; });
  }

  /// 「停止」按钮：标记这是用户主动中断，再中断流——否则 catch 会把它报成发送失败。
  void _stop() {
    _userStopped = true;
    _api.cancel();
  }

  Future<void> _send() async {
    final text = _inputCtrl.value.text.trim();
    if (text.isEmpty || _sending) return;
    _inputCtrl.clear();
    _api.cancel();
    _userStopped = false;
    setState(() {
      _messages = [..._messages, AgentMessage(id: -1, role: 'user', content: text)];
      _liveContent = '';
      _liveReasoning = '';
      _liveTools = [];
      _hasLive = true;
      _sending = true;
    });
    _scrollToBottom();
    var liveError = '';
    try {
      await for (final ev in _api.chat(text, _activeId)) {
        if (!mounted) break;
        switch (ev.event) {
          case 'conversation':
            _activeId = ev.data['id'] as int?;
          case 'token':
            _liveContent += (ev.data['content'] as String?) ?? '';
          case 'reasoning':
            _liveReasoning += (ev.data['content'] as String?) ?? '';
          case 'tool_call':
            _liveTools.add({'id': ev.data['id'], 'name': ev.data['name'], 'arguments': ev.data['arguments'], 'result': null});
          case 'tool_result':
            final tid = ev.data['id'];
            for (final t in _liveTools) {
              if (t['id'] == tid && t['result'] == null) {
                t['result'] = ev.data['result'];
                break;
              }
            }
          case 'confirm_request':
            await _handleConfirm(ev.data);
          case 'error':
            liveError = (ev.data['message'] as String?) ?? '本轮未完成';
            _toast(liveError);
        }
        setState(() {});
        _scrollToBottom();
      }
    } catch (e) {
      if (_userStopped) {
        liveError = '本轮已停止';
      } else {
        liveError = '$e';
        _toast('发送失败：$e');
      }
    } finally {
      _hasLive = false;
      if (mounted) setState(() => _sending = false);
    }
    final conv = _activeId;
    if (conv != null) await _select(conv);
    await _loadConversations();
    // 出错那一轮后端不落助手消息：刷新后记录里只剩用户那句，用户看不出这轮跑过、
    // 也不知道为什么没结果（重发还可能再触发一次写工具）。把原因钉进会话记录。
    if (liveError.isNotEmpty && mounted && _activeId == conv) {
      setState(() => _messages = [..._messages, AgentMessage(id: -2, role: 'assistant', content: '⚠ $liveError')]);
      _scrollToBottom();
    }
  }
}
