import { describe, expect, it } from 'vitest'
import type { SiteDef } from '@/api/site'
import { siteDefSearchQueries, siteDefSearchText, sortSiteDefsBySearchRelevance } from './siteDefSearch'

const pthome: SiteDef = {
  id: 'pthome',
  name: '铂金家',
  schema: 'nexusphp',
  dead: false,
  urls: ['https://pthome.net'],
  aka: ['PTHome', 'PT之家', '铂金家', 'PTH'],
}

const other: SiteDef = {
  id: 'ptfans',
  name: 'PTFans',
  schema: 'nexusphp',
  dead: false,
  urls: ['https://ptfans.cc'],
  aka: [],
}

describe('site definition search', () => {
  it('matches PTHome by quoted URL', () => {
    const queries = siteDefSearchQueries('’https://pthome.net’')
    expect(queries).toContain('https://pthome.net')
    expect(queries).toContain('pthome.net')
    expect(siteDefSearchText(pthome)).toContain('pthome.net')
  })

  it('prioritizes exact URL and alias matches', () => {
    expect(sortSiteDefsBySearchRelevance([other, pthome], '’https://pthome.net’')[0].id).toBe('pthome')
    expect(sortSiteDefsBySearchRelevance([other, pthome], 'PT之家')[0].id).toBe('pthome')
  })
})
