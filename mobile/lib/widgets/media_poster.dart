import 'package:flutter/material.dart';
import 'package:ptpatronus/core/api_client.dart';

/// 媒体海报：豆瓣 doubanio.com 防盗链 → 走后端 /media/image 反代（带 JWT）；
/// 其它公网 CDN（image.tmdb.org / lain.bgm.tv）直连即可。失败/无图回退占位 emoji。
class MediaPoster extends StatelessWidget {
  final String url;
  final String kind; // movie | music
  const MediaPoster({super.key, required this.url, this.kind = 'movie'});

  @override
  Widget build(BuildContext context) {
    if (url.isEmpty) return _fallback();
    final proxied = ApiClient().mediaImageUri(url); // 仅 doubanio 走反代，返回 null=直连
    final headers = proxied != null ? ApiClient().authHeaders : const <String, String>{};
    return Image.network(
      (proxied ?? Uri.parse(url)).toString(),
      headers: headers,
      fit: BoxFit.cover,
      loadingBuilder: (c, child, p) => p == null
          ? child
          : const Center(child: SizedBox(width: 20, height: 20, child: CircularProgressIndicator(strokeWidth: 2))),
      errorBuilder: (c, e, s) => _fallback(),
    );
  }

  Widget _fallback() => Container(
        color: const Color(0x14000000),
        alignment: Alignment.center,
        child: Text(kind == 'music' ? '🎵' : '🎬', style: const TextStyle(fontSize: 28)),
      );
}
