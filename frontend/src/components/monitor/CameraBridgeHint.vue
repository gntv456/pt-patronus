<script setup lang="ts">
// CameraBridgeHint — 摄像头「采集方式」旁的接入指引。
//
// 为什么需要它：小米/米家摄像机（以及大部分消费级 IPC）官方不开放 RTSP/ONVIF，
// 码流走 P2P + 云转发且加密。用户在表单里只看到「ONVIF / RTSP 直连」两个选项，
// 却不知道自己的机器要先刷第三方固件（yi-hack）或经 Miloco/go2rtc 转一道，
// 于是填完地址探活失败、又看不出原因——这是接入这类摄像头的第一道门槛。
//
// 单独成组件而非塞进 DeviceForm.vue：主表单已 386 行，距单文件 400 行门禁很近。
import { useI18n } from 'vue-i18n'

// protocol 允许 undefined：TargetInput.protocol 是可选字段，未选时表单里为空。
// 组件内两个分支都是 v-if/v-else-if，undefined 落不到任何一支（不渲染提示），符合预期。
defineProps<{ protocol?: string }>()
const { t } = useI18n()
</script>

<template>
  <!-- RTSP：桥接方案（小米等无原生协议的设备） -->
  <el-alert v-if="protocol === 'rtsp'" type="info" :closable="false" show-icon class="cam-hint">
    <template #title>
      <div class="cam-hint-title">{{ t('cambridge.rtspTitle') }}</div>
    </template>
    <div class="cam-hint-body">
      <p>{{ t('cambridge.rtspGeneric') }}</p>
      <p>{{ t('cambridge.xiaomiLead') }}</p>
      <ol class="cam-hint-steps">
        <li>{{ t('cambridge.xiaomiStep1') }}</li>
        <li>{{ t('cambridge.xiaomiStep2') }}</li>
        <li>{{ t('cambridge.xiaomiStep3') }}</li>
      </ol>
      <p class="cam-hint-note">{{ t('cambridge.codecNote') }}</p>
      <p class="cam-hint-note">{{ t('cambridge.yiHackNote') }}</p>
    </div>
  </el-alert>

  <!-- ONVIF：提示账号密码与自动发现码流地址 -->
  <el-alert v-else-if="protocol === 'onvif'" type="info" :closable="false" show-icon class="cam-hint">
    <template #title>
      <div class="cam-hint-title">{{ t('cambridge.onvifTitle') }}</div>
    </template>
    <div class="cam-hint-body">
      <p>{{ t('cambridge.onvifBody') }}</p>
      <p class="cam-hint-note">{{ t('cambridge.onvifYiHack') }}</p>
    </div>
  </el-alert>
</template>

<style scoped>
.cam-hint {
  margin-bottom: 16px;
}
.cam-hint-title {
  font-weight: 600;
}
.cam-hint-body {
  font-size: var(--fs-xs);
  line-height: 1.6;
}
.cam-hint-body p {
  margin: 4px 0;
}
.cam-hint-steps {
  margin: 6px 0;
  padding-left: 20px;
}
.cam-hint-steps li {
  margin: 2px 0;
}
.cam-hint-note {
  color: var(--el-text-color-secondary);
}
</style>
