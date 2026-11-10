// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// @vitest-environment jsdom
// #405 第三方 API 响应里的 URL 直接绑 href/src ⇒ javascript: 注入。
//
// safeUrl 本身早就有测试（utils/safeUrl.test.ts），缺的是**接线**：
// 保证这些位置不会又出现裸绑。审计报告（README/16「P2 安全」那条）点名过同一族，
// 而「加了 safeUrl 但漏一处」正是它反复出问题的形状。
//
// 判据走**真实渲染**（createApp + jsdom，同 DashboardHealthStrip.test.ts 的做法）
// 而不是读 SFC 源码：本仓 tsconfig 不暴露 node 类型（node:fs 一律 TS2307，
// vue-tsc 整棵树红）。且「源码里有 safeUrl」曾多次当过假证据（#336）。
//
// 注入带 javascript: 的 profile / url，看渲染出来的 href 实际是什么 ——
// 裸绑会把它原样放进 href，safeUrl 会置空。
import { describe, expect, it } from 'vitest'
import { createApp, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'
import CelebrityDialog from '@/components/CelebrityDialog.vue'
import MediaPoster from '@/components/MediaPoster.vue'

const xsProfile = {
  name: "x",
  latin_name: "X",
  avatar: "javascript:alert(1)",
  bio: ["b"],
  douban_url: "javascript:alert(document.cookie)",
  works: [{ title: "W", url: "javascript:alert(2)" }],
};

async function render(comp: any, props: Record<string, unknown>): Promise<HTMLElement> {
  // 上一轮渲染留下的节点会混进断言，先清干净（append-to-body 挂上去的也在 body 里）。
  document.body.innerHTML = '';
  const el = document.createElement("div");
  document.body.appendChild(el);
  const i18n = createI18n({ legacy: false, locale: "zh-CN", messages: { "zh-CN": zhCN } });
  const app = createApp(comp, props);
  app.use(i18n);
  app.use(ElementPlus);
  app.mount(el);
  await nextTick();
  return document.body;
}

function hrefsOf(el: HTMLElement): string[] {
  return Array.from(el.querySelectorAll('a')).map((a: HTMLAnchorElement) => a.getAttribute('href') || '');
}

describe('#405 外部 URL 不得裸绑 href/src', () => {
  it('CelebrityDialog：豆瓣链接/作品链接里的 javascript: 一律被清空', async () => {
    const el = await render(CelebrityDialog, { visible: true, loading: false, profile: xsProfile });
    const hrefs = hrefsOf(el);
    expect(hrefs.length).toBeGreaterThan(0);
    for (const h of hrefs) {
      if (h.toLowerCase().startsWith('javascript:')) throw new Error("bare href: " + h);
    }
  });

  it('MediaPoster：avatar 的 javascript: 不得成为 img 的 src', async () => {
    const el = await render(MediaPoster, { url: "javascript:alert(1)", kind: "person" });
    for (const i of Array.from(el.querySelectorAll('img'))) {
      const s = i.getAttribute('src') || '';
      if (s.toLowerCase().startsWith('javascript:')) throw new Error("bare src: " + s);
    }
  });

  it('正常 http(s) 链接不受影响（不许把真链接也清掉）', async () => {
    const el = await render(CelebrityDialog, {
      visible: true,
      loading: false,
      profile: {
        name: "x",
        latin_name: "X",
        avatar: "",
        bio: ["b"],
        douban_url: "https://movie.douban.com/celebrity/123",
        works: [{ title: "W", url: "https://example.com/w" }],
      },
    });
    const hrefs = hrefsOf(el);
    const has = (needle: string) => hrefs.some((h) => h.indexOf(needle) >= 0);
    expect(has('movie.douban.com')).toBe(true);
    expect(has('example.com')).toBe(true);
  });
});

