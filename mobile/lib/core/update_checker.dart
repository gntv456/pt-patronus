import 'package:dio/dio.dart';
import 'package:flutter/foundation.dart' show defaultTargetPlatform, TargetPlatform, kIsWeb;
import 'package:flutter/material.dart';
import 'package:url_launcher/url_launcher.dart';
import 'package:ptpatronus/api/system.dart';
import 'apk_downloader.dart';

/// 当前客户端对应的更新清单 platform key：docker(android 兜底) / android / ios / macos / windows。
/// 桌面端（windows/macos）与 iOS 走外链兜底（无应用内安装），android 走应用内 OTA。
String currentUpdatePlatform() {
  if (kIsWeb) return 'android';
  switch (defaultTargetPlatform) {
    case TargetPlatform.windows:
      return 'windows';
    case TargetPlatform.iOS:
      return 'ios';
    case TargetPlatform.macOS:
      return 'macos';
    default:
      return 'android';
  }
}

/// 检查在线更新。有新版本：
///   - Android 且 downloadUrl 是 APK 直链 → 应用内 OTA（下载 + sha256 校验 + 系统安装，pt_mate 式）
///   - 否则（桌面 / 非直链）→ 外链浏览器下载（兜底）
/// silent=true 时不提示「已是最新」（启动场景）；设置页手动检查传 silent=false。
Future<void> checkAndPromptUpdate(BuildContext context, {bool silent = false}) async {
  UpdateCheckResult r;
  try {
    r = await SystemApi.checkUpdate(platform: currentUpdatePlatform());
  } catch (_) {
    if (!silent && context.mounted) {
      ScaffoldMessenger.of(context)
          .showSnackBar(const SnackBar(content: Text('检查更新失败，请稍后重试')));
    }
    return;
  }
  if (!context.mounted) return;
  if (!r.hasUpdate) {
    if (!silent) {
      ScaffoldMessenger.of(context)
          .showSnackBar(const SnackBar(content: Text('已是最新版本')));
    }
    return;
  }
  if (!context.mounted) return;
  if (_canInAppOta(r)) {
    _showOtaDialog(context, r);
  } else {
    _showExternalDialog(context, r);
  }
}

bool _canInAppOta(UpdateCheckResult r) {
  if (!supportsInAppOta()) return false;
  // APK 直链 + 提供 sha256（完整性校验不可省）+ https 源，三者俱全才应用内 OTA；
  // 否则走外链兜底（缺 sha256 则无法防篡改，不应静默下载安装）。
  final url = r.downloadUrl.toLowerCase();
  return url.endsWith('.apk') && url.startsWith('https://') && r.sha256.isNotEmpty;
}

/// 应用内 OTA：下载进度 + sha256 校验 + 系统安装（pt_mate 式，补 sha256）。
void _showOtaDialog(BuildContext context, UpdateCheckResult r) {
  showDialog<void>(
    context: context,
    barrierDismissible: false,
    builder: (_) => _OtaUpdateDialog(result: r),
  );
}

/// 外链兜底对话框（downloadUrl 非直链 或 桌面/Web 平台）。
void _showExternalDialog(BuildContext context, UpdateCheckResult r) {
  showDialog<void>(
    context: context,
    barrierDismissible: !r.isForced,
    builder: (ctx) => AlertDialog(
      title: Text('发现新版本 v${r.latestVersion}'),
      content: SingleChildScrollView(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          mainAxisSize: MainAxisSize.min,
          children: [
            Text('当前 v${r.currentVersion} → 最新 v${r.latestVersion}',
                style: const TextStyle(color: Colors.grey, fontSize: 13)),
            if (r.releaseName.isNotEmpty)
              Padding(
                padding: const EdgeInsets.only(top: 8),
                child: Text(r.releaseName,
                    style: const TextStyle(fontWeight: FontWeight.w600)),
              ),
            if (r.releaseNotes.isNotEmpty)
              Padding(
                padding: const EdgeInsets.only(top: 8),
                child: Text(r.releaseNotes,
                    style: const TextStyle(fontSize: 13, height: 1.5)),
              ),
            if (r.isBelowMin)
              const Padding(
                padding: EdgeInsets.only(top: 8),
                child: Text('当前版本过低，建议立即升级',
                    style: TextStyle(color: Colors.red, fontWeight: FontWeight.w600)),
              ),
            const SizedBox(height: 8),
            Text(
              defaultTargetPlatform == TargetPlatform.iOS
                  ? 'iOS 暂不支持应用内安装，请通过 TestFlight 或官方分发渠道获取更新。'
                  : '当前平台不支持应用内安装，请在浏览器下载安装包后手动安装。',
              style: const TextStyle(fontSize: 12, color: Colors.grey)),
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: r.isForced ? null : () => Navigator.pop(ctx),
          child: const Text('稍后再说'),
        ),
        FilledButton(
          onPressed: () {
            Navigator.pop(ctx);
            if (r.downloadUrl.isNotEmpty) {
              launchUrl(Uri.parse(r.downloadUrl),
                  mode: LaunchMode.externalApplication);
            }
          },
          child: const Text('浏览器下载'),
        ),
      ],
    ),
  );
}

/// 应用内 OTA 下载对话框：进度条 + 取消 + 错误重试。
class _OtaUpdateDialog extends StatefulWidget {
  final UpdateCheckResult result;
  const _OtaUpdateDialog({required this.result});
  @override
  State<_OtaUpdateDialog> createState() => _OtaUpdateDialogState();
}

class _OtaUpdateDialogState extends State<_OtaUpdateDialog> {
  final CancelToken _cancelToken = CancelToken();
  String _stage = '正在下载更新包...';
  double _progress = 0; // 0..1；传 null 给 LinearProgressIndicator 表示未知大小
  bool _busy = false;
  bool _started = false; // 用户显式点「立即下载」才开始（不进对话框即自动下载有违用户知情）
  String? _error;

  @override
  void initState() {
    super.initState();
  }

  void _start() {
    if (_busy) return;
    setState(() {
      _started = true;
      _busy = true;
      _error = null;
    });
    _run();
  }

  Future<void> _run() async {
    try {
      final path = await downloadApk(
        url: widget.result.downloadUrl,
        expectedSha256:
            widget.result.sha256.isEmpty ? null : widget.result.sha256,
        onProgress: (received, total) {
          if (!mounted) return;
          if (total > 0) {
            setState(() => _progress = received / total);
          }
        },
        cancelToken: _cancelToken,
      );
      if (!mounted) return;
      // downloadApk 内部已做 sha256 校验（不符即抛），到这步即校验通过
      setState(() {
        _stage = '下载完成，正在打开安装界面...';
        _busy = false;
      });
      await installApk(path);
      // 系统安装界面已弹出，关闭本对话框（用户在系统界面完成安装）
      if (mounted) Navigator.of(context).pop();
    } on DioException catch (e) {
      if (mounted) {
        setState(() {
          _busy = false;
          _error = CancelToken.isCancel(e)
              ? '下载已取消'
              : '网络错误：${e.message ?? e.type.name}';
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _busy = false;
          _error = '$e';
        });
      }
    }
  }

  void _cancel() {
    if (!_cancelToken.isCancelled) _cancelToken.cancel('user');
    if (mounted) Navigator.of(context).pop();
  }

  void _retry() {
    setState(() {
      _error = null;
      _busy = true;
      _progress = 0;
      _stage = '正在下载更新包...';
    });
    _run();
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: Text('更新到 v${widget.result.latestVersion}'),
      content: SizedBox(
        width: 300,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            if (widget.result.releaseNotes.isNotEmpty)
              Padding(
                padding: const EdgeInsets.only(bottom: 10),
                child: Text(widget.result.releaseNotes,
                    style: const TextStyle(fontSize: 13, height: 1.5)),
              ),
            if (!_started)
              const Text('点击「立即下载」开始下载，完成后将自动校验完整性并打开安装界面。',
                  style: TextStyle(fontSize: 12, color: Colors.grey))
            else if (_error != null)
              Text(_error!, style: const TextStyle(color: Colors.red, fontSize: 13))
            else ...[
              Text(_stage, style: const TextStyle(fontSize: 13)),
              const SizedBox(height: 12),
              LinearProgressIndicator(value: _progress > 0 ? _progress : null),
              if (_progress > 0)
                Padding(
                  padding: const EdgeInsets.only(top: 6),
                  child: Text('${(_progress * 100).toStringAsFixed(0)}%',
                      style: const TextStyle(fontSize: 12, color: Colors.grey)),
                ),
            ],
          ],
        ),
      ),
      actions: [
        if (!_started) ...[
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: Text(widget.result.isForced ? '稍后' : '以后再说'),
          ),
          FilledButton(onPressed: _start, child: const Text('立即下载')),
        ] else if (_error != null) ...[
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: const Text('关闭'),
          ),
          FilledButton(onPressed: _retry, child: const Text('重试')),
        ] else if (_busy) ...[
          TextButton(onPressed: _cancel, child: const Text('取消')),
        ],
      ],
    );
  }
}
