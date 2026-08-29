/**
 * Site favicon URL candidates.
 * Prefer site-local icons; fall back to public resolvers when /favicon.ico is missing
 * or blocked by hotlink protection.
 */
export function faviconCandidates(baseUrl: string): string[] {
  const raw = (baseUrl || '').trim()
  if (!raw) return []

  let parsed: URL
  try {
    parsed = new URL(raw)
  } catch {
    return []
  }
  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') return []

  const origin = parsed.origin
  const base = raw.replace(/\/+$/, '')
  const host = parsed.hostname
  const out: string[] = []
  const push = (u: string) => {
    if (u && !out.includes(u)) out.push(u)
  }

  // Common local locations (base may include a subpath like /pt)
  push(`${base}/favicon.ico`)
  push(`${origin}/favicon.ico`)
  push(`${base}/favicon.png`)
  push(`${origin}/favicon.png`)
  push(`${origin}/apple-touch-icon.png`)
  push(`${origin}/apple-touch-icon-precomposed.png`)

  // Public resolvers — last resort when site path/hotlink fails
  push(`https://icons.duckduckgo.com/ip3/${host}.ico`)
  push(`https://www.google.com/s2/favicons?domain=${encodeURIComponent(host)}&sz=64`)

  return out
}

/** First candidate only (legacy single-URL callers). */
export function primaryFaviconUrl(baseUrl: string): string {
  return faviconCandidates(baseUrl)[0] || ''
}
