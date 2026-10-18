// @vitest-environment jsdom
// 渲染层回归：用户名「存过但解不开」不能显示成「无用户名」。
// 同一张卡片上的「密码 / 私钥」标记只看密文是否存在，所以主密钥换过后会出现
// 「无用户名 · 密码 · 私钥」这种自相矛盾的行，用户据此以为凭据坏了而删掉重建。
import { describe, expect, it } from 'vitest'
import { createApp, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'
import NasCredentialDrawer from './NasCredentialDrawer.vue'
import type { OpsCredential } from '@/api/monitor'

function cred(over: Partial<OpsCredential>): OpsCredential {
  return {
    id: 1,
    name: '凭据',
    type: 'ssh',
    username: '',
    has_username: false,
    has_password: false,
    has_private_key: false,
    tags: '',
    note: '',
    created_at: '',
    updated_at: '',
    ...over,
  }
}

async function render(list: OpsCredential[]): Promise<string> {
  // 每个用例从干净 body 开始：el-drawer 内容 teleport 到 body，
  // 不清掉的话上一个用例的文案会被算进本用例的断言里。
  while (document.body.firstChild) document.body.removeChild(document.body.firstChild)
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp(NasCredentialDrawer, {
    drawerVisible: true,
    credentials: list,
    formVisible: false,
    editing: null,
    form: { name: '', type: 'ssh', username: '', password: '', privateKey: '', tags: '', note: '' },
    credentialUsage: () => 0,
  })
  app.use(createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } }))
  app.use(ElementPlus)
  app.mount(el)
  await nextTick()
  // el-drawer 内容 teleport 到 body，取整页文本
  return document.body.textContent?.replace(/\s+/g, ' ') ?? ''
}

describe('凭据中心：用户名三态要分得开', () => {
  it('解不开时明说无法解密，而不是「无用户名」', async () => {
    const text = await render([cred({ name: '换密钥后', username: '', has_username: true, has_password: true })])
    expect(text).toContain('用户名无法解密')
    expect(text).not.toContain('无用户名')
  })

  it('真没填用户名仍说「无用户名」', async () => {
    const text = await render([cred({ name: '只填密码', has_username: false, has_password: true })])
    expect(text).toContain('无用户名')
    expect(text).not.toContain('无法解密')
  })

  it('读得出来时直接显示用户名', async () => {
    const text = await render([cred({ name: '正常', username: 'deploy', has_username: true })])
    expect(text).toContain('deploy')
    expect(text).not.toContain('无用户名')
  })
})
