// stream_account.dart —— 流式（NDJSON/SSE）done 帧的清单账面统一口径。
import 'package:flutter/material.dart';
//
// 后端在 freeleech / medalwall / harem / site_search / site 五处 done 帧里带 `list_error`
// （空 = 本轮站点清单没报错）。移动端把它丢掉，就等于把「清单读不到」画成「真的没有」。
// 多个流共用这一个判据，避免同一条取法抄几遍后各自漂移。

/// done 帧里的清单读失败留痕（后端 list_error；空串 = 本轮清单没报错）。
String streamListError(Map<String, dynamic> frame) =>
    frame['list_error'] as String? ?? '';

/// 流式一帧：一批条目 + done 帧的清单账面。
/// 各流原先直接把元素定为「条目本身」，done 帧的账面就没有落脚点，
/// 于是「站点清单读失败」被画成「一条记录都没有」。
class StreamBatch<T> {
  final List<T> items;
  final String listError;
  const StreamBatch({this.items = const [], this.listError = ''});
}

/// 帧映射的唯一实现：batch 帧解析条目，done 帧只带清单账面，其它帧空着过去。
/// api 层内部自己 new ApiClient 注不进去，所以把这段抽成纯函数，让它可被用例证伪。
StreamBatch<T> mapStreamFrame<T>(
  MapEntry<String, Map<String, dynamic>> ev,
  T Function(Map<String, dynamic> json) parse,
) {
  if (ev.key == 'batch' && ev.value['results'] is List) {
    return StreamBatch<T>(
      items: (ev.value['results'] as List)
          .map((e) => parse(e as Map<String, dynamic>))
          .toList(),
    );
  }
  if (ev.key == 'done') {
    return StreamBatch<T>(listError: streamListError(ev.value));
  }
  return StreamBatch<T>();
}

/// 流式帧源：生产用 `ApiClient.streamGet`，测试注入假帧。
/// 没有这个缝，api 层「帧 → 条目 + 账面」的接线就只有编译层证明。
typedef StreamFrames =
    Stream<MapEntry<String, Map<String, dynamic>>> Function(String path);

/// 推荐流 done 帧（`sources/batches/failed`）→ 一句人话；空串 = 本轮不缺源、无失败源。
/// 少跑一个源时结果不是「就这么些」，而是「有个源没跑成」，两者在界面上必须分得开。
String recommendStreamNotice(Map<String, dynamic> frame) {
  final sources = (frame['sources'] as num?)?.toInt();
  final batches = (frame['batches'] as num?)?.toInt() ?? 0;
  final failed = ((frame['failed'] as List?) ?? const []).map((e) => '$e').toList();
  if (sources != null && sources > 0 && batches < sources) {
    final who = failed.isEmpty ? '' : '（失败：${failed.join('、')}）';
    return '只跑了 $batches/$sources 个推荐源$who，结果可能不全';
  }
  if (failed.isNotEmpty) {
    return '${failed.length} 个推荐源抓取失败：${failed.join('、')}，结果可能不全';
  }
  return '';
}

/// 账面该落在哪：一条数据都没到 → 整页错误态（带重试）；已有数据 → 顶部常驻横幅。
/// 两者互斥，绝不能同时给，也不能都不给（那就退回「残缺冒充完整」）。
({String? error, String? banner}) streamNoticeOf({
  required int arrived,
  required String listError,
}) {
  if (listError.isEmpty) return (error: null, banner: null);
  return arrived == 0 ? (error: listError, banner: null) : (error: null, banner: listError);
}

/// 流式账面横幅：四个流式页共用（后宫/勋章墙/考核/推荐），空文案时不占位。
class StreamNoticeBanner extends StatelessWidget {
  const StreamNoticeBanner({super.key, this.text});

  final String? text;

  @override
  Widget build(BuildContext context) {
    final t = text;
    if (t == null || t.isEmpty) return const SizedBox.shrink();
    return Container(
      width: double.infinity,
      margin: const EdgeInsets.fromLTRB(12, 10, 12, 0),
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
      decoration: BoxDecoration(
        color: const Color(0xFFF39C12).withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(8),
      ),
      child: Row(
        children: [
          const Icon(Icons.priority_high, size: 15, color: Color(0xFFF39C12)),
          const SizedBox(width: 6),
          Expanded(
            child: Text(
              t,
              style: const TextStyle(fontSize: 12, height: 1.35),
            ),
          ),
        ],
      ),
    );
  }
}
