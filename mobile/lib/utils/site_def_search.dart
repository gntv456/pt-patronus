// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:ptpatronus/api/site.dart';

final RegExp _edgeNoiseRe = RegExp(
  r'''^[\s"'`“”‘’「」『』《》【】<>{}\[\](),，.。;；:：]+|[\s"'`“”‘’「」『』《》【】<>{}\[\](),，.。;；:：]+$''',
);

String normalizeSiteDefSearchInput(String raw) {
  return raw.trim().replaceAll(_edgeNoiseRe, '').trim();
}

List<String> siteDefURLSearchTerms(String raw) {
  final normalized = normalizeSiteDefSearchInput(raw);
  final trimmed = normalized.isNotEmpty ? normalized : raw.trim();
  if (trimmed.isEmpty) return const [];

  final terms = <String>{trimmed};
  if (normalized.isNotEmpty && normalized != raw.trim()) {
    terms.add(normalized);
  }

  final withoutTrailingSlash = trimmed.replaceFirst(RegExp(r'/+$'), '');
  if (withoutTrailingSlash.isNotEmpty) {
    terms.add(withoutTrailingSlash);
    terms.add('$withoutTrailingSlash/');
  }

  final hasScheme = RegExp(
    r'^[a-z][a-z0-9+\-.]*://',
    caseSensitive: false,
  ).hasMatch(trimmed);
  final uri = Uri.tryParse(hasScheme ? trimmed : 'https://$trimmed');
  final host = uri?.host.toLowerCase().replaceFirst(RegExp(r'^www\.'), '');
  if (host != null && host.isNotEmpty) {
    terms.add(host);
    terms.add('https://$host');
    terms.add('https://$host/');
    final path = uri?.path.replaceFirst(RegExp(r'/+$'), '') ?? '';
    if (path.isNotEmpty) terms.add('$host$path');
  }

  return terms.toList();
}

String siteDefSearchText(SiteDef d) {
  return [
    d.id,
    d.name,
    d.schema,
    ...d.urls,
    ...d.urls.expand(siteDefURLSearchTerms),
    ...d.aka,
    d.description,
    ...d.tags,
  ].join(' ').toLowerCase();
}

List<String> siteDefSearchQueries(String query) {
  final normalized = normalizeSiteDefSearchInput(query);
  return <String>{query, normalized, ...siteDefURLSearchTerms(query)}
      .map((term) => term.trim().toLowerCase())
      .where((term) => term.isNotEmpty)
      .toList();
}

List<SiteDef> filterSiteDefsBySearch(
  List<SiteDef> defs,
  String query, {
  int? limit = 50,
}) {
  final raw = query.trim();
  if (raw.isEmpty) return defs;
  final queries = siteDefSearchQueries(raw);
  final matches = defs.where((d) {
    final text = siteDefSearchText(d);
    return queries.any(text.contains);
  }).toList();
  matches.sort((a, b) {
    return _scoreSiteDef(
      b,
      raw,
      queries,
    ).compareTo(_scoreSiteDef(a, raw, queries));
  });
  return limit == null ? matches : matches.take(limit).toList();
}

int _scoreSiteDef(SiteDef d, String raw, List<String> queries) {
  final normalized = normalizeSiteDefSearchInput(raw).toLowerCase();
  final directTerms =
      <String>[
            d.id,
            d.name,
            d.schema,
            ...d.urls,
            ...d.aka,
            ...d.urls.expand(siteDefURLSearchTerms),
          ]
          .map((term) => term.trim().toLowerCase())
          .where((term) => term.isNotEmpty)
          .toList();

  if (directTerms.contains(normalized)) return 100;
  if (queries.any(directTerms.contains)) return 95;
  if (directTerms.any((term) => term.startsWith(normalized))) return 80;
  if (queries.any((query) {
    return directTerms.any((term) => term.startsWith(query));
  })) {
    return 70;
  }
  return siteDefSearchText(d).contains(normalized) ? 10 : 0;
}
