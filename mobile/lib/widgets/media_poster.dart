import 'package:flutter/material.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';

/// 媒体海报：豆瓣 doubanio.com 防盗链 → 走后端 /media/image 反代（带 JWT）；
/// 其它公网 CDN（image.tmdb.org / lain.bgm.tv）直连即可。失败/无图回退占位 emoji。
///
/// 加载中显示 shimmer 骨架（而非裸 spinner），加载完成后淡入，更贴合液态玻璃风格。
class MediaPoster extends StatefulWidget {
  final String url;
  final String kind; // movie | music
  const MediaPoster({super.key, required this.url, this.kind = 'movie'});

  @override
  State<MediaPoster> createState() => _MediaPosterState();
}

class _MediaPosterState extends State<MediaPoster>
    with SingleTickerProviderStateMixin {
  late final AnimationController _fade;

  @override
  void initState() {
    super.initState();
    _fade = AnimationController(
      vsync: this,
      duration: PTMotion.normal,
      value: 0.0,
    );
  }

  @override
  void dispose() {
    _fade.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    if (widget.url.isEmpty) return _fallback();
    final proxied = ApiClient().mediaImageUri(widget.url);
    final headers =
        proxied != null ? ApiClient().authHeaders : const <String, String>{};
    return Image.network(
      (proxied ?? Uri.parse(widget.url)).toString(),
      headers: headers,
      fit: BoxFit.cover,
      loadingBuilder: (c, child, p) {
        if (p == null) {
          // 图片就绪：触发一次性淡入
          if (_fade.status == AnimationStatus.dismissed) _fade.forward();
          return FadeTransition(
            opacity: CurvedAnimation(parent: _fade, curve: PTMotion.fluid),
            child: child,
          );
        }
        // 加载中：shimmer 骨架占位
        return const PTShimmerBlock(height: double.infinity, radius: 0);
      },
      errorBuilder: (c, e, s) => _fallback(),
    );
  }

  Widget _fallback() => Container(
        color: const Color(0x14000000),
        alignment: Alignment.center,
        child: Text(
            widget.kind == 'music' ? '🎵' : '🎬',
            style: const TextStyle(fontSize: 28)),
      );
}