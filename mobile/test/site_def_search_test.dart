// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/api/site.dart';
import 'package:ptpatronus/utils/site_def_search.dart';

const pthome = SiteDef(
  id: 'pthome',
  name: '铂金家',
  schema: 'nexusphp',
  urls: ['https://pthome.net'],
  aka: ['PTHome', 'PT之家', '铂金家', 'PTH'],
);

const ptfans = SiteDef(
  id: 'ptfans',
  name: 'PTFans',
  schema: 'nexusphp',
  urls: ['https://ptfans.cc'],
);

void main() {
  test('matches PTHome by quoted URL', () {
    final queries = siteDefSearchQueries('’https://pthome.net’');

    expect(queries, contains('https://pthome.net'));
    expect(queries, contains('pthome.net'));
    expect(siteDefSearchText(pthome), contains('pthome.net'));
  });

  test('prioritizes exact URL and alias matches', () {
    expect(
      filterSiteDefsBySearch(const [
        ptfans,
        pthome,
      ], '’https://pthome.net’').first.id,
      'pthome',
    );
    expect(
      filterSiteDefsBySearch(const [ptfans, pthome], 'PT之家').first.id,
      'pthome',
    );
    expect(
      filterSiteDefsBySearch(const [ptfans, pthome], '铂金家').first.id,
      'pthome',
    );
  });

  test(
    'keeps PTHome available when the candidate list is opened without a query',
    () {
      final ids = filterSiteDefsBySearch(
        const [ptfans, pthome],
        '',
        limit: null,
      ).map((d) => d.id);

      expect(ids, contains('pthome'));
    },
  );
}
