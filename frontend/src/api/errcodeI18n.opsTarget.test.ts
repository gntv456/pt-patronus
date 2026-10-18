import { describe, expect, it } from 'vitest'
import { translateErrcode } from './errcodeI18n'

// #402 两档的英文文案覆盖。
//
// 这两档是**用户自己能改**的失败，英文译文必须各自保留那一句唯一下一步：
//   ops_target_unsupported → 换一台已配 SSH 的主机 / 先给 NAS 配 SSH
//   ops_command_too_long   → 缩短命令
// 两档若被合并或漏译，用户在英文界面就只能看到中文，且失去「该改什么」。
describe('#402 运维目标不支持 / 命令太长 的英文文案', () => {
  it('ops_target_unsupported 必须说清「换已配 SSH 的主机或给 NAS 配 SSH」', () => {
    const backend = '这个运维目标不支持该操作（远程命令/终端只对已配置 SSH 的主机与 NAS 可用）：请在运维中心换一台已配置 SSH 连接的主机，或先给该 NAS 配置 SSH 后再试。原因见下方。';
    const en = translateErrcode('ops_target_unsupported', backend, 'en-US');
    expect(en).not.toBe(backend);
    expect(en).not.toContain('这个运维目标');
    expect(en).toMatch(/SSH/);
    // 两句下一步都要在：换主机，或给这台 NAS 配 SSH。
    expect(en.toLowerCase()).toMatch(/pick another host/);
    expect(en.toLowerCase()).toMatch(/configure ssh for this nas/);
  });

  it('ops_command_too_long 必须说「缩短命令」，且不许指向换主机', () => {
    const backend = '这条命令太长，没能下发到目标主机（命令原文见下方）：请缩短命令后重试。';
    const en = translateErrcode('ops_command_too_long', backend, 'en-US');
    expect(en).not.toBe(backend);
    expect(en.toLowerCase()).toContain('shorten the command');
    // 方向必须唯一：换了主机还是超长。
    expect(en.toLowerCase()).not.toContain('pick another host');
  });

  it('zh-CN 原样透传后端中文', () => {
    const backend = '这条命令太长，没能下发到目标主机（命令原文见下方）：请缩短命令后重试。';
    expect(translateErrcode('ops_command_too_long', backend, 'zh-CN')).toBe(backend);
  });

  it('两码的译文不许相同（它们的下一步不同）', () => {
    const a = translateErrcode('ops_target_unsupported', 'x', 'en-US');
    const b = translateErrcode('ops_command_too_long', 'y', 'en-US');
    expect(a).not.toBe(b);
  });
});
