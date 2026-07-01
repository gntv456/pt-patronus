// 把 PTPatronus 吉祥物渲染成 1024² PNG，作为 flutter_launcher_icons 的源（image_path）。
// 运行：flutter test tool/render_icon.dart   产物：mobile/assets/icon_source.png
//
// 不走 flutter_svg：logo-mark.svg 用了 feTurbulence 等滤镜，flutter_svg 不支持；
// 这里复用 lib/widgets/pt_logo.dart 的 paintPTLogo（CustomPainter），保证图标与 in-app logo 一致。
// withPT=false：flutter test 默认 Ahem 字体会把字母渲染成方块，且小尺寸下 PT 字不可读，
// 故启动器图标省略 PT 字标（盾牌 + 笑脸 + 彩色尖帽已足以承载品牌）。
import 'dart:io';
import 'dart:ui' as ui;

import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/widgets/pt_logo.dart';

void main() {
  test('Render launcher icon source (1024² PNG)', () async {
    TestWidgetsFlutterBinding.ensureInitialized();

    const dim = 1024;
    final recorder = ui.PictureRecorder();
    final canvas = Canvas(recorder);
    paintPTLogo(
      canvas,
      const Size(1024.0, 1024.0),
      detailed: true,
      background: true,
      withPT: false,
    );
    final picture = recorder.endRecording();
    final image = await picture.toImage(dim, dim);
    final byteData = await image.toByteData(format: ui.ImageByteFormat.png);
    expect(byteData, isNotNull, reason: 'PNG 编码失败');
    final out = File('assets/icon_source.png');
    await out.create(recursive: true);
    await out.writeAsBytes(byteData!.buffer.asUint8List());
    // ignore: avoid_print
    print('✅ 启动器图标源已生成：${out.absolute.path}（${byteData.lengthInBytes} bytes）');
  });
}
