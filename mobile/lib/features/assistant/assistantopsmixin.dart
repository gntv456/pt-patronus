// AssistantPage 的 AssistantOpsMixin mixin（含 setState 段；从 assistant_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/assistant/assistant_page.dart';

mixin AssistantOpsMixin on State<AssistantPage> {

  final _api = AgentApi();
  List<AgentConversation> _convs = [];
  int? _activeId;
  List<AgentMessage> _messages = [];
  final _inputCtrl = TextEditingController();
  final _scrollCtrl = ScrollController();
  bool _sending = false;
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
      _convs = await _api.conversations();
      if (mounted && _activeId == null && _convs.isNotEmpty) {
        await _select(_convs.first.id);
      }
      setState(() {});
    } catch (_) {}
  }

  Future<void> _select(int id) async {
    setState(() { _activeId = id; _hasLive = false; });
    try {
      _messages = await _api.messages(id);
    } catch (_) {
      _messages = [];
    } finally {
      if (mounted) setState(() {});
      _scrollToBottom();
    }
  }

  void _newChat() {
    _api.cancel();
    setState(() { _activeId = null; _messages = []; _hasLive = false; });
  }

  Future<void> _send() async {
    final text = _inputCtrl.value.text.trim();
    if (text.isEmpty || _sending) return;
    _inputCtrl.clear();
    _api.cancel();
    setState(() {
      _messages = [..._messages, AgentMessage(id: -1, role: 'user', content: text)];
      _liveContent = '';
      _liveReasoning = '';
      _liveTools = [];
      _hasLive = true;
      _sending = true;
    });
    _scrollToBottom();
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
            _toast((ev.data['message'] as String?) ?? '错误');
        }
        setState(() {});
        _scrollToBottom();
      }
      _hasLive = false;
      if (_activeId != null) await _select(_activeId!);
      await _loadConversations();
    } catch (e) {
      _toast('发送失败：$e');
    } finally {
      if (mounted) setState(() => _sending = false);
    }
  }
}
