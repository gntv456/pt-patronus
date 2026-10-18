// @vitest-environment jsdom
// 挂载级取证（#309）：后端 monitor.detect 逐条评估 CPU/内存/磁盘/温度/离线五项阈值，
// onboarding 帮助文本（zh-CN「告警阈值（CPU / 内存 / 磁盘 / 温度 / 离线）：编辑设备调整」）
// 也一直这么写着，但设备表单里没有这五个控件 ⇒ 用户按文档找不到入口，告警永远按内置默认
// （90/90/90/75/offline 开）跑，并且改不动。这里既测序列化（纯函数层），也挂真组件
// 证明「控件里的值真能进 PUT body」。
import { afterEach, describe, expect, it } from 'vitest'
import { createApp, defineComponent, h, nextTick, ref, type Component } from 'vue'
import { createI18n } from 'vue-i18n'
import zhCN from '@/i18n/locales/zh-CN'

import DeviceForm from './DeviceForm.vue'
import { useDeviceOpsConfig } from '@/composables/useDeviceOpsConfig'

const baseTarget = {
  id: 4,
  name: '主服务器',
  host: '',
  port: 0,
  vendor: '',
  credential_id: 0,
  rtsp_url: '',
  interval_sec: 60,
  timeout_sec: 15,
  enabled: true,
  disk_paths: '',
  last_online: true,
  last_error: '',
  last_checked_at: '',
  created_at: '',
  updated_at: '',
}

const HOST_TARGET = { ...baseTarget, type: 'host', protocol: 'local', thresholds: '{"cpu_usage":50,"offline":false}' }
const HTTP_TARGET = {
  ...baseTarget,
  id: 9,
  name: '站点',
  type: 'http',
  protocol: '',
  thresholds: '{"response_ms":800}',
}

const elButton = {
  name: 'el-button',
  inheritAttrs: false, // 否则 attrs 里的 onClick 还会透传一次，点一下等于点两下
  setup(_p: unknown, ctx: { slots: Record<string, ((...args: any[]) => any) | undefined>; attrs: any }) {
    return () => h('button', { onClick: (e: any) => ctx.attrs.onClick?.(e) }, ctx.slots.default?.())
  },
}
// el-dialog 桩：默认槽 + footer 槽都要渲染（保存按钮在 footer 里）
const elDialog = {
  name: 'el-dialog',
  setup(_p: unknown, ctx: { slots: Record<string, ((...args: any[]) => any) | undefined> }) {
    return () => h('div', [ctx.slots.default?.(), ctx.slots.footer?.()])
  },
}
const passThrough = (name: string): Component => ({
  name,
  inheritAttrs: false,
  setup(_p: unknown, ctx: { slots: Record<string, ((...args: any[]) => any) | undefined> }) {
    return () => h('div', ctx.slots.default?.())
  },
})
// el-form-item 桩：真实组件会把 label 渲染出来，桩也得渲染，否则断言只看得到控件看不到名字
const elFormItem = {
  name: 'el-form-item',
  inheritAttrs: false,
  setup(_p: unknown, ctx: { slots: Record<string, ((...args: any[]) => any) | undefined>; attrs: any }) {
    return () => h('div', [String(ctx.attrs.label ?? ''), ctx.slots.default?.()])
  },
}
// v-model 桩：把 modelValue 接成 input.value，用户输入回写 onUpdate:modelValue
const elInputNumber = {
  name: 'el-input-number',
  inheritAttrs: false,
  props: ['modelValue'],
  emits: ['update:modelValue'],
  setup(props: any, ctx: { attrs: any; emit: (name: string, ...args: any[]) => void }) {
    return () =>
      h('input', {
        value: String(props.modelValue ?? ''),
        onInput: (e: any) => ctx.emit('update:modelValue', Number(e.target.value)),
      })
  },
}

const apps: { unmount: () => void }[] = []
const hosts: HTMLElement[] = []
const submitted: any[] = []

// 对话框靠 visible 的 false→true 变化才初始化表单（watch(visible)），所以先以关闭态挂载，
// 再翻成打开，模拟「用户点编辑打开对话框」。
const open = ref(false)

function mountForm(target: any) {
  const host = document.createElement('div')
  document.body.appendChild(host)
  hosts.push(host)
  open.value = false
  const app = createApp(
    defineComponent({
      setup() {
        return () =>
          h(DeviceForm, {
            visible: open.value,
            'onUpdate:visible': (v: boolean) => {
              open.value = v
            },
            target,
            onSubmit: (p: any) => submitted.push(p),
          })
      },
    }),
  )
  app.use(createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } }))
  app.component('ElDialog', elDialog as any)
  app.component('ElButton', elButton as any)
  app.component('ElInputNumber', elInputNumber as any)
  app.component('ElFormItem', elFormItem as any)
  for (const n of ['ElForm', 'ElSelect', 'ElOption', 'ElSwitch', 'ElInput', 'ElCheckbox'])
    app.component(n, passThrough(n) as any)
  app.mount(host)
  apps.push(app)
  open.value = true
  return host
}

afterEach(() => {
  while (apps.length) apps.pop()!.unmount()
  while (hosts.length) hosts.pop()!.remove()
  submitted.length = 0
})

describe('阈值序列化（纯函数层）', () => {
  it('填 0 = 交给后端内置默认，不写键', () => {
    const { thresholdCfg, buildThresholds } = useDeviceOpsConfig()
    thresholdCfg.value = { cpu_usage: 0, mem_usage: 90, disk_usage: 0, temp: 0, offline: true }
    expect(JSON.parse(buildThresholds('', 'host'))).toEqual({ mem_usage: 90 })
  })

  it('填了数字就写进 JSON；关掉离线告警写 offline:false', () => {
    const { thresholdCfg, buildThresholds } = useDeviceOpsConfig()
    thresholdCfg.value = { cpu_usage: 62, mem_usage: 0, disk_usage: 80, temp: 70, offline: false }
    expect(JSON.parse(buildThresholds('', 'host'))).toEqual({ cpu_usage: 62, disk_usage: 80, temp: 70, offline: false })
  })

  it('切走服务器/NAS 类型时不残留旧阈值键', () => {
    const { thresholdCfg, buildThresholds } = useDeviceOpsConfig()
    thresholdCfg.value = { cpu_usage: 62, mem_usage: 0, disk_usage: 0, temp: 0, offline: true }
    const out = buildThresholds('{"cpu_usage":50}', 'http')
    expect(out).not.toContain('cpu_usage')
    expect(out).not.toContain('offline')
    expect(JSON.parse(out).http_method).toBe('GET') // 该类型自己的键照旧写
  })
})

describe('设备表单阈值分组（挂载级）', () => {
  it('编辑服务器设备渲染五个阈值控件，并按存量 JSON 预填', async () => {
    const host = mountForm(HOST_TARGET)
    await nextTick()
    expect(host.textContent).toContain('CPU 阈值')
    expect(host.textContent).toContain('离线告警')
    const inputs = Array.from(host.querySelectorAll('input'))
    expect(inputs[0].value).toBe('50')
  })

  it('把 CPU 阈值改成 62 再保存，PUT body 真带上新值', async () => {
    const host = mountForm(HOST_TARGET)
    await nextTick()
    const input = host.querySelectorAll('input')[0]
    input.value = '62'
    input.dispatchEvent(new Event('input'))
    await nextTick()
    const btns = Array.from(host.querySelectorAll('button'))
    expect(btns.length, 'footer 里的取消/保存必须渲染出来').toBeGreaterThan(0)
    btns[btns.length - 1].click() // 保存
    await nextTick()
    const sent = submitted[submitted.length - 1]
    expect(sent, '必须真的 emit submit').toBeTruthy()
    expect(JSON.parse(sent.thresholds).cpu_usage).toBe(62)
    expect(JSON.parse(sent.thresholds).offline).toBe(false)
  })

  it('网站监控不渲染阈值分组（按类型收口）', async () => {
    const host = mountForm(HTTP_TARGET)
    await nextTick()
    expect(host.textContent).not.toContain('CPU 阈值')
    expect(host.textContent).not.toContain('温度阈值')
  })
})
