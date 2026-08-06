import type { ExportTheme } from './dashboardExportConfig'
import type { ThemeColors } from './dashboardImageTypes'

export function themeColors(theme: ExportTheme, hasBg: boolean): ThemeColors {
  if (hasBg || theme === 'glass') {
    // 有背景图：白字 + 暗遮罩保证可读
    return {
      text: '#ffffff',
      sub: 'rgba(255,255,255,0.72)',
      muted: 'rgba(255,255,255,0.5)',
      divider: 'rgba(255,255,255,0.16)',
      levelBg: 'rgba(120,160,255,0.28)',
      levelText: '#cdd9ff',
      base: '#0f0f1e',
    }
  }
  if (theme === 'light') {
    return {
      text: '#2c2c2c',
      sub: '#5a6072',
      muted: '#9aa0b4',
      divider: 'rgba(0,0,0,0.1)',
      levelBg: 'rgba(80,120,200,0.14)',
      levelText: '#3a6fd8',
      base: '#ffffff',
    }
  }
  return {
    text: '#ffffff',
    sub: '#9aa0b4',
    muted: '#6b7088',
    divider: 'rgba(255,255,255,0.1)',
    levelBg: 'rgba(120,160,255,0.18)',
    levelText: '#6ea8fe',
    base: '#0f0f1e',
  }
}

export function drawBackground(
  ctx: CanvasRenderingContext2D,
  w: number,
  h: number,
  theme: ExportTheme,
  bgImg: HTMLImageElement | null,
  blur: number,
  dim: number,
  base: string,
): void {
  ctx.fillStyle = base
  ctx.fillRect(0, 0, w, h) // 兜底，防 blur 边缘漏白
  if (bgImg && bgImg.width > 0) {
    ctx.save()
    if (blur > 0) {
      try {
        ctx.filter = `blur(${blur}px)`
      } catch {
        /* 不支持 filter 则清晰绘制 */
      }
    }
    const s = Math.max(w / bgImg.width, h / bgImg.height) * 1.12 // 放大 12% 防 blur 边缘
    const dw = bgImg.width * s
    const dh = bgImg.height * s
    ctx.drawImage(bgImg, (w - dw) / 2, (h - dh) / 2, dw, dh)
    ctx.restore()
    ctx.filter = 'none'
    if (dim > 0) {
      ctx.fillStyle = `rgba(0,0,0,${dim})`
      ctx.fillRect(0, 0, w, h)
    }
    return
  }
  const bg = ctx.createLinearGradient(0, 0, w, h)
  if (theme === 'light') {
    bg.addColorStop(0, '#F5F5F5')
    bg.addColorStop(1, '#FFFFFF')
  } else {
    bg.addColorStop(0, '#1a1a2e')
    bg.addColorStop(1, '#0f0f1e')
  }
  ctx.fillStyle = bg
  ctx.fillRect(0, 0, w, h)
}
