import 'dart:async';

import 'package:flutter/material.dart';
import 'package:ptpatronus/api/trakt.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';
import 'package:url_launcher/url_launcher.dart';

/// Trakt 页（对齐 Web Trakt.vue）：配置 + 设备授权 + WatchList→订阅 同步。
class TraktPage extends StatefulWidget {
  const TraktPage({super.key});
  @override
  State<TraktPage> createState() => _TraktPageState();
}

class _TraktPageState extends State<TraktPage> {
  final _api = TraktApi();
  TraktConfig? _cfg;
  bool _loading = false;
  bool _busy = false; // 授权/同步中
  TraktDeviceCode? _device;
  Timer? _poll;

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _poll?.cancel();
    super.dispose();
  }

  void _toast(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
  }

  Future<void> _load() async {
    setState(() => _loading = true);
    try {
      final cfg = await _api.getConfig();
      if (!mounted) return;
      setState(() {
        _cfg = cfg;
        if (cfg.pendingUserCode.isNotEmpty && _device == null) {
          _device = TraktDeviceCode(
            userCode: cfg.pendingUserCode,
            verificationUrl: cfg.pendingVerificationUrl,
            expiresIn: 600,
            interval: 5,
            deviceUrl: '/api/v1/trakt/device',
          );
          _startPoll();
        }
      });
    } catch (e) {
      _toast('加载失败：$e');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _openConfig() async {
    final cfg = _cfg;
    if (!mounted) return;
    final saved = await showAdaptiveSheet<bool>(
      context,
      title: 'Trakt 配置',
      builder: (_) => _TraktConfigForm(
        cfg: cfg,
        onSubmit: (body) async {
          try {
            await _api.setConfig(body);
            _toast('已保存');
            return true;
          } catch (e) {
            _toast('保存失败：$e');
            return false;
          }
        },
      ),
    );
    if (saved == true) await _load();
  }

  Future<void> _startAuth() async {
    setState(() => _busy = true);
    try {
      final dc = await _api.deviceCode();
      if (!mounted) return;
      setState(() => _device = dc);
      _toast('请在打开的页面输入授权码');
      await _launch(dc.verificationUrl);
      _startPoll();
    } catch (e) {
      // 从前这句猜死一个原因（「请先在配置填 client_id/secret」），真被墙/超时时会把人指错方向；
      // ApiException.toString() 就是后端那句分档好的人话。
      _toast('发起授权失败：$e');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  void _startPoll() {
    _poll?.cancel();
    var fails = 0;
    _poll = Timer.periodic(const Duration(seconds: 3), (_) async {
      final st = await _api.deviceStatus();
      if (st.authorized) {
        _poll?.cancel();
        if (!mounted) return;
        setState(() => _device = null);
        _toast('Trakt 授权成功');
        await _load();
        return;
      }
      if (st.terminal) {
        // 授权码过期 / 凭据没填：停下来把后端那句下一步说出来，别让用户盯着废码干等。
        _poll?.cancel();
        if (!mounted) return;
        setState(() => _device = null);
        _toast(st.message.isEmpty ? '设备授权未能完成，请重新发起授权' : st.message);
        return;
      }
      // 什么都判不出来（网络抖动）：连续多次才放弃，措辞不许说成「授权失败」。
      if (++fails >= traktPollGiveUpAfter) {
        _poll?.cancel();
        _toast(traktPollGiveUpText(fails));
      }
    });
  }

  Future<void> _sync() async {
    setState(() => _busy = true);
    try {
      final n = await _api.sync();
      _toast('同步完成，新增 $n 条订阅');
      await _load();
    } catch (e) {
      _toast('同步失败：$e');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _launch(String url) async {
    if (url.isEmpty) return;
    final uri = Uri.tryParse(url);
    if (uri != null) await launchUrl(uri, mode: LaunchMode.externalApplication);
  }

  @override
  Widget build(BuildContext context) {
    final cfg = _cfg;
    final authorized = cfg?.authorized ?? false;
    return PTPageScaffold(
      title: 'Trakt',
      actions: [
        PTBarAction(icon: Icons.settings, label: '配置', onPressed: _openConfig),
      ],
      body: ListView(
        padding: const EdgeInsets.fromLTRB(12, 8, 12, 24),
        children: [
          Text('把 Trakt.tv 的 WatchList 自动同步为本地订阅。',
              style: TextStyle(fontSize: 12, color: context.dimText)),
          const SizedBox(height: 12),
          if (_loading)
            const PTLoadingView()
          else if (cfg == null)
            const Padding(padding: EdgeInsets.all(40), child: Center(child: Text('无配置')))
          else ...[
            _statusCard(cfg, authorized),
            if (_device != null) _deviceCard(_device!),
            const SizedBox(height: 12),
            FilledButton.icon(
              onPressed: _busy ? null : (authorized ? _sync : _startAuth),
              icon: Icon(authorized ? Icons.sync : Icons.login),
              label: Text(authorized ? '立即同步' : '授权 Trakt'),
            ),
          ],
        ],
      ),
    );
  }

  Widget _statusCard(TraktConfig cfg, bool authorized) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(14),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Row(children: [
            Icon(Icons.circle, size: 12, color: authorized ? Colors.green : Colors.red),
            const SizedBox(width: 8),
            Text(authorized ? '已授权' : '未授权', style: const TextStyle(fontWeight: FontWeight.w600)),
            if (cfg.enabled) ...[
              const SizedBox(width: 10),
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
                decoration: BoxDecoration(color: Colors.deepPurple.withValues(alpha: 0.12), borderRadius: BorderRadius.circular(10)),
                child: const Text('同步已开启', style: TextStyle(fontSize: 11, color: Colors.deepPurple)),
              ),
            ],
          ]),
          if (cfg.lastSyncAt > 0)
            Padding(
              padding: const EdgeInsets.only(top: 8),
              child: Text(
                '最近同步：${DateTime.fromMillisecondsSinceEpoch(cfg.lastSyncAt * 1000).toLocal()} · 新增 ${cfg.lastSyncCount} 条',
                style: TextStyle(fontSize: 12, color: context.dimText),
              ),
            ),
          if (cfg.lastError.isNotEmpty)
            Padding(padding: const EdgeInsets.only(top: 6), child: Text(cfg.lastError, style: const TextStyle(fontSize: 12, color: Colors.red))),
        ]),
      ),
    );
  }

  Widget _deviceCard(TraktDeviceCode dc) {
    return Card(
      margin: const EdgeInsets.only(top: 12),
      child: Padding(
        padding: const EdgeInsets.all(14),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Text('1. 打开授权页：'),
          InkWell(
            onTap: () => _launch(dc.verificationUrl),
            child: Text(dc.verificationUrl, style: const TextStyle(color: Colors.blue, decoration: TextDecoration.underline)),
          ),
          const SizedBox(height: 8),
          Text('2. 输入授权码：'),
          const SizedBox(height: 4),
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 8),
            decoration: BoxDecoration(color: Colors.black.withValues(alpha: 0.05), borderRadius: BorderRadius.circular(8)),
            child: Text(dc.userCode, style: const TextStyle(fontSize: 24, fontWeight: FontWeight.bold, letterSpacing: 3)),
          ),
          const SizedBox(height: 6),
          Text('授权完成后本页自动检测。', style: TextStyle(fontSize: 11, color: context.dimText)),
        ]),
      ),
    );
  }
}

/// Trakt 配置表单（底部 sheet）。
class _TraktConfigForm extends StatefulWidget {
  final TraktConfig? cfg;
  final Future<bool> Function(Map<String, dynamic>) onSubmit;
  const _TraktConfigForm({required this.cfg, required this.onSubmit});
  @override
  State<_TraktConfigForm> createState() => _TraktConfigFormState();
}

class _TraktConfigFormState extends State<_TraktConfigForm> {
  bool _enabled = false;
  final _cid = TextEditingController();
  final _secret = TextEditingController();
  final _redirect = TextEditingController(text: 'urn:ietf:wg:oauth:2.0:oob');
  bool _movies = true;
  bool _shows = true;
  final _res = TextEditingController();
  bool _saving = false;

  @override
  void initState() {
    super.initState();
    final c = widget.cfg;
    if (c != null) {
      _enabled = c.enabled;
      _cid.text = c.clientId;
      _redirect.text = c.redirectUri.isNotEmpty ? c.redirectUri : 'urn:ietf:wg:oauth:2.0:oob';
      _movies = c.syncMovies;
      _shows = c.syncShows;
      _res.text = c.resolution;
    }
  }

  @override
  void dispose() {
    _cid.dispose();
    _secret.dispose();
    _redirect.dispose();
    _res.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    setState(() => _saving = true);
    final ok = await widget.onSubmit({
      'enabled': _enabled,
      'client_id': _cid.text.trim(),
      if (_secret.text.isNotEmpty) 'client_secret': _secret.text.trim(),
      'redirect_uri': _redirect.text.trim(),
      'sync_movies': _movies,
      'sync_shows': _shows,
      'resolution': _res.text.trim(),
    });
    if (!mounted) return;
    setState(() => _saving = false);
    if (ok) Navigator.of(context).pop(true);
  }

  @override
  Widget build(BuildContext context) {
    return SingleChildScrollView(
      child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, mainAxisSize: MainAxisSize.min, children: [
        SwitchListTile(title: const Text('启用定时同步'), value: _enabled, onChanged: (v) => setState(() => _enabled = v)),
        _field('Client ID', TextField(controller: _cid, decoration: const InputDecoration(hintText: 'trakt.tv 创建应用所得'))),
        _field(
          'Client Secret',
          TextField(
            controller: _secret,
            obscureText: true,
            decoration: InputDecoration(hintText: widget.cfg?.clientSecretConfigured == true ? '已配置（留空=不修改）' : 'secret'),
          ),
        ),
        _field('Redirect URI', TextField(controller: _redirect)),
        SwitchListTile(title: const Text('同步电影'), value: _movies, onChanged: (v) => setState(() => _movies = v)),
        SwitchListTile(title: const Text('同步剧集'), value: _shows, onChanged: (v) => setState(() => _shows = v)),
        _field('默认分辨率', TextField(controller: _res, decoration: const InputDecoration(hintText: '如 1080p，留空不过滤'))),
        const SizedBox(height: 8),
        FilledButton(onPressed: _saving ? null : _submit, child: Text(_saving ? '保存中…' : '保存')),
      ]),
    );
  }

  Widget _field(String label, Widget child) => Padding(
        padding: const EdgeInsets.only(bottom: 12, left: 16, right: 16),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Text(label, style: TextStyle(fontSize: 12, fontWeight: FontWeight.w600, color: context.dimText)),
          const SizedBox(height: 6),
          child,
        ]),
      );
}
