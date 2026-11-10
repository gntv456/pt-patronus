// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// ReseedPage 的展示层扩展（无 setState 段；从 reseed_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/reseed/reseed_page.dart';

extension on _ReseedPageState {
  // IYUU 配置状态徽标（原 header 内联，现并入标题区）。
  Widget _iyuuBadge() {
    final on = _configured;
    final c = on ? PTColors.success : PTColors.warn;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
      decoration: BoxDecoration(
        color: c.withValues(alpha: 0.16),
        borderRadius: BorderRadius.circular(999),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            width: 6,
            height: 6,
            decoration: BoxDecoration(color: c, shape: BoxShape.circle),
          ),
          const SizedBox(width: 5),
          Text(
            on ? 'IYUU 已配置' : 'IYUU 未配置',
            style: TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w600,
              color: c,
            ),
          ),
        ],
      ),
    );
  }

  // 窄屏：纵向卡片列表（配置 + 辅种 + 任务卡片）。
  Widget _narrowBody() {
    return ListView(
      children: [
        _configCard(),
        const SizedBox(height: 14),
        _reseedCard(),
        const SizedBox(height: 14),
        _tasksArea(narrow: true),
      ],
    );
  }

  // 宽屏：配置/辅种两栏并排 + 任务表格。
  Widget _wideBody() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Expanded(child: _configCard()),
            const SizedBox(width: 14),
            Expanded(child: _reseedCard()),
          ],
        ),
        const SizedBox(height: 14),
        Expanded(child: _tasksArea(narrow: false)),
      ],
    );
  }


  Widget _configCard() {
    return GlassCard(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text(
            'IYUU 配置',
            style: TextStyle(fontWeight: FontWeight.w600, fontSize: 15),
          ),
          const SizedBox(height: 8),
          TextField(
            controller: _tokenCtrl,
            obscureText: true,
            decoration: InputDecoration(
              isDense: true,
              hintText: _configured ? '已配置：$_masked（输入新值覆盖）' : '粘贴 IYUU token',
              border: const OutlineInputBorder(),
            ),
          ),
          const SizedBox(height: 8),
          FilledButton.icon(
            onPressed: _savingToken ? null : _saveToken,
            icon: const Icon(Icons.save_outlined, size: 18),
            label: const Text('保存 token'),
          ),
        ],
      ),
    );
  }

  // 该下载器的辅种设置：自动辅种 / 自动添加 / 保存路径 / 添加后暂停。
  Widget _buildAutoRow() {
    if (_selectedDl == null) return const SizedBox.shrink();
    final autoReseed = _setting?.autoReseed ?? false;
    final autoReseedAdd = _setting?.autoReseedAdd ?? false;
    final pauseAfterAdd = _setting?.pauseAfterAdd ?? true;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Divider(height: 24),
        Row(
          children: [
            const Expanded(
              child: Text('定时自动辅种', style: TextStyle(fontSize: 13)),
            ),
            Switch(
              value: autoReseed,
              onChanged: _loadingSetting
                  ? null
                  : (v) => _toggleAuto(autoReseed: v),
            ),
          ],
        ),
        Row(
          children: [
            Expanded(
              child: Text(
                '自动添加新候选',
                style: TextStyle(fontSize: 13, color: context.dimText),
              ),
            ),
            Switch(
              value: autoReseedAdd,
              onChanged: autoReseed
                  ? ((v) => _toggleAuto(autoReseedAdd: v))
                  : null,
            ),
          ],
        ),
        Row(
          children: [
            Expanded(
              child: Text(
                '添加后暂停',
                style: TextStyle(fontSize: 13, color: context.dimText),
              ),
            ),
            Switch(
              value: pauseAfterAdd,
              onChanged: _loadingSetting ? null : _togglePause,
            ),
          ],
        ),
        const SizedBox(height: 6),
        Row(
          children: [
            Expanded(
              child: TextField(
                controller: _savePathCtrl,
                decoration: const InputDecoration(
                  hintText: '保存路径（留空=下载器默认）',
                  border: OutlineInputBorder(),
                  isDense: true,
                ),
              ),
            ),
            const SizedBox(width: 8),
            FilledButton.tonal(
              onPressed: _savePathAndPause,
              child: const Text('保存路径'),
            ),
          ],
        ),
        Text(
          '每天 3 点扫描（KEEPSEED_CRON 可改）',
          style: TextStyle(color: context.dimText, fontSize: 11),
        ),
      ],
    );
  }


  // 打开「未辅种检查」面板：选目标站 → 后端按站比对 IYUU 目录 → 列出每站缺失种子。
  void _openMissingCheck() {
    if (_selectedDl == null) {
      _toast('请先选择下载器');
      return;
    }
    final match = _downloaders.where((d) => d.id == _selectedDl).toList();
    final name = match.isNotEmpty ? match.first.name : '下载器 $_selectedDl';
    showAdaptiveSheet(
      context,
      title: '未辅种检查',
      wideWidth: 620,
      builder: (_) => _MissingCheckSheet(
        downloaderId: _selectedDl!,
        downloaderName: name,
      ),
    );
  }

  Future<void> _copyLink(String url) async {
    if (url.isEmpty) return;
    await Clipboard.setData(ClipboardData(text: url));
    _toast('已复制下载链接');
  }

  void _openIyuu() async {
    const url = 'https://iyuu.cn';
    if (!await launchUrl(
      Uri.parse(url),
      mode: LaunchMode.externalApplication,
    )) {
      _toast(url);
    }
  }

  void _toast(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
  }
}
