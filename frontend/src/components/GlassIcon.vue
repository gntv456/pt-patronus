<script setup lang="ts">
import { computed } from 'vue'

// GlassIcon — iOS 26 液态玻璃风格应用图标：圆角 squircle 渐变色块 + 顶部玻璃高光 + 白色描边字形。
// 用于导航栏、品牌 Logo、仪表盘指标卡、下载器类型徽标，替换原先的 emoji。
//
// 设计要点（对齐 iOS 26 Liquid Glass）：
//  - 大圆角 squircle（约 30%，贴近 iOS 连续圆角观感）
//  - 135° 双色渐变底（from→to），每个图标独立配色
//  - 顶部内高光 + 1px 内白边，模拟玻璃边缘折射（同 .glass::before 的 overlay 思路）
//  - 柔和外阴影，营造悬浮厚度
//  - 字形用 24×24 Lucide 风格描边路径，stroke=currentColor（白色），圆角端点
//
// 通过 unplugin-vue-components 自动注册（src/components 默认扫描目录），模板里直接用 <GlassIcon/>。

const props = withDefaults(
  defineProps<{
    /** 字形名称（见 GLYPHS 字典） */
    name: string
    /** 渐变起点色 */
    from?: string
    /** 渐变终点色 */
    to?: string
    /** 尺寸（px 或 CSS 字符串） */
    size?: number | string
  }>(),
  {
    from: '#6c5ce7',
    to: '#4285f4',
    size: 34,
  },
)

// 字形字典：name → SVG 内部标记（多条 path/rect/circle）。外层 <svg> 统一包裹。
const GLYPHS: Record<string, string> = {
  // 仪表盘：四宫格
  dashboard:
    '<rect x="3" y="3" width="7" height="7" rx="1.6"/><rect x="14" y="3" width="7" height="7" rx="1.6"/><rect x="14" y="14" width="7" height="7" rx="1.6"/><rect x="3" y="14" width="7" height="7" rx="1.6"/>',
  // 站点：地球
  sites:
    '<circle cx="12" cy="12" r="9"/><path d="M3 12h18"/><path d="M12 3c2.6 2.8 2.6 15.2 0 18"/><path d="M12 3c-2.6 2.8-2.6 15.2 0 18"/>',
  // 搜索：放大镜
  search: '<circle cx="11" cy="11" r="7"/><path d="M21 21l-4.3-4.3"/>',
  // 订阅：心形（自动追剧/追片）
  subscribe:
    '<path d="M12 20.5s-7-4.4-9.3-8.8C1.1 8.7 2.6 5.3 6 5.3c1.9 0 3.4 1.2 4 2.4.6-1.2 2.1-2.4 4-2.4 3.4 0 4.9 3.4 3.3 6.4C19 16.1 12 20.5 12 20.5z"/>',
  // 下载器：下载箭头入托盘
  downloaders:
    '<path d="M12 4v11"/><path d="M7.5 11l4.5 4.5L16.5 11"/><path d="M5 19.5h14"/>',
  // 媒体：胶片
  media:
    '<rect x="3" y="4" width="18" height="16" rx="2.2"/><path d="M7 4v16M17 4v16M3 9.5h4M3 14.5h4M17 9.5h4M17 14.5h4"/>',
  // 娱乐：手柄
  entertainment:
    '<line x1="6" y1="11" x2="10" y2="11"/><line x1="8" y1="9" x2="8" y2="13"/><line x1="15" y1="12" x2="15.01" y2="12"/><line x1="18" y1="10" x2="18.01" y2="10"/><path d="M17.32 5H6.68a4 4 0 0 0-3.98 3.59c-.01.05-.01.1-.02.15C2.6 9.42 2 14.46 2 16a3 3 0 0 0 3 3c1 0 1.5-.5 2-1l1.41-1.41A2 2 0 0 1 9.83 16h4.34a2 2 0 0 1 1.42.59L17 18c.5.5 1 1 2 1a3 3 0 0 0 3-3c0-1.54-.6-6.58-.68-7.26-.01-.05-.01-.1-.02-.15A4 4 0 0 0 17.32 5z"/>',
  // 文件：文件夹
  files:
    '<path d="M3 7a2 2 0 0 1 2-2h3.6l2 2H19a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>',
  // 备份：数据库
  backup:
    '<ellipse cx="12" cy="5" rx="8" ry="3"/><path d="M4 5v6c0 1.66 3.58 3 8 3s8-1.34 8-3V5"/><path d="M4 11v6c0 1.66 3.58 3 8 3s8-1.34 8-3v-6"/>',
  // 品牌：盾牌（ patronus 守护）
  shield:
    '<path d="M12 3l7 3v6c0 4.5-3 7.6-7 9-4-1.4-7-4.5-7-9V6z"/><path d="M9 12l2 2 4-4"/>',
  // 指标-上传
  upload: '<path d="M12 20V8"/><path d="M7.5 12L12 7.5 16.5 12"/>',
  // 指标-下载
  download: '<path d="M12 4v11"/><path d="M7.5 11l4.5 4.5L16.5 11"/><path d="M5 19.5h14"/>',
  // 指标-做种：嫩芽/叶
  seeding:
    '<path d="M11 20A7 7 0 0 1 9.8 6.1C15.5 5 17 4.48 19.2 2c1 2 1.8 4.18 1.8 8 0 5.5-4.78 10-10 10z"/><path d="M2 21c0-3 1.85-5.36 5.08-6"/>',
  // 指标-魔力：宝石
  bonus: '<path d="M12 3l8 5-8 13L4 8z"/><path d="M4 8h16"/><path d="M9.5 8L12 3l2.5 5"/>',
  // 操作-刷新：循环箭头
  refresh: '<path d="M21 12a9 9 0 1 1-2.64-6.36"/><path d="M21 3v6h-6"/>',
  // 操作-历史：时钟 + 逆时针
  history: '<path d="M3 3v5h5"/><path d="M3.05 13A9 9 0 1 0 6 5.3L3 8"/><path d="M12 7v5l4 2"/>',
  // 告警：三角警告（Cookie 失效 / H&R 危险）
  alert: '<path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z"/><path d="M12 9v4"/><path d="M12 17h.01"/>',
  // NAS 服务器：机架式服务器（双层机箱 + 指示灯）
  server:
    '<rect x="4" y="4" width="16" height="6" rx="1.5"/><rect x="4" y="14" width="16" height="6" rx="1.5"/><circle cx="7" cy="7" r="0.9"/><circle cx="7" cy="17" r="0.9"/>',
  // NAS：存储阵列（机箱 + 硬盘槽位）
  nas:
    '<rect x="3" y="5" width="18" height="14" rx="2"/><path d="M7 9h10M7 12h10M7 15h6"/>',
  // 摄像头：监控探头
  camera:
    '<path d="M2 7l4 2V7a2 2 0 0 1 2-2h9a3 3 0 0 1 3 3v4a3 3 0 0 1-3 3H8a2 2 0 0 1-2-2v-2L2 11z"/><circle cx="15" cy="9.5" r="1.4"/>',
  // 站免：礼物盒（全站免费/促销）
  freeleech:
    '<rect x="3" y="8" width="18" height="4" rx="1"/><path d="M12 8v13"/><path d="M5 12v7a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-7"/><path d="M7.5 8a2.5 2.5 0 0 1 0-5C11 3 12 8 12 8"/><path d="M16.5 8a2.5 2.5 0 0 0 0-5C13 3 12 8 12 8"/>',
  // 公告：喇叭
  announcement:
    '<path d="M3 11v2a1 1 0 0 0 1 1h2l5 4V6L6 10H4a1 1 0 0 0-1 1z"/><path d="M14 8a3 3 0 0 1 0 8"/>',
  // PT监护室：心电心跳（站点健康监护）
  savept:
    '<path d="M20.8 4.6a5.5 5.5 0 0 0-7.8 0L12 5.7l-1-1.1a5.5 5.5 0 0 0-7.8 7.8L12 21l8.8-8.6a5.5 5.5 0 0 0 0-7.8z"/><path d="M3.5 12.5h4l2-4 3 7 2-3h6"/>',
  // 后宫：人群（邀请管理）
  harem:
    '<path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/>',
  // 考核：剪贴板勾选
  assessment:
    '<rect x="8" y="3" width="8" height="4" rx="1"/><path d="M9 5H6a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7a2 2 0 0 0-2-2h-3"/><path d="M9 14l2 2 4-4"/>',
  // 消息：信封
  message:
    '<rect x="3" y="5" width="18" height="14" rx="2"/><path d="M3 7l9 6 9-6"/>',
  // 种子浏览：磁铁（种子/BT）
  seedbrowse:
    '<path d="M6 4a4 4 0 0 1 4 4v6a2 2 0 0 0 4 0V8"/><path d="M14 4a4 4 0 0 1 4 4v6a6 6 0 0 1-12 0V9"/><path d="M18 15l2.5 2.5a3.5 3.5 0 0 1-5 5L13 20"/>',
  // 推荐：火焰（热门/趋势）
  recommend:
    '<path d="M12 2c1.5 3 4 4.5 4 8a4 4 0 0 1-8 0c0-1 .5-2 1-2.5C8 9 7 10.5 7 13a5 5 0 0 0 10 0c0-4-3-7-5-11z"/>',
  // 探索：指南针（发现/探索方向）
  discover:
    '<circle cx="12" cy="12" r="9"/><path d="M15.5 8.5l-2 5-5 2 2-5z"/>',
  // 设置：滑块（调节/配置）
  settings:
    '<line x1="21" x2="14" y1="4" y2="4"/><line x1="10" x2="3" y1="4" y2="4"/><line x1="21" x2="12" y1="12" y2="12"/><line x1="8" x2="3" y1="12" y2="12"/><line x1="21" x2="16" y1="20" y2="20"/><line x1="12" x2="3" y1="20" y2="20"/><line x1="14" x2="14" y1="2" y2="6"/><line x1="8" x2="8" y1="10" y2="14"/><line x1="16" x2="16" y1="18" y2="22"/>',
  // 工作流：节点图（三节点 + 连线）
  workflow:
    '<circle cx="5" cy="6" r="2.4"/><circle cx="19" cy="6" r="2.4"/><circle cx="12" cy="18" r="2.4"/><path d="M7 6h10"/><path d="M6.5 7.8L11 16"/><path d="M17.5 7.8L13 16"/>',
  // 插件：拼图块
  plugin:
    '<path d="M8 3h4a2 2 0 0 1 2 2v2h2a2 2 0 1 1 0 4h-2v3h2a2 2 0 0 1 2 2v3H5v-3a2 2 0 0 1 2-2h2v-3H7a2 2 0 1 1 0-4h2V5a2 2 0 0 1-1-2z"/>',
  // RSS：广播信号弧
  rss:
    '<path d="M5 19a1 1 0 1 0 0-2 1 1 0 0 0 0 2z"/><path d="M4 11a7 7 0 0 1 7 7"/><path d="M4 5a13 13 0 0 1 13 13"/>',
  // 勋章墙：奖牌（圆牌 + 星 + 缎带）
  medalwall:
    '<circle cx="12" cy="9" r="6"/><path d="M9 14l-2 7 5-3 5 3-2-7"/><path d="M12 5.5l1 2 2.2.3-1.6 1.5.4 2.2L12 14l-2-1-2 1 .4-2.2L6.8 7.8 9 7.5z"/>',
}

const dim = computed(() => (typeof props.size === 'number' ? `${props.size}px` : props.size))
// 字形按 tile 尺寸的固定比例渲染（viewBox 保正方形等比缩放）。
const glyph = computed(() => {
  const inner = GLYPHS[props.name] ?? GLYPHS.shield
  return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">${inner}</svg>`
})
</script>

<template>
  <span
    class="glass-icon"
    :style="{
      width: dim,
      height: dim,
      background: `linear-gradient(135deg, ${from}, ${to})`,
    }"
    role="img"
    aria-hidden="true"
  >
    <span class="gi-rim"></span>
    <span class="gi-glyph" v-html="glyph"></span>
  </span>
</template>

<style scoped>
.glass-icon {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 30%; /* squircle 近似：iOS 连续圆角 */
  color: #fff;
  flex: none;
  /* 玻璃边缘：内白边 + 顶部高光 + 柔和外阴影 */
  box-shadow:
    inset 0 1px 0.5px rgba(255, 255, 255, 0.65),
    inset 0 0 0 1px rgba(255, 255, 255, 0.22),
    0 5px 12px rgba(31, 38, 135, 0.22);
}
/* 顶部内高光，模拟液态玻璃边缘折射 */
.gi-rim {
  position: absolute;
  inset: 0;
  border-radius: inherit;
  pointer-events: none;
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.55), rgba(255, 255, 255, 0) 45%);
  mix-blend-mode: overlay;
  opacity: 0.7;
}
/* 字形容器：绝对填满 tile，给出确定宽高，子 svg 用百分比才能正确解析居中 */
.gi-glyph {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  line-height: 0;
  filter: drop-shadow(0 1px 1px rgba(0, 0, 0, 0.18));
}
.gi-glyph :deep(svg) {
  display: block;
  width: 60%;
  height: 60%;
}
</style>
