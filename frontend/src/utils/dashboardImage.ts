// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 零依赖：Canvas 2D 绘制「PTPatronus 仪表盘」成就海报，config 驱动（字段显隐/主题/脱敏/背景图/冠军 7 类）。
// 参考 PT-depiler UserDataTimeline + 旧版 Kotlin DashboardExporter。@2x 渲染，不引 html2canvas/konva。
import type { DashboardExportConfig } from './dashboardExportConfig'
import { FONT, W, PAD, type DashboardShareData } from './dashboardImageTypes'
export type { DashboardShareData, DashboardShareSite } from './dashboardImageTypes'
import {
  fmtBytes,
  fmtShort,
  nowStamp,
  roundRect,
  divider,
  ptYears,
  loadFavicon,
  loadImage,
  drawAvatar,
} from './dashboardImageHelpers'
import { themeColors, drawBackground } from './dashboardImageTheme'
import { layout } from './dashboardImageData'

// ---------- 主绘制 ----------
export async function exportDashboardImage(d: DashboardShareData, config: DashboardExportConfig): Promise<void> {
  const maskName = (s: string): string => (config.blurSite ? '***' : s)
  const maskUser = (s: string): string => (config.blurUser ? '***' : s)

  const L = layout(d, config)
  const favicons = config.blurSite ? [] : await Promise.all(L.viewSites.map((s) => loadFavicon(s.base_url)))
  const bgImg = config.backgroundImage ? await loadImage(config.backgroundImage) : null
  const hasBg = !!bgImg
  const th = themeColors(config.theme, hasBg)

  const scale = 2
  const c = document.createElement('canvas')
  c.width = W * scale
  c.height = L.totalH * scale
  const ctx = c.getContext('2d')
  if (!ctx) return
  ctx.scale(scale, scale)
  ctx.textBaseline = 'alphabetic'

  // 1. 背景
  drawBackground(ctx, W, L.totalH, config.theme, bgImg, config.backgroundBlur, config.backgroundDim, th.base)

  const when = nowStamp()

  // 2. 顶部
  ctx.fillStyle = th.text
  ctx.font = `700 26px ${FONT}`
  ctx.textAlign = 'left'
  ctx.fillText(maskUser(d.username || 'PT 玩家'), PAD, 48)
  ctx.fillStyle = th.sub
  ctx.font = `400 14px ${FONT}`
  ctx.fillText('PTPatronus · 仪表盘成就', PAD, 70)
  ctx.fillStyle = th.sub
  ctx.font = `400 13px ${FONT}`
  ctx.textAlign = 'right'
  ctx.fillText(`生成于 ${when}`, W - PAD, 36)
  ctx.fillText(`${d.siteCount} 站点 · ${d.downloaderCount} 下载器`, W - PAD, 56)
  ctx.textAlign = 'left'

  divider(ctx, PAD, W - PAD, L.headDividerY, th.divider)

  // 3. 总览：总站点（标签 + 突出大号数字）+ PT 龄
  const yrs = config.fields.ptAge ? ptYears(d.sites) : null
  ctx.textAlign = 'left'
  let sx = PAD
  ctx.fillStyle = th.sub
  ctx.font = `600 16px ${FONT}`
  ctx.fillText('总站点', sx, L.siteRowY)
  sx += ctx.measureText('总站点').width + 8
  ctx.fillStyle = th.text
  ctx.font = `700 24px ${FONT}`
  ctx.fillText(`${d.siteCount}`, sx, L.siteRowY)
  if (yrs !== null) {
    sx += ctx.measureText(`${d.siteCount}`).width + 12
    ctx.fillStyle = th.sub
    ctx.font = `600 16px ${FONT}`
    ctx.fillText(`· PT 龄 ${yrs.toFixed(1)} 年`, sx, L.siteRowY)
  }

  if (L.earliest) {
    ctx.fillStyle = th.sub
    ctx.font = `400 13px ${FONT}`
    ctx.fillText(`最早入站 ${L.earliest.date} · ${maskName(L.earliest.name)}`, PAD, L.earliestY)
  }

  if (L.numeric.length) {
    const colW = (W - PAD * 2 - 20) / 2
    L.numeric.forEach((m, i) => {
      const col = i % 2
      const row = Math.floor(i / 2)
      const mx = PAD + col * (colW + 20)
      const my = L.gridY + row * L.gridRowH
      ctx.fillStyle = m.color
      roundRect(ctx, mx, my + 5, 4, 16, 2)
      ctx.fill()
      ctx.fillStyle = th.sub
      ctx.font = `400 13px ${FONT}`
      ctx.fillText(m.label, mx + 12, my + 17)
      ctx.fillStyle = th.text
      ctx.font = `700 17px ${FONT}`
      ctx.fillText(m.value, mx + 12, my + 33)
    })
  }

  // 4. 冠军排行（7 类各冠军站点）
  if (L.champions.length) {
    divider(ctx, PAD, W - PAD, L.rankDividerY, th.divider)
    ctx.fillStyle = th.text
    ctx.font = `600 14px ${FONT}`
    ctx.fillText('各项冠军', PAD, L.rankTitleY)
    const colW = (W - PAD * 2 - 20) / 2
    L.champions.forEach((r, i) => {
      const col = i % 2
      const row = Math.floor(i / 2)
      const mx = PAD + col * (colW + 20)
      const my = L.rankGridY + row * L.rankRowH
      ctx.beginPath()
      ctx.arc(mx + 8, my + 14, 6, 0, Math.PI * 2)
      ctx.fillStyle = r.def.color
      ctx.fill()
      ctx.fillStyle = th.sub
      ctx.font = `400 12px ${FONT}`
      ctx.textAlign = 'left'
      ctx.fillText(`${r.def.label}冠军`, mx + 22, my + 12)
      ctx.fillStyle = th.text
      ctx.font = `600 14px ${FONT}`
      ctx.fillText(r.def.fmt(r.value), mx + 22, my + 29)
      ctx.fillStyle = th.muted
      ctx.font = `400 12px ${FONT}`
      ctx.textAlign = 'right'
      ctx.fillText(maskName(r.site.site_name), mx + colW, my + 29)
      ctx.textAlign = 'left'
    })
  }

  // 5. 每站明细（两列）
  if (L.viewSites.length) {
    divider(ctx, PAD, W - PAD, L.siteDividerY, th.divider)
    ctx.fillStyle = th.text
    ctx.font = `600 15px ${FONT}`
    ctx.textAlign = 'center'
    ctx.fillText(`各站点（${L.viewSites.length}）`, W / 2, L.siteTitleY)
    ctx.textAlign = 'left'

    const innerW = W - PAD * 2
    const colGap = 20
    const cellW = (innerW - colGap) / 2
    const ar = 18
    const identityH = config.showIdentity ? 18 : 0
    L.viewSites.forEach((s, i) => {
      const col = i % 2
      const row = Math.floor(i / 2)
      const cx = PAD + col * (cellW + colGap)
      const cy = L.siteGridY + row * L.SITE_ROW_H
      const displayName = maskName(s.site_name)
      drawAvatar(ctx, cx + ar, cy + ar, ar, displayName, config.blurSite ? null : favicons[i])
      const tx = cx + ar * 2 + 12
      ctx.fillStyle = th.text
      ctx.font = `600 15px ${FONT}`
      ctx.fillText(displayName, tx, cy + 14)
      if (s.level) {
        const nameW = ctx.measureText(displayName).width
        const lvX = tx + nameW + 10
        ctx.font = `400 11px ${FONT}`
        const lvW = ctx.measureText(s.level).width
        roundRect(ctx, lvX, cy + 2, lvW + 12, 16, 8)
        ctx.fillStyle = th.levelBg
        ctx.fill()
        ctx.fillStyle = th.levelText
        ctx.fillText(s.level, lvX + 6, cy + 14)
      }
      // 身份行：用户名 / ID / 邮箱（受 blurUser 脱敏 → ***）；空值跳过，best-effort
      if (identityH > 0) {
        const parts: string[] = []
        if (s.username) parts.push(maskUser(s.username))
        if (s.user_id) parts.push(`ID ${maskUser(s.user_id)}`)
        if (s.email) parts.push(maskUser(s.email))
        if (parts.length) {
          ctx.fillStyle = th.muted
          ctx.font = `400 11px ${FONT}`
          ctx.fillText(parts.join('   ·   '), tx, cy + 31)
        }
      }
      ctx.fillStyle = th.sub
      ctx.font = `400 12px ${FONT}`
      ctx.fillText(
        `↑${fmtBytes(s.uploaded)}   ↓${fmtBytes(s.downloaded)}   分享 ${s.ratio > 0 ? s.ratio.toFixed(2) : '-'}`,
        tx,
        cy + 34 + identityH,
      )
      ctx.fillText(
        `做种 ${s.seeding || 0}   下载中 ${s.leeching || 0}   体积 ${fmtBytes(s.seeding_size)}`,
        tx,
        cy + 52 + identityH,
      )
      const bh = s.bonus_per_hour > 0 ? `   时魔 ${s.bonus_per_hour.toFixed(1)}/h` : ''
      ctx.fillText(
        `发布 ${s.uploads > 0 ? s.uploads : '-'}   魔力 ${s.bonus > 0 ? Math.round(s.bonus) : 0}${bh}`,
        tx,
        cy + 70 + identityH,
      )
      const joinStr = s.join_date ? `加入 ${s.join_date}` : ''
      const updStr = s.updated_at ? `更新 ${fmtShort(s.updated_at)}` : ''
      const meta = [joinStr, updStr].filter(Boolean).join('   ')
      if (meta) {
        ctx.fillStyle = th.muted
        ctx.fillText(meta, tx, cy + 90 + identityH)
      }
    })
  }

  // 6. 页脚
  divider(ctx, PAD, W - PAD, L.footerY - 18, th.divider)
  ctx.fillStyle = th.muted
  ctx.font = `400 13px ${FONT}`
  ctx.textAlign = 'left'
  ctx.fillText('PTPatronus', PAD, L.footerY)
  ctx.textAlign = 'right'
  ctx.fillText(`客户端导出 · ${when}`, W - PAD, L.footerY)
  ctx.textAlign = 'left'

  c.toBlob((blob) => {
    if (!blob) return
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    const safeName = (d.username || 'dashboard').replace(/[^\w一-龥]/g, '')
    a.download = `ptpatronus-${safeName}-${when.replace(/[: ]/g, '-').slice(0, 16)}.png`
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
  }, 'image/png')
}
