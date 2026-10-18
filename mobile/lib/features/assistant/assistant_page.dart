import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:ptpatronus/api/agent.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/features/common/capped_list_notice.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';

part 'assistantopsmixin.dart';
part 'assistant_widgets.dart';

/// AI 助手页（对齐 Web Assistant.vue）：自然语言控制全平台 + 多轮上下文 + 深度思考。
/// 会话历史持久化；SSE 流式（fetch 流读）；思考折叠 + 工具调用卡 + 停止。
class AssistantPage extends StatefulWidget {
  const AssistantPage({super.key});
  @override
  State<AssistantPage> createState() => _AssistantPageState();
}

class _AssistantPageState extends State<AssistantPage> with AssistantOpsMixin {

  @override
  /// 危险工具二次确认：收到 confirm_request 时弹窗，用户选择后 POST 决断。
  /// await-for 在弹窗期间自然暂停——后端在 Resolve 前不下发后续帧，确认后同一 SSE 连接继续。
  Future<void> _handleConfirm(Map<String, dynamic> d) async {
    final name = (d['name'] as String?) ?? '';
    final summary = (d['summary'] as String?) ?? 'AI 想执行一个不可逆操作';
    final args = (d['arguments'] as String?) ?? '';
    final id = (d['id'] as String?) ?? '';
    final argLine = (args.isNotEmpty && args != '{}') ? '\n参数：$args' : '';
    if (!mounted) return;
    final approved = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('⚠️ 危险操作确认'),
        content: Text('$summary\n工具：$name$argLine'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('拒绝')),
          FilledButton(onPressed: () => Navigator.pop(context, true), child: const Text('允许执行')),
        ],
      ),
    );
    try {
      if (_activeId != null) await _api.confirmDecision(_activeId!, id, approved == true);
    } catch (e) {
      _toast('确认回传失败：$e');
    }
  }

  @override
  void _scrollToBottom() {
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (_scrollCtrl.hasClients) _scrollCtrl.jumpTo(_scrollCtrl.position.maxScrollExtent);
    });
  }

  @override
  void _toast(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg), duration: const Duration(seconds: 2)));
  }
  @override
  void initState() {
    super.initState();
    // /agent/* 除 GET /config 外全是 RequireAdmin：非管理员不发必然 403 的请求。
    if (ApiClient().isAdmin) _loadConversations();
  }

  @override
  void dispose() {
    _inputCtrl.dispose();
    _scrollCtrl.dispose();
    _api.cancel();
    super.dispose();
  }

  Future<void> _delete(int id) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('删除会话'),
        content: const Text('确定删除该会话？'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('取消')),
          TextButton(onPressed: () => Navigator.pop(context, true), child: const Text('删除')),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await _api.deleteConversation(id);
    } catch (e) {
      // 没有这层兜底，异常会逃出 async 的 onPressed 进未处理 Future：
      // 弹窗已关、列表没变、也不吭声——看起来就像「删除点了没反应」。
      // （同仓 filter_rules 的 _deleteCustom 就是这个写法。）
      _toast('删除会话失败：$e');
      return;
    }
    if (_activeId == id) _newChat();
    await _loadConversations();
  }

  @override
  Widget build(BuildContext context) {
    // 整页拦截（后端仍是权威鉴权，这里只挡 UI 入口；对齐系统设置页的既有惯例）：
    // 顶栏常驻的助手按钮与命令面板都到得了这一页，普通账号点开不该是一屏 403。
    if (!ApiClient().isAdmin) {
      return const PTPageScaffold(
        title: 'AI 助手',
        body: PTEmptyState(
          icon: Icons.admin_panel_settings_outlined,
          text: '仅管理员可访问',
        ),
      );
    }
    return PTPageScaffold(
      title: const Text('AI 助手', style: TextStyle(fontSize: 17, fontWeight: FontWeight.w700)),
      actions: [
        PTBarAction(icon: Icons.add_comment, label: '新对话', onPressed: _newChat),
        PTBarAction(icon: Icons.settings, label: '设置', onPressed: _openConfig),
      ],
      body: Column(children: [
        // 会话选择条
        if (_convs.isNotEmpty)
          Padding(
            padding: const EdgeInsets.fromLTRB(12, 6, 12, 0),
            child: Row(children: [
              Expanded(
                child: DropdownButton<int>(
                  value: _activeId,
                  isExpanded: true,
                  hint: const Text('选择会话'),
                  items: _convs.map((c) => DropdownMenuItem(value: c.id, child: Text(c.title.isEmpty ? '(新对话)' : c.title, maxLines: 1, overflow: TextOverflow.ellipsis))).toList(),
                  onChanged: (v) { if (v != null) _select(v); },
                ),
              ),
              IconButton(icon: const Icon(Icons.delete_outline, size: 20), onPressed: _activeId == null ? null : () => _delete(_activeId!), tooltip: '删除会话'),
            ]),
          ),
        if (_convsNotice.isNotEmpty)
          Padding(
            padding: const EdgeInsets.fromLTRB(12, 2, 12, 4),
            child: Text(_convsNotice, style: TextStyle(fontSize: 12, color: context.dimText)),
          ),
        Expanded(child: _msgList()),
        _composer(),
      ]),
    );
  }

}
