// 文件选择门面：业务代码统一 import 本文件，用 appFilePicker.pickFiles /
// saveFile，不直接依赖具体平台的 file_picker 包。
//
// 为什么需要门面：鸿蒙的 file_picker 适配版（CPF-Flutter/fluttertpc_file_picker
// br_v10.3.8_ohos）把包名改成了 file_picker_ohos 且硬依赖 web ^1.0.0，
// 无法以 dependency_overrides 替换 pub.dev 的 file_picker（其 8.x ohos
// 分支锁 web ^0.5.1，与其它鸿蒙插件的 web ^1.0.0 冲突）。两包的类型
// （FilePickerResult/PlatformFile/FileType）分属不同 package 前缀、互不
// 兼容，故本层定义中立的 PickedFile 做桥接。
//
// 实现切换（由 scripts/build_ohos.ps1 完成，零 import 改动）：
// 默认 file_picker_default.dart 转发 pub.dev file_picker 8.x；鸿蒙构建时
// 脚本用 file_picker_ohos_impl.dart 的内容临时替换该文件（实现类同名
// PlatformFilePickerImpl），构建后复原。之所以不用
// `if (dart.library.ohos)` 条件导入：本仓库 3.41 ohos fork 的 sky_engine
// 未导出 dart:ohos library（条件恒假），fork 独有的 dart:io
// Platform.isOhos 又无法进条件导入表达式。
// 详见 docs/鸿蒙端完善.md「file_picker 特殊处理」。
import 'dart:typed_data';

import 'file_picker_default.dart';

/// 中立的「已选文件」：path（本地路径，withData=false 场景）、name、
/// bytes（withData=true 时的内存字节）。
class PickedFile {
  final String? path;
  final String name;
  final Uint8List? bytes;

  const PickedFile({this.path, required this.name, this.bytes});
}

abstract class AppFilePicker {
  /// 选文件；null=用户取消。[allowedExtensions] 仅 type=='custom' 时生效。
  /// [type] 取 'any'（默认）/'custom'/'image'/'media'。
  Future<List<PickedFile>?> pickFiles({
    String? dialogTitle,
    bool multiple = false,
    bool withData = false,
    String? type,
    List<String>? allowedExtensions,
  });

  /// 保存文件对话框；null=用户取消。移动端需传 [bytes]（直接落盘），
  /// 桌面端返回路径由调用方写文件。
  Future<String?> saveFile({
    String? dialogTitle,
    required String fileName,
    String? type,
    List<String>? allowedExtensions,
    Uint8List? bytes,
  });
}

/// 全局单例：实现类见 file_picker_default.dart（切换机制见文件头注释）。
final appFilePicker = PlatformFilePickerImpl();
