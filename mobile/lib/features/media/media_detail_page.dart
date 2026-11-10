// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter/material.dart';

import 'package:ptpatronus/api/media.dart';

import 'package:ptpatronus/api/subscribe.dart';

import 'package:ptpatronus/core/api_client.dart';

import 'package:ptpatronus/theme/glass.dart';

import 'package:ptpatronus/widgets/media_poster.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';

import '../search/search_page.dart';

part 'media_detail_panes.dart';
part 'media_detail_recs.dart';
part 'media_detail_widgets.dart';

// 媒体详情页（对标 MoviePilot / Web MediaDetail）：点海报卡片进入，展示

// 背景图/海报/简介/演职/推荐，并提供「搜索资源」一键跳种子搜索（按标题在站点找片）。

// 入参仅 source + id，富字段由 /media/detail 现取（搜索结果只有基础字段）。

class MediaDetailPage extends StatefulWidget {

  final String source;

  final String id;

  const MediaDetailPage({super.key, required this.source, required this.id});

  @override

  State<MediaDetailPage> createState() => _MediaDetailPageState();

}

class _MediaDetailPageState extends State<MediaDetailPage> {

  @override
  Widget build(BuildContext context) => _buildScaffold(context);


  MediaInfo? _detail;

  bool _loading = true;

  bool _subscribing = false;

  String? _error;

  @override

  void initState() {

    super.initState();

    _load();

  }

  Future<void> _load() async {

    setState(() {

      _loading = true;

      _error = null;

    });

    try {

      _detail = await MediaApi().detail(widget.source, widget.id);

    } catch (e) {

      _error = '$e';

    } finally {

      if (mounted) setState(() => _loading = false);

    }

  }

  // 一键用标题跨站搜种子（跳搜索页，预填关键词）。

  // 订阅：创建后 cron 自动搜索+下载（命中推送到默认下载器 + 通知）。同源同 id 幂等。

  Future<void> _subscribe() async {

    final d = _detail;

    if (d == null) return;

    setState(() => _subscribing = true);

    try {

      await SubscribeApi().create(

        source: d.source,

        sourceId: d.sourceId,

        title: d.title,

        year: d.year,

        type: d.type,

        category: d.type == 'tv' ? 'tv' : 'movie',

        posterUrl: d.posterUrl,

      );

      _toast('已订阅，将自动搜索并下载到默认下载器');

    } catch (e) {

      _toast('$e');

    } finally {

      if (mounted) setState(() => _subscribing = false);

    }

  }

  // 点推荐项 → 换源/id 重载本页。

  Widget get _errorView => Center(

        child: Column(

          mainAxisSize: MainAxisSize.min,

          children: [

            Text('⚠ $_error', textAlign: TextAlign.center,

                style: const TextStyle(color: Colors.redAccent)),

            const SizedBox(height: 12),

            FilledButton(onPressed: _load, child: const Text('重试')),

          ],

        ),

      );

  // hero：背景图 + 海报 + 标题/评分/类型/来源彩签。海报叠在背景图下沿。

}
