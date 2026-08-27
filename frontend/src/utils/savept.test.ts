import { describe, it, expect } from 'vitest'
import type { SaveptSiteCard, SiteHealthStatus } from '@/api/savept'
import {
  getStatusMeta,
  getMpStatusMeta,
  siteTypeLabel,
  buildSiteKeywords,
  getAnniversaryDisplayText,
  isTodayAnniversaryText,
  buildNoticeSections,
  tokenizeNoticeText,
} from './savept'

// PT监护室前端工具：状态元信息/站庆文案/公告 4 分区/站点名高亮分词。
// 这些纯函数直接驱动卡片与公告 UI，错一个映射就整页错乱。

describe('getStatusMeta', () => {
  it('healthy/critical/closed 各自元信息', () => {
    expect(getStatusMeta('healthy').label).toBe('健康')
    expect(getStatusMeta('healthy').cardClass).toBe('healthy')
    expect(getStatusMeta('critical').label).toBe('病危')
    expect(getStatusMeta('closed').label).toBe('死亡')
  })
  it('未知/其他 → 未知', () => {
    expect(getStatusMeta('unknown').label).toBe('未知')
    // 故意传非法值测 default 分支（运行时兜底），需 cast 绕过字面量类型
    expect(getStatusMeta('whatever' as SiteHealthStatus).label).toBe('未知')
    expect(getStatusMeta('whatever' as SiteHealthStatus).icon).toBe('')
  })
})

describe('getMpStatusMeta', () => {
  it('精确匹配类 → 已拥有', () => {
    expect(getMpStatusMeta('url_exact').label).toBe('已拥有')
    expect(getMpStatusMeta('domain_exact').label).toBe('已拥有')
  })
  it('名称类 → 可添加', () => {
    expect(getMpStatusMeta('name_exact').label).toBe('可添加')
    expect(getMpStatusMeta('name_fuzzy').label).toBe('可添加')
  })
  it('none/其他 → 未收录', () => {
    expect(getMpStatusMeta('none').label).toBe('未收录')
  })
})

describe('siteTypeLabel', () => {
  it('内站/外站/未知', () => {
    expect(siteTypeLabel('internal')).toBe('内站')
    expect(siteTypeLabel('external')).toBe('外站')
    expect(siteTypeLabel(undefined)).toBe('未知')
    expect(siteTypeLabel('')).toBe('未知')
  })
})

describe('buildSiteKeywords', () => {
  it('聚合 name/search/year/status_text/type 并小写', () => {
    const card = {
      name: 'MTeam',
      search: 'mteam',
      year: '2020',
      status_text: '病危',
      site_type: 'internal',
    } as SaveptSiteCard
    expect(buildSiteKeywords(card)).toBe('mteam mteam 2020 病危 内站')
  })
  it('缺字段过滤 + 空卡片（site_type 缺省→未知 仍计入）', () => {
    // siteTypeLabel(undefined)='未知' 是真值，不被 filter(Boolean) 去掉
    expect(buildSiteKeywords({} as SaveptSiteCard)).toBe('未知')
    expect(buildSiteKeywords({ name: 'A' } as SaveptSiteCard)).toBe('a 未知')
  })
})

describe('getAnniversaryDisplayText', () => {
  it('空/空白 → 空串', () => {
    expect(getAnniversaryDisplayText(undefined)).toBe('')
    expect(getAnniversaryDisplayText('')).toBe('')
    expect(getAnniversaryDisplayText('   ')).toBe('')
  })
  it('今天站庆 / 0天 → 🎉 今天站庆！', () => {
    expect(getAnniversaryDisplayText('今天站庆')).toBe('🎉 今天站庆！')
    expect(getAnniversaryDisplayText('0天')).toBe('🎉 今天站庆！')
  })
  it('剥离开头 emoji 站庆 前缀', () => {
    expect(getAnniversaryDisplayText('emoji 站庆 5天')).toBe('5天')
    expect(getAnniversaryDisplayText('EMOJI站庆30天')).toBe('30天')
  })
  it('普通文案原样返回', () => {
    expect(getAnniversaryDisplayText('5天')).toBe('5天')
  })
})

describe('isTodayAnniversaryText', () => {
  it('仅今天站庆/0天 为 true', () => {
    expect(isTodayAnniversaryText('今天站庆')).toBe(true)
    expect(isTodayAnniversaryText('0天')).toBe(true)
    expect(isTodayAnniversaryText('5天')).toBe(false)
    expect(isTodayAnniversaryText('')).toBe(false)
  })
})

describe('buildNoticeSections', () => {
  it('空数组仍返回 4 个分区（顺序：病危/恢复/站庆/讣告）', () => {
    const secs = buildNoticeSections([])
    expect(secs.map((s) => s.key)).toEqual(['critical', 'success', 'info', 'error'])
    expect(secs.every((s) => s.items.length === 0)).toBe(true)
  })
  it('alert type → 4 分区映射，空 content 丢弃', () => {
    const secs = buildNoticeSections([
      { type: 'danger', title: 't', content: '病危通知A' },
      { type: 'obituary', title: 't', content: '讣告B' },
      { type: 'warning', title: 't', content: '恢复C' },
      { type: 'celebration', title: 't', content: '站庆D' },
      { type: 'info', title: 't', content: '   ' }, // 空白 → 丢弃
    ])
    const byKey = Object.fromEntries(secs.map((s) => [s.key, s.items]))
    expect(byKey.critical).toEqual(['病危通知A'])
    expect(byKey.error).toEqual(['讣告B'])
    expect(byKey.success).toEqual(['恢复C'])
    expect(byKey.info).toEqual(['站庆D']) // celebration 落 info
  })
})

describe('tokenizeNoticeText', () => {
  it('空/空白 → []', () => {
    expect(tokenizeNoticeText('', [])).toEqual([])
    expect(tokenizeNoticeText('   ', [])).toEqual([])
  })
  it('已知站点名开头的正文 → 站点名高亮 token', () => {
    const cards = [{ name: 'MTeam' } as SaveptSiteCard]
    const tokens = tokenizeNoticeText('MTeam 已抢救成功', cards)
    expect(tokens).toEqual([
      { type: 'site', text: 'MTeam' },
      { type: 'text', text: ' 已抢救成功' },
    ])
  })
  it('无已知站点/无启发式 → 整段 text', () => {
    const tokens = tokenizeNoticeText('系统维护通知', [])
    expect(tokens).toEqual([{ type: 'text', text: '系统维护通知' }])
  })
  it('「|」分隔符在行内保留（合并进相邻 text）', () => {
    const tokens = tokenizeNoticeText('a | b', [])
    expect(tokens).toEqual([{ type: 'text', text: 'a | b' }])
  })
})

// ── 站庆日历 ──
import { parseAnnivCountdownDays, parseOpenDate, buildAnniversaryMap, hasKnownAnniversaryDate } from './savept'

function annivCard(overrides: Partial<SaveptSiteCard> = {}): SaveptSiteCard {
  return {
    name: '站点A', url: 'https://a.pt', domain: 'a.pt', year: '2020', status: 'healthy',
    site_type: 'internal', match_type: 'none', ...overrides,
  }
}

describe('parseAnnivCountdownDays', () => {
  it('今天站庆/0天 → 0', () => {
    expect(parseAnnivCountdownDays('今天站庆')).toBe(0)
    expect(parseAnnivCountdownDays('0天')).toBe(0)
  })
  it('N天 → N（含站庆前缀）', () => {
    expect(parseAnnivCountdownDays('5天')).toBe(5)
    expect(parseAnnivCountdownDays('站庆 12天')).toBe(12)
  })
  it('非倒计时文案/空 → null', () => {
    expect(parseAnnivCountdownDays('建站 7 周年')).toBeNull()
    expect(parseAnnivCountdownDays('')).toBeNull()
    expect(parseAnnivCountdownDays(undefined)).toBeNull()
  })
})

describe('parseOpenDate', () => {
  it('合法 YYYY-MM-DD → 年月日', () => {
    expect(parseOpenDate('2020-03-15')).toEqual({ year: 2020, month: 3, day: 15 })
    expect(parseOpenDate('2020-3-5 10:00')).toEqual({ year: 2020, month: 3, day: 5 })
  })
  it('非法/越界日期 → null', () => {
    expect(parseOpenDate('')).toBeNull()
    expect(parseOpenDate('2020-13-01')).toBeNull()
    expect(parseOpenDate('2020-02-30')).toBeNull()
    expect(parseOpenDate('about 2020')).toBeNull()
  })
})

describe('buildAnniversaryMap', () => {
  const now = new Date(2026, 7, 25) // 2026-08-25
  it('开站日期 → 每年同月日站庆，周年数 = 查看年 - 建站年', () => {
    const map = buildAnniversaryMap([annivCard({ name: 'A站', open_time: '2020-03-15', year: '2020' })], 2026, now)
    expect(map.get('3-15')).toEqual([{ card: expect.objectContaining({ name: 'A站' }), years: 6, source: 'open_time' }])
  })
  it('无 open_time 时倒计时补位（今天+N 天），且只落在查看当年', () => {
    const map = buildAnniversaryMap([annivCard({ name: 'B站', anniversary_text: '5天' })], 2026, now)
    expect(map.get('8-30')?.[0].card.name).toBe('B站')
    expect(buildAnniversaryMap([annivCard({ name: 'B站', anniversary_text: '5天' })], 2027, now).size).toBe(0)
  })
  it('今天站庆文案 → 当天；已关站不进日历', () => {
    const map = buildAnniversaryMap([
      annivCard({ name: 'C站', anniversary_text: '今天站庆' }),
      annivCard({ name: 'D站', open_time: '2020-08-25', status: 'closed' }),
    ], 2026, now)
    expect(map.get('8-25')?.map((e) => e.card.name)).toEqual(['C站'])
  })
  it('查看年早于开站年 → 不出现；建站年未知 → 周年数 0', () => {
    expect(buildAnniversaryMap([annivCard({ name: 'E站', open_time: '2020-03-15' })], 2019, now).size).toBe(0)
    const map = buildAnniversaryMap([annivCard({ name: 'F站', open_time: '2020-03-15', year: '未知' })], 2026, now)
    expect(map.get('3-15')?.[0].years).toBe(0)
  })
  it('2/29 建站平年落 2/28，闰年落 2/29', () => {
    const cards = [annivCard({ name: 'G站', open_time: '2020-02-29' })]
    expect(buildAnniversaryMap(cards, 2026, now).get('2-28')?.length).toBe(1) // 2026 平年
    expect(buildAnniversaryMap(cards, 2028, now).get('2-29')?.length).toBe(1) // 2028 闰年
  })
  it('同名站点去重', () => {
    const map = buildAnniversaryMap([
      annivCard({ name: 'H站', open_time: '2020-03-15' }),
      annivCard({ name: 'H站', open_time: '2020-03-15', year: '2021' }),
    ], 2026, now)
    expect(map.get('3-15')?.length).toBe(1)
  })
})

describe('hasKnownAnniversaryDate', () => {
  it('有开站日期或倒计时文案 → true', () => {
    expect(hasKnownAnniversaryDate(annivCard({ open_time: '2020-03-15' }))).toBe(true)
    expect(hasKnownAnniversaryDate(annivCard({ anniversary_text: '5天' }))).toBe(true)
  })
  it('周年文案/空 → false', () => {
    expect(hasKnownAnniversaryDate(annivCard({ anniversary_text: '建站 7 周年' }))).toBe(false)
    expect(hasKnownAnniversaryDate(annivCard())).toBe(false)
  })
})
