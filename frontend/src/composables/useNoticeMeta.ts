import { useI18n } from 'vue-i18n'
import type { NoticeSource } from '@/api/notice'

// 通知来源元信息与路由映射——单源字典（W2 重构：此前 Notices.vue 与
// NotificationCenter.vue 各持一份完全相同的拷贝，改文案/加来源必然漏一边）。
// 配色改用语义 token（var(--pt-*)）：暗色主题下按各自主题对比度重映射，
// 此前 Tailwind hex 按明色对比度选色，暗色下偏暗。
export type NoticeMeta = { icon: string; color: string; label: string }

export function useNoticeMeta() {
  // useI18n() 必须在 setup 上下文内调用：此前放在模块顶层导致 Vite 代码分割后
  // 该 chunk 被动态 import() 求值时抛 I18nError(26)=MUST_BE_CALL_SETUP_TOP，
  // 导航中止整页白屏（自 1.03.22 起）。移入组合式函数内由消费组件 setup 调用。
  const { t } = useI18n()
  const sourceMeta: Record<NoticeSource, NoticeMeta> = {
    site_anniv: { icon: 'party-popper', color: 'var(--pt-warn)', label: t('notices.source.site_anniv') },
    new_mail: { icon: 'mail', color: 'var(--pt-primary-2)', label: t('notices.source.new_mail') },
    assess_pass: { icon: 'check-circle', color: 'var(--pt-success)', label: t('notices.source.assess_pass') },
    assess_fail: { icon: 'alert-circle', color: 'var(--pt-danger)', label: t('notices.source.assess_fail') },
    announce: { icon: 'megaphone', color: 'var(--pt-primary-2)', label: t('notices.source.announce') },
    host_offline: { icon: 'alert-triangle', color: 'var(--pt-danger)', label: t('notices.source.host_offline') },
    nas_offline: { icon: 'alert-triangle', color: 'var(--pt-danger)', label: t('notices.source.nas_offline') },
    camera_offline: { icon: 'alert-triangle', color: 'var(--pt-danger)', label: t('notices.source.camera_offline') },
    reseed_done: { icon: 'check-circle', color: 'var(--pt-success)', label: t('notices.source.reseed_done') },
    dl_done: { icon: 'download', color: 'var(--pt-success)', label: t('notices.source.dl_done') },
    site_offline: { icon: 'alert-triangle', color: 'var(--pt-danger)', label: t('notices.source.site_offline') },
    backup_done: { icon: 'check-circle', color: 'var(--pt-success)', label: t('notices.source.backup_done') },
    backup_import: { icon: 'check-circle', color: 'var(--pt-success)', label: t('notices.source.backup_import') },
    freeleech: { icon: 'party-popper', color: 'var(--pt-warn)', label: 'Freeleech' },
    subscribe_hit: { icon: 'check-circle', color: 'var(--pt-success)', label: t('notices.source.subscribe_hit') },
    subscribe_list_fail: {
      icon: 'alert-circle',
      color: 'var(--pt-warn)',
      label: t('notices.source.subscribe_list_fail'),
    },
    rss_list_fail: { icon: 'alert-circle', color: 'var(--pt-warn)', label: t('notices.source.rss_list_fail') },
    rss_check_fail: { icon: 'alert-circle', color: 'var(--pt-warn)', label: t('notices.source.rss_check_fail') },
    strm_list_fail: { icon: 'alert-circle', color: 'var(--pt-warn)', label: t('notices.source.strm_list_fail') },
    brushflow_list_fail: {
      icon: 'alert-circle',
      color: 'var(--pt-warn)',
      label: t('notices.source.brushflow_list_fail'),
    },
    contract_list_fail: {
      icon: 'alert-circle',
      color: 'var(--pt-warn)',
      label: t('notices.source.contract_list_fail'),
    },
    dirsync_list_fail: {
      icon: 'alert-circle',
      color: 'var(--pt-warn)',
      label: t('notices.source.dirsync_list_fail'),
    },
    brushflow: { icon: 'brushflow', color: 'var(--pt-primary-2)', label: t('notices.source.brushflow') },
    cloud: { icon: 'cloud', color: 'var(--pt-primary-2)', label: t('notices.source.cloud') },
    contract: { icon: 'contract', color: 'var(--pt-warn)', label: t('notices.source.contract') },
    mediaserver: { icon: 'mediaserver', color: 'var(--pt-primary-2)', label: t('notices.source.mediaserver') },
    workflow: { icon: 'zap', color: 'var(--pt-primary)', label: t('notices.source.workflow') },
    rss_hit: { icon: 'rss', color: 'var(--pt-primary-2)', label: t('notices.source.rss_hit') },
    webhook: { icon: 'webhook', color: 'var(--pt-primary)', label: t('notices.source.webhook') },
    transfer_done: { icon: 'check-circle', color: 'var(--pt-success)', label: t('notices.source.transfer_done') },
    transfer_fail: { icon: 'alert-circle', color: 'var(--pt-danger)', label: t('notices.source.transfer_fail') },
    subtitle_fix: { icon: 'captions', color: 'var(--pt-success)', label: t('notices.source.subtitle_fix') },
    medal_new: { icon: 'medal', color: 'var(--pt-warn)', label: t('notices.source.medal_new') },
    medal_available: { icon: 'medal', color: 'var(--pt-success)', label: t('notices.source.medal_available') },
    medal_price_drop: { icon: 'medal', color: 'var(--pt-success)', label: t('notices.source.medal_price_drop') },
    medal_price_rise: { icon: 'medal', color: 'var(--pt-warn)', label: t('notices.source.medal_price_rise') },
    medal_out_of_stock: { icon: 'medal', color: 'var(--text-tertiary)', label: t('notices.source.medal_out_of_stock') },
  }
  function meta(s: NoticeSource): NoticeMeta {
    return sourceMeta[s] ?? { ...sourceMeta.announce, label: t('notices.source.fallback') }
  }
  return { sourceMeta, meta }
}

// 点击通知体后跳转的目标路由（按来源分类），两处消费方共用。
export const noticeSourceRoute: Record<NoticeSource, string> = {
  new_mail: 'messages',
  assess_pass: 'assessment',
  assess_fail: 'assessment',
  dl_done: 'downloaders',
  reseed_done: 'reseed',
  site_anniv: 'sites',
  site_offline: 'nas-server',
  host_offline: 'nas-server',
  nas_offline: 'nas-server',
  camera_offline: 'nas-server',
  freeleech: 'freeleech',
  announce: 'announcements',
  backup_done: 'backup',
  backup_import: 'backup',
  subscribe_hit: 'subscriptions',
  subscribe_list_fail: 'subscriptions',
  rss_list_fail: 'rss',
  rss_check_fail: 'rss',
  strm_list_fail: 'cloud',
  brushflow_list_fail: 'brushflow',
  contract_list_fail: 'contract',
  dirsync_list_fail: 'transfer',
  brushflow: 'brushflow',
  cloud: 'cloud',
  contract: 'contract',
  mediaserver: 'mediaserver',
  workflow: 'workflow',
  rss_hit: 'rss',
  webhook: 'workflow',
  transfer_done: 'transfer',
  transfer_fail: 'transfer',
  subtitle_fix: 'mediaserver',
  medal_new: 'medal-wall',
  medal_available: 'medal-wall',
  medal_price_drop: 'medal-wall',
  medal_price_rise: 'medal-wall',
  medal_out_of_stock: 'medal-wall',
}
