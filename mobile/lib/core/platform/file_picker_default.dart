// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// file_picker 门面的默认实现（官方 SDK 全平台）：转发 pub.dev file_picker 8.x。
//
// 鸿蒙构建时 scripts/build_ohos.ps1 用 file_picker_ohos_impl.dart 的内容
// 临时替换本文件（原内容暂存为 .bak，构建后复原），门面（file_picker.dart）
// 与所有业务调用点的 import 保持不变——两个实现文件里的实现类同名
// （PlatformFilePickerImpl），切换零代码改动。
// 详见 file_picker.dart 文件头与 docs/鸿蒙端完善.md。
import 'dart:typed_data';

import 'package:file_picker/file_picker.dart' as fp;

import 'file_picker.dart';

/// 默认实现（与 file_picker_ohos_impl.dart 的类同名，见文件头说明）。
class PlatformFilePickerImpl extends AppFilePicker {
  fp.FileType _type(String? type) {
    switch (type) {
      case 'custom':
        return fp.FileType.custom;
      case 'image':
        return fp.FileType.image;
      case 'media':
        return fp.FileType.media;
      default:
        return fp.FileType.any;
    }
  }

  @override
  Future<List<PickedFile>?> pickFiles({
    String? dialogTitle,
    bool multiple = false,
    bool withData = false,
    String? type,
    List<String>? allowedExtensions,
  }) async {
    final result = await fp.FilePicker.platform.pickFiles(
      dialogTitle: dialogTitle,
      type: _type(type),
      allowedExtensions: allowedExtensions,
      allowMultiple: multiple,
      withData: withData,
    );
    if (result == null) return null;
    return [
      for (final f in result.files)
        PickedFile(path: f.path, name: f.name, bytes: f.bytes),
    ];
  }

  @override
  Future<String?> saveFile({
    String? dialogTitle,
    required String fileName,
    String? type,
    List<String>? allowedExtensions,
    Uint8List? bytes,
  }) {
    return fp.FilePicker.platform.saveFile(
      dialogTitle: dialogTitle,
      fileName: fileName,
      type: _type(type),
      allowedExtensions: allowedExtensions,
      bytes: bytes,
    );
  }
}
