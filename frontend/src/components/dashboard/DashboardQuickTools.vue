<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// 快捷工具：TOTP 两步验证码、AES-128/256 密钥、
// 网络连通性测试（多目标 ping）、种子名称识别（转 /transfer 识别测试已有入口）。
// 全部纯前端实现（TOTP 用 WebCrypto HMAC；AES 密钥 crypto.getRandomValues；连通性 fetch HEAD 计时）。
import { useI18n } from 'vue-i18n'
import { ref, computed, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { registerPolling } from '@/composables/usePolling'

const router = useRouter()

// ---- TOTP 两步验证码 ----
const totpSecret = ref('')
const totpCodes = ref<{ account: string; code: string; second: number }[]>([])
let stopTotpPolling: (() => void) | undefined

function base32Decode(s: string): Uint8Array {
  const alpha = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
  const clean = s.toUpperCase().replace(/[^A-Z2-7]/g, '')
  let bits = 0
  let value = 0
  const out: number[] = []
  for (const ch of clean) {
    const idx = alpha.indexOf(ch)
    if (idx < 0) continue
    value = (value << 5) | idx
    bits += 5
    if (bits >= 8) {
      out.push((value >>> (bits - 8)) & 0xff)
      bits -= 8
    }
  }
  return new Uint8Array(out)
}

async function totpAt(secretB32: string, counter: number): Promise<string> {
  const key = base32Decode(secretB32)
  if (!key.length) return '------'
  const msg = new Uint8Array(8)
  // 高 4 字节为 0（counter < 2^32 足够）；DataView 大端写低 4 字节
  new DataView(msg.buffer).setUint32(4, counter)
  const cryptoKey = await crypto.subtle.importKey(
    'raw',
    key.buffer as ArrayBuffer,
    { name: 'HMAC', hash: 'SHA-1' },
    false,
    ['sign'],
  )
  const sig = new Uint8Array(await crypto.subtle.sign('HMAC', cryptoKey, msg.buffer as ArrayBuffer))
  const offset = sig[sig.length - 1] & 0x0f
  const code = ((sig[offset] & 0x7f) << 24) | (sig[offset + 1] << 16) | (sig[offset + 2] << 8) | sig[offset + 3]
  return String(code % 1_000_000).padStart(6, '0')
}

async function refreshTotp() {
  const secret = totpSecret.value.trim()
  if (!secret) {
    totpCodes.value = []
    return
  }
  const counter = Math.floor(Date.now() / 1000 / 30)
  const [cur, next] = await Promise.all([
    totpAt(secret, counter),
    totpAt(secret, counter + 1),
  ])
  const second = 30 - (Math.floor(Date.now() / 1000) % 30)
  totpCodes.value = [
    { account: t('dqt.current'), code: cur, second },
    { account: t('dqt.next'), code: next, second },
  ]
}

function startTotp() {
  stopTotp()
  void refreshTotp()
  stopTotpPolling = registerPolling(() => void refreshTotp(), 1000, { runOnShow: true })
}
function stopTotp() {
  stopTotpPolling?.()
  stopTotpPolling = undefined
}
onUnmounted(stopTotp)

// ---- AES 密钥生成 ----
const aesBits = ref<256 | 128>(256)
const aesKey = ref('')
const aesIv = ref('')
function genAes() {
  const key = crypto.getRandomValues(new Uint8Array(aesBits.value / 8))
  const iv = crypto.getRandomValues(new Uint8Array(12))
  aesKey.value = btoa(String.fromCharCode(...key))
  aesIv.value = btoa(String.fromCharCode(...iv))
}
const { t } = useI18n()

function copyText(text: string) {
  if (!text) return
  navigator.clipboard.writeText(text).then(() => ElMessage.success(t('dqt.copied')))
}

// ---- 网络连通性测试 ----
const netTarget = ref('https://www.google.com')
const netResults = ref<{ url: string; ok: boolean; ms: number; err?: string }[]>([])
const DEFAULT_TARGETS = [
  'https://www.baidu.com',
  'https://api.themoviedb.org',
  'https://github.com',
  'https://pt.hdhome.org',
]
const netTesting = ref(false)

async function probe(url: string): Promise<{ url: string; ok: boolean; ms: number; err?: string }> {
  const t0 = performance.now()
  try {
    await fetch(url, { mode: 'no-cors', cache: 'no-store', signal: AbortSignal.timeout(8000) })
    return { url, ok: true, ms: Math.round(performance.now() - t0) }
  } catch (e: any) {
    return {
      url,
      ok: false,
      ms: Math.round(performance.now() - t0),
      err: e?.name === 'TimeoutError' ? t('dqt.timeout') : t('common.failed'),
    }
  }
}

async function runNet() {
  netTesting.value = true
  netResults.value = []
  const targets = netTarget.value.trim() ? [netTarget.value.trim(), ...DEFAULT_TARGETS] : DEFAULT_TARGETS
  netResults.value = await Promise.all(targets.map(probe))
  netTesting.value = false
}
</script>

<template>
  <div class="quick-tools solid-card">
    <div class="qt-title">{{ t('dqt.title') }}</div>
    <div class="qt-grid">
      <!-- TOTP -->
      <div class="qt-card">
        <div class="qt-head"><SptIcon name="lock" :size="16" aria-hidden="true" /> {{ t('dqt.totp') }}</div>
        <div class="qt-body">
          <el-input v-model="totpSecret" :placeholder="t('dqt.totpPh')" size="small" @change="startTotp" />
          <template v-if="totpCodes.length">
            <div v-for="c in totpCodes" :key="c.account" class="totp-row">
              <span class="totp-account">{{ c.account }}</span>
              <button type="button" class="totp-code qt-copy" :aria-label="t('dqt.copy')" @click="copyText(c.code)">{{ c.code }}</button>
              <span class="totp-sec">{{ c.second }}s</span>
            </div>
          </template>
          <span v-else class="qt-hint">{{ t('dqt.totpHint') }}</span>
        </div>
      </div>

      <!-- AES -->
      <div class="qt-card">
        <div class="qt-head"><SptIcon name="key" :size="16" aria-hidden="true" /> {{ t('dqt.aes') }}</div>
        <div class="qt-body">
          <el-radio-group v-model="aesBits" size="small">
            <el-radio-button :value="128">AES-128</el-radio-button>
            <el-radio-button :value="256">AES-256</el-radio-button>
          </el-radio-group>
          <el-button size="small" type="primary" @click="genAes">{{ t('dqt.generate') }}</el-button>
          <div v-if="aesKey" class="aes-out">
            <div class="aes-row">
              <span>Key</span><button type="button" class="qt-copy" :aria-label="t('dqt.copy')" @click="copyText(aesKey)">{{ aesKey }}</button>
            </div>
            <div class="aes-row">
              <span>IV</span><button type="button" class="qt-copy" :aria-label="t('dqt.copy')" @click="copyText(aesIv)">{{ aesIv }}</button>
            </div>
          </div>
          <span v-else class="qt-hint">{{ t('dqt.aesHint') }}</span>
        </div>
      </div>

      <!-- 网络测试 -->
      <div class="qt-card">
        <div class="qt-head"><SptIcon name="wifi" :size="16" aria-hidden="true" /> {{ t('dqt.net') }}</div>
        <div class="qt-body">
          <div style="display: flex; gap: 8px; width: 100%">
            <el-input v-model="netTarget" :placeholder="t('dqt.netPh')" size="small" style="flex: 1" />
            <el-button size="small" type="primary" :loading="netTesting" @click="runNet">{{ t('dqt.test') }}</el-button>
          </div>
          <div v-if="netResults.length" class="net-list">
            <div v-for="(r, i) in netResults" :key="i" class="net-row">
              <span class="net-url">{{ r.url }}</span>
              <span class="net-ms" :class="{ ok: r.ok, bad: !r.ok }">{{ r.ok ? `${r.ms}ms` : r.err }}</span>
            </div>
          </div>
          <span v-else class="qt-hint">{{ t('dqt.netHint') }}</span>
        </div>
      </div>

      <!-- 种子识别：跳转整理设置的识别测试 -->
      <div class="qt-card">
        <div class="qt-head"><SptIcon name="film" :size="16" aria-hidden="true" /> {{ t('dqt.parse') }}</div>
        <div class="qt-body">
          <el-button size="small" @click="router.push('/transfer')">{{ t('dqt.openParse') }}</el-button>
          <span class="qt-hint">{{ t('dqt.parseHint') }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.quick-tools {
  padding: var(--sp-4) var(--sp-5);
}
.qt-title {
  font-weight: 700;
  font-size: var(--fs-md);
  margin-bottom: var(--sp-3);
}
.qt-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: var(--sp-3);
}
.qt-card {
  border: 1px solid var(--divider);
  border-radius: var(--radius-md);
  padding: var(--sp-3);
  background: var(--surface-muted);
}
.qt-head {
  font-weight: 600;
  font-size: var(--fs-body);
  margin-bottom: var(--sp-2);
}
.qt-body {
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
}
.qt-hint {
  font-size: var(--fs-xs);
  color: var(--text-muted);
}
.totp-row {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
}
.totp-account {
  font-size: var(--fs-xs);
  color: var(--text-muted);
  width: 44px;
}
.totp-code {
  font-size: var(--fs-lg);
  font-weight: 700;
  letter-spacing: 2px;
  cursor: pointer;
}
.totp-sec {
  font-size: var(--fs-xs);
  color: var(--text-disabled);
  margin-left: auto;
}
.aes-out {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.aes-row {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  font-size: var(--fs-xs);
}
.aes-row span {
  color: var(--text-muted);
  flex-shrink: 0;
}
.aes-row .qt-copy {
  word-break: break-all;
  cursor: pointer;
  border: none;
  background: transparent;
  padding: 0;
  font: inherit;
  color: var(--color-text);
  text-align: left;
  min-height: 24px;
}
.qt-copy:hover { color: var(--color-primary); }
.qt-copy:focus-visible { outline-offset: 2px; }
.totp-code.qt-copy { letter-spacing: 2px; }
.net-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.net-row {
  display: flex;
  justify-content: space-between;
  gap: var(--sp-2);
  font-size: var(--fs-xs);
}
.net-url {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.net-ms.ok {
  color: var(--pt-success);
}
.net-ms.bad {
  color: var(--pt-danger);
}
</style>
