<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
defineProps<{ visible: boolean }>()
defineEmits<{ (e: 'update:visible', v: boolean): void }>()
</script>

<template>
  <el-dialog
    :model-value="visible"
    :title="t('nh.title')"
    width="620px"
    class="help"
    @update:model-value="$emit('update:visible', $event)"
  >
    <h4>{{ t('nh.pickTitle') }}</h4>
    <!-- eslint-disable-next-line vue/no-v-html -- i18n 富文本（b/code/li 结构随文案走） -->
    <p v-html="t('nh.pickServer')" />
    <!-- eslint-disable-next-line vue/no-v-html -- i18n 富文本 -->
    <ul v-html="t('nh.pickServerList')" />
    <!-- eslint-disable-next-line vue/no-v-html -- i18n 富文本 -->
    <p v-html="t('nh.pickNas')" />
    <!-- eslint-disable-next-line vue/no-v-html -- i18n 富文本 -->
    <p v-html="t('nh.pickCamera')" />

    <h4>{{ t('nh.sshTitle') }}</h4>
    <!-- eslint-disable-next-line vue/no-v-html -- i18n 富文本 -->
    <p v-html="t('nh.sshNoMethods')" />
    <!-- eslint-disable-next-line vue/no-v-html -- i18n 富文本 -->
    <ul v-html="t('nh.sshNoMethodsList')" />
    <!-- eslint-disable-next-line vue/no-v-html -- i18n 富文本 -->
    <p v-html="t('nh.sshEof')" />
    <!-- eslint-disable-next-line vue/no-v-html -- i18n 富文本 -->
    <ol v-html="t('nh.sshEofList')" />

    <h4>{{ t('nh.agentTitle') }}</h4>
    <!-- eslint-disable-next-line vue/no-v-html -- i18n 富文本 -->
    <ol v-html="t('nh.agentSteps')" />

    <h4>{{ t('nh.otherTitle') }}</h4>
    <!-- eslint-disable-next-line vue/no-v-html -- i18n 富文本 -->
    <ul v-html="t('nh.otherList')" />
  </el-dialog>
</template>

<style scoped src="../../views/nas-shared/help.css"></style>
