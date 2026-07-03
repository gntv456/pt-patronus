import 'dart:convert';

import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:ptpatronus/features/home/home_menu_button.dart';
import 'package:flutter/services.dart';
import 'package:ptpatronus/api/backup.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:share_plus/share_plus.dart';

/// 备份页：导出 / 导入（PTPatronus 格式）/ 导入 MoviePilot 站点。对齐 Web Backup.vue。
class BackupPage extends StatefulWidget {
  const BackupPage({super.key});
  @override
  State<BackupPage> createState() => _BackupPageState();
}

class _BackupPageState extends State<BackupPage> {
  final _api = BackupApi();
  bool _busy = false;

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  Future<void> _export() async {
    setState(() => _busy = true);
    try {
      final bytes = await _api.export();
      await Share.shareXFiles([
        XFile.fromData(bytes, mimeType: 'application/json', name: 'ptpatronus-backup.json'),
      ]);
      _toast('已导出');
    } on MissingPluginException {
      _toast('当前平台不支持分享/保存，请用 Web 端导出');
    } catch (e) {
      _toast(e.toString().replaceFirst('Exception: ', ''));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _import(bool moviepilot) async {
    setState(() => _busy = true);
    try {
      final result = await FilePicker.platform.pickFiles(
        type: FileType.custom,
        allowedExtensions: ['json'],
        withData: true,
      );
      if (result == null || result.files.isEmpty) return;
      final bytes = result.files.first.bytes;
      if (bytes == null) {
        _toast('无法读取所选文件');
        return;
      }
      final decoded = jsonDecode(utf8.decode(bytes));
      if (moviepilot) {
        if (decoded is! List) {
          _toast('文件应为 MoviePilot 站点 JSON 数组');
          return;
        }
        final n = await _api.importMoviePilot(decoded);
        _toast('已导入 $n 个站点');
      } else {
        if (decoded is! Map) {
          _toast('文件应为 PTPatronus 备份 JSON 对象');
          return;
        }
        final r = await _api.importOwn(decoded as Map<String, dynamic>);
        _toast('导入完成：站点 ${r.sites} / 下载器 ${r.downloaders}');
      }
    } on MissingPluginException {
      _toast('选文件不可用，请用 Web 端导入');
    } catch (e) {
      _toast(e.toString().replaceFirst('Exception: ', ''));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _importPTD() async {
    setState(() => _busy = true);
    try {
      final result = await FilePicker.platform.pickFiles(
        type: FileType.custom,
        allowedExtensions: ['ptd'],
        withData: true,
      );
      if (result == null || result.files.isEmpty) return;
      final file = result.files.first;
      final bytes = file.bytes;
      if (bytes == null) {
        _toast('无法读取所选文件');
        return;
      }
      final n = await _api.importPTD(bytes, file.name);
      _toast('已导入 $n 个站点');
    } on MissingPluginException {
      _toast('选文件不可用，请用 Web 端导入');
    } catch (e) {
      _toast(e.toString().replaceFirst('Exception: ', ''));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Widget _actionCard({
    required IconData icon,
    required String title,
    required String desc,
    required String buttonLabel,
    required VoidCallback onTap,
  }) =>
      Padding(
        padding: const EdgeInsets.only(bottom: 14),
        child: GlassCard(
          padding: const EdgeInsets.fromLTRB(18, 16, 18, 16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(children: [
                Icon(icon, size: 22),
                const SizedBox(width: 8),
                Text(title, style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w600)),
              ]),
              const SizedBox(height: 6),
              Text(desc, style: const TextStyle(color: Colors.grey, fontSize: 13)),
              const SizedBox(height: 14),
              Align(
                alignment: Alignment.centerRight,
                child: FilledButton(
                  onPressed: _busy ? null : onTap,
                  child: Text(buttonLabel),
                ),
              ),
            ],
          ),
        ),
      );

  @override
  Widget build(BuildContext context) {
    return GlassBackground(
      child: SafeArea(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(14, 4, 14, 2),
              child: Row(children: [
                const HomeMenuButton(),
                const Text('备份', style: TextStyle(fontSize: 16, fontWeight: FontWeight.w700)),
              ]),
            ),
            if (_busy) const LinearProgressIndicator(minHeight: 2),
            Expanded(
              child: ListView(
                padding: const EdgeInsets.all(16),
                children: [
                  _actionCard(
                    icon: Icons.download_outlined,
                    title: '导出配置',
                    desc: '导出全部站点与下载器配置（含明文密钥，请妥善保管）为 JSON 文件。',
                    buttonLabel: '导出',
                    onTap: _export,
                  ),
                  if (ApiClient().isAdmin) ...[
                    _actionCard(
                      icon: Icons.upload_file,
                      title: '导入（PTPatronus 格式）',
                      desc: '从 PTPatronus 备份 JSON 恢复站点与下载器。重复项自动跳过。',
                      buttonLabel: '选择文件',
                      onTap: () => _import(false),
                    ),
                    _actionCard(
                      icon: Icons.movie_outlined,
                      title: '从 MoviePilot 导入站点',
                      desc: '导入 MoviePilot 站点 JSON 数组，自动创建为 NexusPHP 站点。',
                      buttonLabel: '选择文件',
                      onTap: () => _import(true),
                    ),
                    _actionCard(
                      icon: Icons.extension_outlined,
                      title: '从 PT-Depiler (.ptd) 导入站点',
                      desc: '导入 PT-Depiler 浏览器扩展导出的 .ptd 备份，自动恢复站点 URL/Cookie/Passkey/账号。加密备份需先导出未加密版本。',
                      buttonLabel: '选择文件',
                      onTap: _importPTD,
                    ),
                  ],
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}
