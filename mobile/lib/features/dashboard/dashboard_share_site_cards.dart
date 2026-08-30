// _DashboardPageState 的分享站点卡组：站点卡/图标/身份信息。
// （二次拆分；行为不变。）
part of 'package:ptpatronus/features/dashboard/dashboard_page.dart';

extension on _DashboardPageState {
  Widget _shareSiteCard(SiteStatItem s, int index) {
    final metrics = <ShareMetric>[
      if (_config.showUsername)
        ShareMetric(
          '用户名',
          _config.blurUserInfo
              ? '***'
              : (s.username.isEmpty ? '--' : s.username),
          const Color(0xFFF8FAFC),
        ),
      if (_config.showUsername)
        ShareMetric(
          'UID',
          _config.blurUserInfo ? '***' : (s.userId.isEmpty ? '--' : s.userId),
          const Color(0xFFF8FAFC),
        ),
      if (_config.showUsername)
        ShareMetric(
          '等级',
          _config.blurUserInfo ? '***' : (s.level.isEmpty ? '--' : s.level),
          const Color(0xFFF8FAFC),
        ),
      if (_config.showUploadCount)
        ShareMetric(
          '发布',
          s.uploads > 0 ? '${s.uploads}' : '--',
          const Color(0xFFFFC857),
        ),
      if (_config.showSeedingCount)
        ShareMetric('做种', '${s.seeding}', const Color(0xFF45C4F0)),
      if (_config.showUpload)
        ShareMetric('上传', _fmtBytes(s.uploaded), const Color(0xFF62D48E)),
      if (_config.showDownload)
        ShareMetric('下载', _fmtBytes(s.downloaded), const Color(0xFFFF6B6B)),
      if (_config.showRatio)
        ShareMetric('分享率', _fmtRatio(s.ratio), const Color(0xFF45C4F0)),
      if (_config.showSeedingVolume)
        ShareMetric(
          '做种量',
          s.seedingSize > 0 ? _fmtBytes(s.seedingSize) : '--',
          const Color(0xFF62D48E),
        ),
      if (_config.showBonus)
        ShareMetric(
          '积分',
          s.bonus > 0 ? _fmtCompact(s.bonus) : '--',
          const Color(0xFFFFC857),
        ),
      if (_config.showBonusPerHour)
        ShareMetric(
          '每小时积分',
          s.bonusPerHour > 0 ? _fmtCompact(s.bonusPerHour) : '--',
          const Color(0xFFFF9F43),
        ),
    ];
    return _sharePanel(
      color: index.isEven ? const Color(0xFF1E293B) : const Color(0xFF263238),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              _shareSiteIcon(s, index),
              const SizedBox(width: 9),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      _shareDisplaySiteName(s),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(
                        color: Color(0xFFF8FAFC),
                        fontSize: 14,
                        fontWeight: FontWeight.w800,
                      ),
                    ),
                    const SizedBox(height: 2),
                    Text(
                      _config.showUsername
                          ? _shareDisplayIdentity(s)
                          : '用户信息已隐藏',
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(
                        color: Color(0xFFB6C2CF),
                        fontSize: 10,
                      ),
                    ),
                  ],
                ),
              ),
            ],
          ),
          const SizedBox(height: 12),
          _shareMetricGrid(metrics, columns: 4),
        ],
      ),
    );
  }

  Widget _shareSiteIcon(SiteStatItem s, int index) {
    final fallback = _shareSiteIconFallback(s, index);
    final url = _siteFaviconUrl(s);
    if (url == null) return fallback;
    return Container(
      width: 28,
      height: 28,
      clipBehavior: Clip.antiAlias,
      decoration: const BoxDecoration(
        color: Color(0xFFF8FAFC),
        shape: BoxShape.circle,
      ),
      child: Image.network(
        url,
        fit: BoxFit.contain,
        filterQuality: FilterQuality.medium,
        gaplessPlayback: true,
        loadingBuilder: (context, child, progress) =>
            progress == null ? child : fallback,
        errorBuilder: (context, error, stackTrace) => fallback,
      ),
    );
  }

  Widget _shareSiteIconFallback(SiteStatItem s, int index) {
    final name = _shareDisplaySiteName(s).trim();
    final label = name.isEmpty || name == '--' || name == '***'
        ? '${index + 1}'
        : name.substring(0, 1).toUpperCase();
    return Container(
      width: 28,
      height: 28,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: _shareSiteAccent(index),
        shape: BoxShape.circle,
      ),
      child: Text(
        label,
        style: const TextStyle(
          color: Color(0xFF121212),
          fontSize: 12,
          fontWeight: FontWeight.w800,
        ),
      ),
    );
  }

  Color _shareSiteAccent(int index) {
    const colors = [
      Color(0xFF45C4F0),
      Color(0xFF62D48E),
      Color(0xFFFFC857),
      Color(0xFFB98CFF),
      Color(0xFFFF9F43),
      Color(0xFFFF6B6B),
    ];
    return colors[index % colors.length];
  }

  String _shareDisplayUser(List<SiteStatItem> sites) {
    if (_config.blurUserInfo) return '***';
    if (!_config.showUsername) return 'PT 用户';
    final counts = <String, int>{};
    for (final site in sites) {
      final username = site.username.trim();
      if (username.isNotEmpty) {
        counts[username] = (counts[username] ?? 0) + 1;
      }
    }
    if (counts.isEmpty) return 'PT 用户';
    final entries = counts.entries.toList()
      ..sort((a, b) {
        final byCount = b.value.compareTo(a.value);
        return byCount != 0 ? byCount : a.key.compareTo(b.key);
      });
    return entries.first.key;
  }

  String _shareUserInitial(String displayUser) {
    if (displayUser.isEmpty || displayUser == '***') return 'P';
    return displayUser.substring(0, 1).toUpperCase();
  }

  String _shareDisplaySiteName(SiteStatItem site) {
    if (!_config.showSiteName) return '站点信息已隐藏';
    if (_config.blurSensitiveInfo) return '***';
    return site.siteName.isEmpty ? '--' : site.siteName;
  }

  String _shareDisplayIdentity(SiteStatItem site) {
    if (_config.blurUserInfo) return '*** <***> UID ***';
    final parts = <String>[
      if (site.username.isNotEmpty) site.username,
      if (site.userId.isNotEmpty) 'UID ${site.userId}',
      if (site.level.isNotEmpty) '<${site.level}>',
      if (site.email.isNotEmpty) _shareDisplayEmail(site.email),
    ];
    return parts.isEmpty ? '--' : parts.join('  ');
  }

  String _shareDisplayEmail(String email) {
    if (_config.blurUserInfo) return '***';
    final at = email.indexOf('@');
    if (at <= 0) return email;
    final local = email.substring(0, at);
    final domain = email.substring(at + 1);
    final suffix = domain.contains('.') ? domain.split('.').last : '';
    return '${local.substring(0, 1)}***@${suffix.isEmpty ? '***' : '***.$suffix'}';
  }
}
