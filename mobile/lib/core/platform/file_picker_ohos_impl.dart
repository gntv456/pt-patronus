// file_picker 门面的鸿蒙实现：转发 file_picker_ohos 10.3.8
// （CPF-Flutter 适配版，MethodChannel 桥接鸿蒙原生文件选择器）。
//
// 本文件平时游离于编译图之外：file_picker_ohos 是鸿蒙专用依赖
// （pubspec_overrides.ohos.yaml 引入），官方 SDK 的 pub get 不解析它，
// 直接 import 会报 uri_does_not_exist。鸿蒙构建时 scripts/build_ohos.ps1
// 会把本文件临时改名为 file_picker_default.dart（原默认实现暂存为
// .default.bak），使门面（file_picker.dart）无需改动即切换到本实现，
// 构建后复原。详见 docs/鸿蒙端完善.md「file_picker 特殊处理」。
//
// 注意：本文件与 file_picker_default.dart 是「同名互斥」关系，二者的
// 类名保持一致（AppFilePicker 子类都叫 PlatformFilePickerImpl），
// 切换只靠文件名替换，不改任何 import。
import 'dart:typed_data';

import 'package:file_picker_ohos/file_picker_ohos.dart' as fp;

import 'file_picker.dart';

/// 鸿蒙实现（与 file_picker_default.dart 的类同名，见文件头说明）。
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
