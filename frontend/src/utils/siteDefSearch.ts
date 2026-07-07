import type { SiteDef } from '@/api/site'

const edgeNoiseRe = /^[\s"'`“”‘’「」『』《》【】<>{}\[\](),，.。;；:：]+|[\s"'`“”‘’「」『』《》【】<>{}\[\](),，.。;；:：]+$/g

export function normalizeSiteDefSearchInput(raw: string) {
  return raw.trim().replace(edgeNoiseRe, '').trim()
}

export function siteDefURLSearchTerms(raw: string) {
  const normalized = normalizeSiteDefSearchInput(raw)
  const trimmed = normalized || raw.trim()
  if (!trimmed) return []

  const terms = new Set<string>([trimmed])
  if (normalized && normalized !== raw.trim()) terms.add(normalized)

  const withoutTrailingSlash = trimmed.replace(/\/+$/, '')
  if (withoutTrailingSlash) {
    terms.add(withoutTrailingSlash)
    terms.add(`${withoutTrailingSlash}/`)
  }

  try {
    const withScheme = /^[a-z][a-z\d+\-.]*:\/\//i.test(trimmed) ? trimmed : `https://${trimmed}`
    const url = new URL(withScheme)
    const host = url.hostname.replace(/^www\./i, '')
    if (host) {
      terms.add(host)
      terms.add(`https://${host}`)
      terms.add(`https://${host}/`)
      const path = url.pathname.replace(/\/+$/, '')
      if (path) terms.add(`${host}${path}`)
    }
  } catch {
    // Non-URL search terms still participate through the raw query above.
  }

  return Array.from(terms)
}

export function siteDefSearchText(d: SiteDef) {
  return [
    d.id,
    d.name,
    d.schema,
    ...(d.aka || []),
    ...(d.tags || []),
    ...(d.urls || []),
    ...(d.urls || []).flatMap(siteDefURLSearchTerms),
    d.description || '',
  ]
    .join(' ')
    .toLowerCase()
}

export function siteDefSearchQueries(query: string) {
  const normalized = normalizeSiteDefSearchInput(query)
  return Array.from(
    new Set([query, normalized, ...siteDefURLSearchTerms(query)].map((term) => term.trim().toLowerCase()).filter(Boolean)),
  )
}

export function sortSiteDefsBySearchRelevance(defs: SiteDef[], query: string) {
  const normalized = normalizeSiteDefSearchInput(query).toLowerCase()
  if (!normalized) return defs
  const queries = siteDefSearchQueries(query)
  return [...defs].sort((a, b) => scoreSiteDef(b, normalized, queries) - scoreSiteDef(a, normalized, queries))
}

function scoreSiteDef(d: SiteDef, normalized: string, queries: string[]) {
  const directTerms = [
    d.id,
    d.name,
    d.schema,
    ...(d.aka || []),
    ...(d.urls || []),
    ...(d.urls || []).flatMap(siteDefURLSearchTerms),
  ]
    .map((term) => term.trim().toLowerCase())
    .filter(Boolean)

  if (directTerms.includes(normalized)) return 100
  if (queries.some((query) => directTerms.includes(query))) return 95
  if (directTerms.some((term) => term.startsWith(normalized))) return 80
  if (queries.some((query) => directTerms.some((term) => term.startsWith(query)))) return 70
  return siteDefSearchText(d).includes(normalized) ? 10 : 0
}
