# PTPatronus Logo · 守护者吉祥物 · iOS 26 液态玻璃

以用户提供的参考图为模板，保留其全部识别元素（**圆角方形容器 + 多彩对角背景 + 居中盾牌角色 + 彩色尖帽 + 可爱笑脸 + PT 字标 + 帽顶小球**），用 iOS 26 液态玻璃语言重新诠释，并以品牌紫蓝为核心统一配色（避免原图七彩杂乱）。

> 在线预览：浏览器直接打开 [`preview.html`](./preview.html)（单文件，含尺寸阵列 / 明暗 / 动画 / 应用场景）。

## 文件清单

| 文件 | 用途 |
| --- | --- |
| [`../../frontend/public/logo.svg`](../../frontend/public/logo.svg) | **主 logo**：多彩玻璃容器 + 守护者角色（盾牌 + 尖帽 + 笑脸 + PT），1024² 矢量。用作 favicon / app icon / 品牌标。 |
| [`../../frontend/public/logo-mark.svg`](../../frontend/public/logo-mark.svg) | **透明标志**：去除容器的纯守护者角色。用于侧边栏 header 叠加、启动页、彩色背景。 |
| [`./preview.html`](./preview.html) | 设计预览展示页（多尺寸 / 明暗 / 动画 / 应用场景 mockup）。 |

favicon 已接入 [frontend/index.html](../../frontend/index.html)：
```html
<link rel="icon" href="/logo.svg" type="image/svg+xml" />
<meta name="theme-color" content="#6c5ce7" />
```

## 设计要点

- **守护者吉祥物**：盾牌＝Patronus 守护神，化身可爱角色（大眼睛、微笑），亲和又传递"守护"语义，呼应 `GlassIcon.vue` 的 `shield` 与「PT 监护室」。
- **主体撑满画幅**：盾牌占容器 ~75% 宽、角色（帽顶到盾底）占 ~85% 高，形象最大化、大气；精简掉小尺寸最先糊的腮红，保证 favicon 级别仍可辨识。
- **彩色尖帽**：模板原图的强识别点。橙→粉→紫水平彩虹渐变 + 帽顶青色小球（品牌化原图的绿球），保留活泼 DNA。
- **液态玻璃盾牌**：`feTurbulence` + `feDisplacementMap` 让盾牌轮廓液态流动。**关键技巧**：脸与 PT 字标画在 `displacement` 滤镜组之外（仅 `clip` 到盾内），所以盾牌边缘液态、而笑脸/字标保持清晰不变形。
- **玻璃体**：半透明白渐变 + 紫/蓝环境色折射叠层 + 顶部 specular 高光。
- **多彩背景**：模板四彩 DNA，调和为粉(#ff9ec7)→紫(#c08cee)→蓝(#7b7ff0)→青(#4ad4ef) 对角渐变 + 三个柔光斑，以品牌紫蓝为核心。
- **配色一致性**：主色仍为 `glass.css` 的 `--pt-primary`(#6c5ce7) / `--pt-primary-2`(#4285f4)，与导航、玻璃卡片视觉统一。

## 生成位图（按需）

SVG 为矢量母版。如需 PNG app icon / 启动图：

```bash
rsvg-convert -w 1024 -h 1024 logo.svg -o logo-1024.png
# 或用 favicon 生成器（realfavicongenerator.net）一键产出多尺寸
```

Flutter / 原生端的 `mobile/{ios,android}` 现有占位图标可替换为导出的 PNG。
