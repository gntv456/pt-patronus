import { describe, expect, it } from 'vitest'
import { translateErrcode } from './errcodeI18n'

// #398 site_credential_unreadable 的 i18n 覆盖。
//
// translateErrcode 只在 en-US 时查表（中文界面原样透传后端中文，那是有意设计，
// 见 #355）。因此这一档漏了英文译文的后果是：英文界面显示一段中文，
// 而这段中文恰恰是**唯一**说明「本次没向站点发请求」的线索 ——
// 漏译会让用户以为站点那边出了问题、去站点页反复检查。
describe('#398 站点凭据读不出来的英文文案', () => {
  it('en-US 必须查得到译文，且译文要保留「没向站点发请求」与「重存凭据」两个要点', () => {
    const backend = '这个站点的凭据读不出来（可能更换过主密钥 SECRET_KEY）：本次没有向站点发请求，站点那边没有发生任何事。请到「站点」里重新保存该站点的凭据后再试。原因见下方。';
    const en = translateErrcode('site_credential_unreadable', backend, 'en-US');
    // 查不到表就会回落后端中文 —— 那正是这条断言要挡的。
    expect(en).not.toBe(backend);
    expect(en).not.toContain('这个站点');
    // 两个要点：用户据此判断能否放心重试、以及下一步做什么。
    expect(en).toContain('No request was sent to the site');
    expect(en.toLowerCase()).toContain('re-save');
  });

  it('zh-CN 原样透传后端中文（不被前端翻译磨掉具体原因）', () => {
    const backend = '站点「幻月府」的Cookie 无法解密（主密钥可能已更换）——请到「站点」里重新保存该站点凭据；本次未向站点发请求';
    expect(translateErrcode('site_credential_unreadable', backend, 'zh-CN')).toBe(backend);
  });

  it('未知码仍原样透传（不许因这次加表而改变兜底行为）', () => {
    const backend = '某条后端自带的原因';
    expect(translateErrcode('some_code_nobody_registered', backend, 'en-US')).toBe(backend);
  });
});
