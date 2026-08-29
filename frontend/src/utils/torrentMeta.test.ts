import { describe, expect, it } from 'vitest'
import {
  cardGroupKey,
  extractTorrentTags,
  formatTorrentSize,
  relativeTime,
  torrentGroupKey,
  volumeFactorLabel,
} from './torrentMeta'

describe('torrentMeta', () => {
  it('extracts resolution/codec/source tags', () => {
    const tags = extractTorrentTags('Ne Zha 2 2025 2160p WEB-DL H.265 HDR Atmos-Team')
    const labels = tags.map((t) => t.label)
    // 2160p 归一为 4K 卡片标签（与 STRIP/TAG_RULES 一致），分辨率 tone 仍为 res
    expect(tags.some((t) => t.tone === 'res' && /4K|2160p/i.test(t.label))).toBe(true)
    expect(labels).toContain('4K')
    expect(labels).toContain('WEB-DL')
    expect(labels).toContain('H265')
    expect(labels).toContain('HDR')
    expect(labels).toContain('Atmos')
  })

  it('groups similar releases with same tech tokens and size', () => {
    const a = cardGroupKey({
      title: 'Ne.Zha.2.2025.2160p.WEB-DL.H265-CMCT',
      size: '25.49 GB',
    })
    const b = cardGroupKey({
      title: 'Ne Zha 2 2025 2160p WEB-DL H.265-CMCT',
      size: '25.49 GB',
    })
    expect(a).toBe(b)
    const c = cardGroupKey({
      title: 'Ne Zha 2 2025 1080p WEB-DL H.265-CMCT',
      size: '25.49 GB',
    })
    expect(c).not.toBe(a)
  })

  it('builds promo labels from volume factors', () => {
    expect(volumeFactorLabel(1, 0)?.text).toBe('免费')
    expect(volumeFactorLabel(2, 0.5)?.text).toBe('2X 50%')
    expect(volumeFactorLabel(1, 1)).toBeNull()
    expect(volumeFactorLabel(undefined, undefined, true)?.text).toBe('免费')
  })

  it('formats relative time', () => {
    expect(relativeTime('3 天前')).toBe('3 天前')
    expect(relativeTime('')).toBe('')
  })

  it('normalizes group key', () => {
    expect(torrentGroupKey('Foo.2025.1080p.BluRay.x264')).toContain('foo')
    expect(torrentGroupKey('Foo.2025.1080p.BluRay.x264')).not.toMatch(/1080p|bluray|x264/i)
  })

  it('formats bare byte sizes', () => {
    expect(formatTorrentSize('7569879853')).toMatch(/GB/)
    expect(formatTorrentSize('7.05 GB')).toBe('7.05 GB')
    expect(formatTorrentSize('')).toBe('')
  })

  it('does not chip long subtitle extras', () => {
    const tags = extractTorrentTags('Moana 2016 1080p', [
      '国语粤语中字动画海洋奇缘 / 魔瓦娜 / 摩阿娜 / 莫阿娜 迪士尼系列 国台粤英四轨 10bit HEVC版本',
      'BluRay',
    ])
    const labels = tags.map((x) => x.label)
    expect(labels.some((l) => l.includes('魔瓦娜'))).toBe(false)
  })
})
