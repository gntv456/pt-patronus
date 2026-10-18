// 仪表盘模型解析测试：匹配后端 /dashboard 与 /dashboard/trend 响应形状（含缺字段容错）。
import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/api/dashboard.dart';

void main() {
  test('DashboardOverview.fromJson parses totals + sites + counts', () {
    final o = DashboardOverview.fromJson({
      'totals': {
        'uploaded': 13629248163840,
        'downloaded': 1975684956160,
        'seeding': 328,
        'leeching': 12,
        'seeding_size': 2199023255552,
        'bonus': 1234.5,
        'bonus_per_hour': 56.7,
        'rescue_seeding_bonus_per_hour': 3.25,
        'unique_groups': 6,
        'perfect_flac': 5,
        'invited': 4,
      },
      'sites': [
        {
          'site_id': 1,
          'site_name': '好学',
          'level': '维护开发员',
          'uploaded': 1000,
          'downloaded': 200,
          'ratio': 6.89,
          'seeding': 10,
          'leeching': 1,
          'seeding_size': 1073741824,
          'uploads': 5,
          'bonus': 9.0,
          'bonus_per_hour': 123.4,
          'rescue_seeding_bonus_per_hour': 10.0,
          'unique_groups': 6,
          'perfect_flac': 5,
          'invited': 4,
          'join_date': '2024-01-15',
          'updated_at': '2026-06-26T15:28:00+08:00',
        },
        {'site_id': 2, 'site_name': '馒头', 'uploaded': 2000, 'bonus': 5.5},
      ],
      'site_count': 2,
      'downloader_count': 1,
    });
    expect(o.uploaded, 13629248163840);
    expect(o.downloaded, 1975684956160);
    expect(o.seeding, 328);
    expect(o.leeching, 12);
    expect(o.seedingSize, 2199023255552);
    expect(o.bonus, 1234.5);
    expect(o.bonusPerHour, 56.7);
    expect(o.rescueSeedingBonusPerHour, 3.25);
    expect(o.uniqueGroups, 6);
    expect(o.perfectFlac, 5);
    expect(o.invited, 4);
    expect(o.sites.length, 2);
    // 站点明细新字段（此前 fromJson 容错丢弃，现已同步）。
    final hx = o.sites[0];
    expect(hx.siteName, '好学');
    expect(hx.level, '维护开发员');
    expect(hx.ratio, 6.89);
    expect(hx.leeching, 1);
    expect(hx.seedingSize, 1073741824);
    expect(hx.uploads, 5);
    expect(hx.bonusPerHour, 123.4);
    expect(hx.rescueSeedingBonusPerHour, 10.0);
    expect(hx.uniqueGroups, 6);
    expect(hx.perfectFlac, 5);
    expect(hx.invited, 4);
    expect(hx.joinDate, '2024-01-15');
    expect(hx.updatedAt, '2026-06-26T15:28:00+08:00');
    // 缺字段的站点：新字段回落默认值，不报错。
    final mt = o.sites[1];
    expect(mt.level, '');
    expect(mt.seedingSize, 0);
    expect(mt.uploads, 0);
    expect(mt.bonusPerHour, 0);
    expect(mt.rescueSeedingBonusPerHour, 0);
    expect(mt.uniqueGroups, 0);
    expect(mt.perfectFlac, 0);
    expect(mt.invited, 0);
    expect(mt.joinDate, '');
    expect(o.siteCount, 2);
    expect(o.downloaderCount, 1);
  });

  test('DashboardOverview.fromJson tolerates missing totals/sites', () {
    final o = DashboardOverview.fromJson({});
    expect(o.uploaded, 0);
    expect(o.seeding, 0);
    expect(o.seedingSize, 0);
    expect(o.bonusPerHour, 0);
    expect(o.rescueSeedingBonusPerHour, 0);
    expect(o.uniqueGroups, 0);
    expect(o.perfectFlac, 0);
    expect(o.invited, 0);
    expect(o.sites, isEmpty);
    expect(o.siteCount, 0);
  });

  test('DashboardTrendPoint.fromJson parses fields + snake_case day', () {
    final p = DashboardTrendPoint.fromJson({
      'day': '2026-06-19',
      'uploaded': 13629248163840,
      'downloaded': 1975684956160,
      'seeding': 328,
      'bonus': 1234.5,
    });
    expect(p.day, '2026-06-19');
    expect(p.uploaded, 13629248163840);
    expect(p.downloaded, 1975684956160);
    expect(p.seeding, 328);
    expect(p.bonus, 1234.5);
  });

  test('DashboardTrendPoint.fromJson tolerates missing fields', () {
    final p = DashboardTrendPoint.fromJson({'day': '2026-06-19'});
    expect(p.day, '2026-06-19');
    expect(p.uploaded, 0);
    expect(p.seeding, 0);
    expect(p.bonus, 0);
  });

  // #255：移动端从前没有 stat_read_error 这个字段，后端那句原因在解析层就蒸发，
  // 于是库读失败在手机上画成「每站 0 上传 0 下载」且没有任何说明。
  test('DashboardOverview 带出每站统计的读取失败原因', () {
    final o = DashboardOverview.fromJson({
      'sites': [
        {'site_id': 1, 'site_name': '好学'},
      ],
      'site_count': 1,
      'downloader_count': 2,
      'stat_read_error': 'database is locked',
    });
    expect(o.statReadError, 'database is locked');
    // 读失败时数字仍是 0：只有这条账面能把它和「真没数据」分开。
    expect(o.uploaded, 0);
  });

  test('正常读数不带原因（缺字段/空串都算没有，不许画出一条假告警）', () {
    expect(
      DashboardOverview.fromJson({'sites': [], 'site_count': 0}).statReadError,
      isNull,
    );
    expect(
      DashboardOverview.fromJson({
        'sites': [],
        'site_count': 0,
        'stat_read_error': '   ',
      }).statReadError,
      isNull,
    );
  });
}
