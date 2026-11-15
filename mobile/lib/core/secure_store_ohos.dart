// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'dart:convert';
import 'dart:math';
import 'dart:typed_data';

import 'package:pointycastle/export.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// ohos 平台的 token 密封持久化（降级实现）。
///
/// 为什么存在：登录态已迁 flutter_secure_storage（收权加固），但该插件没有
/// ohos 实现——此前 ohos 端 setToken/loadToken 的异常全被 catch 吞掉，token
/// 只在内存，每次杀进程重启都要重新登录（docs/鸿蒙端完善.md 真机验证清单
/// 「登录 → token 持久化」一直无法勾选）。
///
/// 方案：AES-256-GCM 加密后存 SharedPreferences。KEK（设备密钥）首次随机生成、
/// 同存 SharedPreferences——「设备密钥档」语义（与扩展 secrets.js 同口径）：
/// 防明文直接提取/误拷贝，不防 root 全盘读取（客户端无 TEE 时本地加密的物理
/// 上限）。比不可用强：ohos 用户从「每次重启必重登」变成「token 密封持久化」。
/// pointycastle 是纯 Dart 实现、无平台通道——官方 SDK 与 ohos fork SDK 都能跑。
class OhosSecureStore {
  /// KEK 的 SharedPreferences 键（32 字节 hex）。
  static const _kKEK = 'ptp_ohos_kek';

  /// 密封值的键前缀：_prefix + 业务 key。
  static const _prefix = 'ptp_ohos_seal_';

  /// 密封格式版本（前缀段；换算法/KEK 时递增，读到旧版按未存处理）。
  static const _version = 'v1';

  static Future<Uint8List> _kek() async {
    final p = await SharedPreferences.getInstance();
    final cur = p.getString(_kKEK);
    if (cur != null && cur.length == 64) {
      final k = _hexToBytes(cur);
      if (k != null) return k;
    }
    final rnd = Random.secure();
    final key =
        Uint8List.fromList(List<int>.generate(32, (_) => rnd.nextInt(256)));
    await p.setString(_kKEK, _bytesToHex(key));
    return key;
  }

  /// 写入并密封；value 为空串等价删除（与调用方「登出删 token」语义对齐）。
  static Future<void> write(String key, String value) async {
    final p = await SharedPreferences.getInstance();
    if (value.isEmpty) {
      await p.remove(_prefix + key);
      return;
    }
    final rnd = Random.secure();
    final iv =
        Uint8List.fromList(List<int>.generate(12, (_) => rnd.nextInt(256)));
    final cipher = GCMBlockCipher(AESEngine())
      ..init(true, ParametersWithIV(KeyParameter(await _kek()), iv));
    final plain = Uint8List.fromList(utf8.encode(value));
    final ct = cipher.process(plain);
    await p.setString(
        _prefix + key, '$_version.${_bytesToB64(iv)}.${_bytesToB64(ct)}');
  }

  /// 读取并解封。密文损坏/KEK 已换/格式不认识 → 返回 null（按未存处理，
  /// 调用方走重新登录），绝不抛异常打断启动链路。
  static Future<String?> read(String key) async {
    final p = await SharedPreferences.getInstance();
    final raw = p.getString(_prefix + key);
    if (raw == null || !raw.startsWith('$_version.')) return null;
    final parts = raw.split('.');
    if (parts.length != 3) return null;
    try {
      final iv = _b64ToBytes(parts[1]);
      final ct = _b64ToBytes(parts[2]);
      if (iv == null || ct == null) return null;
      final cipher = GCMBlockCipher(AESEngine())
        ..init(false, ParametersWithIV(KeyParameter(await _kek()), iv));
      return utf8.decode(cipher.process(ct));
    } catch (_) {
      return null;
    }
  }

  static Future<void> delete(String key) async {
    final p = await SharedPreferences.getInstance();
    await p.remove(_prefix + key);
  }

  // —— 编码工具（base64/std + hex；不引额外依赖）——

  static String _bytesToHex(Uint8List b) =>
      b.map((x) => x.toRadixString(16).padLeft(2, '0')).join();

  static Uint8List? _hexToBytes(String s) {
    if (s.length.isOdd) return null;
    try {
      return Uint8List.fromList(List<int>.generate(s.length ~/ 2,
          (i) => int.parse(s.substring(i * 2, i * 2 + 2), radix: 16)));
    } catch (_) {
      return null;
    }
  }

  static String _bytesToB64(Uint8List b) => base64.encode(b);

  static Uint8List? _b64ToBytes(String s) {
    try {
      return Uint8List.fromList(base64.decode(s));
    } catch (_) {
      return null;
    }
  }
}
