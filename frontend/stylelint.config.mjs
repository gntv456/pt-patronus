// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Stylelint 配置（W2：锁住 token 化存量，防止硬编码色值回流）。
// 策略分层：
//   - error：品牌主色 hex（#6c5ce7/#4285f4）已 codemod 清零，出现在 color/background/
//     border/box-shadow 任何位置直接报错防回流；
//   - warning：其余 hex（存量 ~800 处，随页面迁移逐步消化，游戏类独立视觉模块可长期豁免）。
// 执行：`npx stylelint "src/**/*.{css,vue}"`（可接入 CI / pre-commit）。
export default {
  extends: ['stylelint-config-standard'],
  ignoreFiles: ['**/node_modules/**', 'dist/**'],
  rules: {
    // —— 品牌 token 防回流 + 去 AI 感 / 可读性 / 动效纪律 ——
    // advisory 级别：硬门禁由 scripts/check-ui-a11y.mjs 棘轮负责（只许减不许增）。
    // 正则一律写成「不含反斜杠」的简单模式，避免 JS 字符串与正则两层转义踩坑。
    'declaration-property-value-disallowed-list': [
      {
        '/^color$/': ['/#6c5ce7/i', '/#4285f4/i'],
        '/^background(-image|-color)?$/': ['/#6c5ce7/i', '/#4285f4/i', '/gradient/i'],
        '/^border(-color)?$/': ['/#6c5ce7/i', '/#4285f4/i'],
        '/^box-shadow$/': ['/#6c5ce7/i', '/#4285f4/i'],
        '/^font-size$/': ['/^(9|10|11)px$/i', '/^(9|10|11)[.]5px$/i', '/^0[.]75rem$/i'],
        '/^transition$/': ['/^all/i'],
        '/^animation$/': ['/infinite/i'],
        '/^backdrop-filter$/': ['/blur/i', '/saturate/i'],
        '/^-webkit-backdrop-filter$/': ['/blur/i', '/saturate/i'],
      },
      {
        severity: 'warning',
        message:
          '品牌主色/渐变/小字号/transition:all/无限动画/装饰模糊需用语义 token 与克制样式（硬门禁见 scripts/check-ui-a11y.mjs）',
      },
    ],
    // —— 存量消化（warning）——
    'color-no-hex': [
      true,
      {
        severity: 'warning',
        message: '优先语义 token（var(--pt-*)/--surface-*/--divider 等）；游戏类独立视觉模块可豁免',
      },
    ],
    // —— 命名 ——
    'custom-property-pattern': '^([a-z][a-z0-9]*)(-[a-z0-9]+)*$',
    // —— 与现有代码风格共存（非本次目标的风格规则全部关闭/放宽）——
    'keyframes-name-pattern': null,
    'no-descending-specificity': null,
    'no-duplicate-selectors': null,
    'selector-class-pattern': null,
    'selector-pseudo-class-no-unknown': [true, { ignorePseudoClasses: ['deep', 'global', 'slotted', 'v-deep'] }],
    'selector-not-notation': null,
    'comment-empty-line-before': null,
    'declaration-empty-line-before': null,
    'rule-empty-line-before': null,
    'custom-property-empty-line-before': null,
    'at-rule-empty-line-before': null,
    'media-feature-range-notation': null,
    'at-rule-no-unknown': [
      true,
      { ignoreAtRules: ['tailwind', 'apply', 'layer', 'config', 'theme', 'utility', 'variants', 'responsive', 'screen'] },
    ],
    'declaration-block-single-line-max-declarations': null,
    'declaration-block-no-duplicate-properties': null,
    'declaration-block-no-redundant-longhand-properties': null,
    'declaration-property-value-keyword-no-deprecated': null,
    'value-keyword-case': null,
    'font-family-no-missing-generic-family-keyword': null,
    'font-family-name-quotes': null,
    'import-notation': null,
    'property-no-vendor-prefix': null,
    'value-no-vendor-prefix': null,
    'hue-degree-notation': null,
    'alpha-value-notation': null,
    'color-function-notation': null,
    'color-function-alias-notation': null,
    'length-zero-no-unit': null,
    'color-hex-length': null,
    'block-no-empty': null,
    'shorthand-property-no-redundant-values': null,
  },
  overrides: [
    {
      files: ['**/*.vue'],
      customSyntax: 'postcss-html',
    },
  ],
}
