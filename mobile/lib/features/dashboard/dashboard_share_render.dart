// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// _DashboardPageState 的总览分享导出扩展：截图导出/保存/分享 + 分享卡渲染全家桶。
// （无 setState 段；从 dashboard_page.dart 拆出，行为不变。）
part of 'package:ptpatronus/features/dashboard/dashboard_page.dart';

/// 导出失败要说什么：截图取不到像素（RepaintBoundary 还没布局完 / 平台不支持）
/// 与本地写盘/分享异常是两种原因，但共同点是**不能静默**——从前这两处都只有一句
/// `'$e'`（英文原文）或直接 return，用户按「保存图片」后弹窗不关、也不吭声，只会反复点。
String shareExportFailure(Object? err) {
  // 这里的失败都来自本地（RepaintBoundary 未布局、写盘、分享通道），不是后端响应，
  // 所以不给 ApiException 分支；英文原文只作细节附在中文主句后面。
  if (err == null) return '截图失败：预览还没渲染完成，请关闭后重新打开导出预览再试';
  return '导出图片失败：$err';
}

/// 分享通道缺失时的说法（与保存失败区分：这里用户能做的是改用保存）。
String shareUnsupported() => '当前平台不支持分享，请使用保存图片';

extension on _DashboardPageState {
  Future<void> _exportImage() async {
    await _prepareShareFavicons();
    if (!mounted) return;
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      builder: (sheetContext) => SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(16, 12, 16, 18),
          child: SizedBox(
            height: MediaQuery.sizeOf(sheetContext).height * 0.86,
            child: Column(
              children: [
                const Text(
                  '导出总仪表盘图片',
                  style: TextStyle(fontWeight: FontWeight.w700),
                ),
                const SizedBox(height: 12),
                Expanded(
                  child: SingleChildScrollView(
                    child: Center(
                      child: SingleChildScrollView(
                        scrollDirection: Axis.horizontal,
                        child: RepaintBoundary(
                          key: _shareKey,
                          child: Material(
                            color: Colors.transparent,
                            child: _shareCard(),
                          ),
                        ),
                      ),
                    ),
                  ),
                ),
                const SizedBox(height: 14),
                Row(
                  children: [
                    Expanded(
                      child: OutlinedButton.icon(
                        onPressed: _saveImage,
                        icon: const Icon(Icons.save_alt),
                        label: const Text('保存图片'),
                      ),
                    ),
                    const SizedBox(width: 10),
                    Expanded(
                      child: FilledButton.icon(
                        onPressed: _shareImage,
                        icon: const Icon(Icons.share),
                        label: const Text('分享图片'),
                      ),
                    ),
                  ],
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  String _dashboardImageFileName() {
    final d = DateTime.now();
    String p(int n) => n.toString().padLeft(2, '0');
    return 'ptpatronus-dashboard-${d.year}${p(d.month)}${p(d.day)}-${p(d.hour)}${p(d.minute)}${p(d.second)}.png';
  }

  Future<Uint8List?> _captureDashboardPng() async {
    final ctx = _shareKey.currentContext;
    final boundary = ctx?.findRenderObject() is RenderRepaintBoundary
        ? ctx!.findRenderObject() as RenderRepaintBoundary
        : null;
    if (boundary == null) return null;
    final img = await boundary.toImage(pixelRatio: 3.0);
    final byteData = await img.toByteData(format: ui.ImageByteFormat.png);
    img.dispose();
    if (byteData == null) return null;
    return byteData.buffer.asUint8List(
      byteData.offsetInBytes,
      byteData.lengthInBytes,
    );
  }

  bool _filePickerSaveWritesBytes() {
    return defaultTargetPlatform == TargetPlatform.android ||
        defaultTargetPlatform == TargetPlatform.iOS ||
        isOhosPlatform; // 鸿蒙走移动端同款 save 通道：bytes 直接落盘
  }

  Future<String> _writeImageToDocuments(Uint8List png, String fileName) async {
    final dir = await getApplicationDocumentsDirectory();
    final file = File('${dir.path}${Platform.pathSeparator}$fileName');
    await file.writeAsBytes(png, flush: true);
    return file.path;
  }

  Future<void> _saveImage() async {
    String? savedPath;
    try {
      final png = await _captureDashboardPng();
      if (png == null) {
        if (mounted) {
          ScaffoldMessenger.of(
            context,
          ).showSnackBar(SnackBar(content: Text(shareExportFailure(null))));
        }
        return;
      }
      final fileName = _dashboardImageFileName();

      try {
        final path = await appFilePicker.saveFile(
          dialogTitle: '保存总仪表盘图片',
          fileName: fileName,
          type: 'custom',
          allowedExtensions: const ['png'],
          bytes: png,
        );
        if (path == null) return;
        savedPath = path;
        if (!_filePickerSaveWritesBytes()) {
          await File(path).writeAsBytes(png, flush: true);
        }
      } on MissingPluginException {
        savedPath = await _writeImageToDocuments(png, fileName);
      } on UnimplementedError {
        savedPath = await _writeImageToDocuments(png, fileName);
      }

      if (!mounted) return;
      Navigator.pop(context);
      _toast('图片已保存：$savedPath');
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text(shareExportFailure(e))));
      }
    }
  }

  Future<void> _shareImage() async {
    try {
      final png = await _captureDashboardPng();
      if (png == null) {
        if (mounted) {
          ScaffoldMessenger.of(
            context,
          ).showSnackBar(SnackBar(content: Text(shareExportFailure(null))));
        }
        return;
      }
      await Share.shareXFiles([
        XFile.fromData(
          png,
          mimeType: 'image/png',
          name: _dashboardImageFileName(),
        ),
      ]);
      if (mounted) {
        Navigator.pop(context);
      }
    } on MissingPluginException {
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text(shareUnsupported())));
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text(shareExportFailure(e))));
      }
    }
  }
}
