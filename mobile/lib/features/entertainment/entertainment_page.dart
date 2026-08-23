import 'package:flutter/material.dart';
import 'package:ptpatronus/core/responsive.dart';
import 'package:ptpatronus/api/entertainment.dart';
import 'package:ptpatronus/api/site.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_filter_sheet.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';

import 'jgg_game_view.dart';
import 'cspt_scratch_view.dart';
import 'magic_scratch_view.dart';
import 'card_synth_view.dart';
import 'longpt_wheel_view.dart';
import 'longpt_daily_view.dart';
import 'longpt_medal_view.dart';
import 'magic_farm_view.dart';
import 'novahd_scratch_view.dart';
import 'novahd_lucky_view.dart';
import 'spinner_game_view.dart';
import 'bet_game_view.dart';
import 'blackjack_game_view.dart';
import 'panda_flip_view.dart';
import 'bigsmall_view.dart';
import 'event_bet_view.dart';

// 娱乐页：选站点 → 选玩法 → 该玩法的富 UI（按 gameKey 分发）。
class EntertainmentPage extends StatefulWidget {
  const EntertainmentPage({super.key});
  @override
  State<EntertainmentPage> createState() => _EntertainmentPageState();
}

class _EntertainmentPageState extends State<EntertainmentPage> {
  final _entApi = EntertainmentApi();
  List<Site> _sites = [];
  int? _siteId;
  List<Game> _games = [];
  String? _gameId;
  bool _loadingGames = false;
  // 通用兜底玩法（无富 UI 的游戏）
  final _playing = <String, bool>{};
  final _results = <String, PlayResult>{};

  @override
  void initState() {
    super.initState();
    _loadSites();
  }

  Future<void> _loadSites() async {
    try {
      _sites = await _entApi.sites();
      if (_sites.isNotEmpty) {
        if (!mounted) return;
        setState(() => _siteId = _sites.first.id);
        _loadGames();
      }
    } catch (_) {
      _toast('加载站点失败');
    }
  }

  Future<void> _loadGames() async {
    if (_siteId == null) return;
    setState(() {
      _loadingGames = true;
      _games = [];
      _results.clear();
    });
    try {
      _games = await _entApi.games(_siteId!);
      _gameId = _games.isNotEmpty ? _games.first.id : null;
    } catch (_) {
      _toast('加载游戏失败');
    } finally {
      if (mounted) setState(() => _loadingGames = _loadingGames = false);
    }
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  @override
  Widget build(BuildContext context) {
    return PTPageScaffold(
      title: '娱乐',
      siteFilter: _sites.isEmpty
          ? null
          : PTSiteFilterConfig(
              single: true, // 娱乐页按站点切换加载游戏，单选业务
              selected: _siteId == null ? const <int>[] : [_siteId!],
              onChanged: (v) {
                final id = v.isEmpty ? null : v.first;
                if (id != _siteId) {
                  setState(() => _siteId = id);
                  _loadGames();
                }
              },
              options: _sites
                  .map(
                    (s) => PTFilterOption<Object?>(value: s.id, label: s.name, count: s.gameCount),
                  )
                  .toList(),
            ),
      filterDimensions: _games.length > 1 ? _gameDimension() : const [],
      body: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          if (_loadingGames) const LinearProgressIndicator(minHeight: 2),
          Expanded(child: _bodyWrapped()),
        ],
      ),
    );
  }

  // 玩法维度（收进通用筛选按钮；仅当该站点 >1 种玩法时出现）。首个玩法为默认（不计徽标）。
  List<PTFilterDimension> _gameDimension() {
    return [
      PTFilterDimension.choice(
        label: '玩法',
        selected: _gameId,
        isDefault: _gameId == (_games.isNotEmpty ? _games.first.id : null),
        onChanged: (v) => setState(() => _gameId = v as String?),
        options: _games
            .map((g) => PTFilterOption<Object?>(value: g.id, label: g.name))
            .toList(),
      ),
    ];
  }

  // 宽屏把游戏面板居中并限宽，避免 ListView 全宽在桌面端被拉成超宽长条（控件稀疏拉伸）。
  // 窄屏原样全宽。一处包裹惠及全部 16 个 game view。
  Widget _bodyWrapped() {
    final body = _body();
    if (!isWideScreen(context)) return body;
    return Center(
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 760),
        child: body,
      ),
    );
  }

  Widget _body() {
    if (_games.isEmpty) {
      return PTEmptyState(
        icon: Icons.sports_esports_outlined,
        text: _siteId == null ? '请先在「站点」添加站点' : '该站点暂无游戏',
        description: _loadingGames ? '加载中…' : '不同站点开放不同玩法，切换站点试试',
      );
    }
    if (_siteId == null) return const SizedBox.shrink();
    const spinnerIds = [
      'cbg_wheel',
      'cbg_slot',
      'ub_wheel',
      'luckydraw_wheel',
      'march_wheel',
      'tangpt_lottery',
      'hxpt_redpacket',
    ];
    if (_gameId != null && spinnerIds.contains(_gameId)) {
      final g = _games.firstWhere(
        (x) => x.id == _gameId,
        orElse: () => Game(id: _gameId!, name: ''),
      );
      return SpinnerGameView(siteId: _siteId!, game: _gameId!, name: g.name);
    }
    if (_gameId == 'panda_flip') {
      return PandaFlipView(siteId: _siteId!);
    }
    // 猜大小（好学 hxpt_bigsmall + 熊猫 panda_bigsmall）走富 UI
    if (_gameId == 'hxpt_bigsmall' || _gameId == 'panda_bigsmall') {
      final g = _games.firstWhere(
        (x) => x.id == _gameId,
        orElse: () => Game(id: _gameId!, name: '猜大小'),
      );
      return BigSmallView(siteId: _siteId!, game: _gameId!, name: g.name);
    }
    if (_gameId == 'ourbits_bet') {
      return EventBetView(siteId: _siteId!);
    }
    const betIds = ['cbg_sicbo', 'crab_unionlotto', 'ubits_redpacket'];
    if (_gameId != null && betIds.contains(_gameId)) {
      final g = _games.firstWhere(
        (x) => x.id == _gameId,
        orElse: () => Game(id: _gameId!, name: ''),
      );
      return BetGameView(siteId: _siteId!, game: _gameId!, name: g.name);
    }
    const blackjackIds = ['audiences_blackjack'];
    if (_gameId != null && blackjackIds.contains(_gameId)) {
      final g = _games.firstWhere(
        (x) => x.id == _gameId,
        orElse: () => Game(id: _gameId!, name: ''),
      );
      return BlackjackGameView(siteId: _siteId!, game: _gameId!, name: g.name);
    }
    switch (_gameId) {
      case 'jgg':
        return JggGameView(siteId: _siteId!);
      case 'cspt_scratch':
        return CsptScratchView(siteId: _siteId!);
      case 'magic_scratch':
        return MagicScratchView(siteId: _siteId!);
      case 'medal_synthesis':
        return CardSynthView(siteId: _siteId!);
      case 'longpt_wheel':
        return LongPTWheelView(siteId: _siteId!);
      case 'longpt_daily':
        return LongPTDailyView(siteId: _siteId!);
      case 'longpt_medal':
        return LongPTMedalView(siteId: _siteId!);
      case 'magic_farm':
        return MagicFarmView(siteId: _siteId!);
      case 'novahd_scratch':
        return NovaHDScratchView(siteId: _siteId!);
      case 'novahd_lucky':
        return NovaHDLuckyView(siteId: _siteId!);
      default:
        return _genericList();
    }
  }

  // 通用兜底：游戏卡片 + 玩一次
  Widget _genericList() {
    final g = _games.where((x) => x.id == _gameId).toList();
    return ListView(
      padding: const EdgeInsets.fromLTRB(12, 4, 12, 24),
      children: g.map((x) => _gameCard(x)).toList(),
    );
  }

  Widget _gameCard(Game g) {
    final r = _results[g.id];
    final busy = _playing[g.id] ?? false;
    return Padding(
      padding: const EdgeInsets.only(bottom: 10),
      child: GlassCard(
        padding: const EdgeInsets.all(14),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                const Icon(
                  Icons.casino_outlined,
                  color: PTColors.primary,
                  size: 20,
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(
                    g.name,
                    style: const TextStyle(
                      fontWeight: FontWeight.w700,
                      fontSize: 16,
                    ),
                  ),
                ),
                FilledButton(
                  onPressed: busy ? null : () => _play(g),
                  child: busy
                      ? const SizedBox(
                          width: 16,
                          height: 16,
                          child: CircularProgressIndicator(strokeWidth: 2),
                        )
                      : const Text('玩一次'),
                ),
              ],
            ),
            if (r != null) ...[const SizedBox(height: 10), _resultView(r)],
          ],
        ),
      ),
    );
  }

  Future<void> _play(Game g) async {
    if (_siteId == null) return;
    setState(() => _playing[g.id] = true);
    try {
      final res = await _entApi.play(_siteId!, g.id);
      if (!mounted) return;
      setState(() => _results[g.id] = res);
      _toast(
        res.success
            ? (res.prize.isNotEmpty ? '${g.name}：${res.prize}' : '${g.name} 完成')
            : (res.message.isNotEmpty ? res.message : '失败'),
      );
    } catch (e) {
      _toast('请求失败：$e');
    } finally {
      if (mounted) setState(() => _playing[g.id] = false);
    }
  }

  Widget _resultView(PlayResult r) {
    final win = r.success;
    return Container(
      padding: const EdgeInsets.all(10),
      decoration: BoxDecoration(
        color: win
            ? PTColors.success.withValues(alpha: 0.12)
            : Colors.redAccent.withValues(alpha: 0.10),
        borderRadius: BorderRadius.circular(10),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Wrap(
            spacing: 8,
            runSpacing: 4,
            crossAxisAlignment: WrapCrossAlignment.center,
            children: [
              Text(
                win ? '✓ 成功' : '✗ 失败',
                style: TextStyle(
                  fontWeight: FontWeight.w700,
                  color: win ? PTColors.success : Colors.redAccent,
                  fontSize: 13,
                ),
              ),
              if (r.prize.isNotEmpty)
                Text(
                  r.prize,
                  style: const TextStyle(
                    color: PTColors.primary,
                    fontWeight: FontWeight.w600,
                    fontSize: 13,
                  ),
                ),
              if (r.amount > 0)
                Text(
                  '×${r.amount.toStringAsFixed(r.amount % 1 == 0 ? 0 : 1)}',
                  style: const TextStyle(
                    color: Colors.orange,
                    fontWeight: FontWeight.w600,
                    fontSize: 13,
                  ),
                ),
            ],
          ),
          if (r.message.isNotEmpty) ...[
            const SizedBox(height: 4),
            Text(
              r.message,
              style: TextStyle(color: context.dimText, fontSize: 12),
            ),
          ],
        ],
      ),
    );
  }
}
