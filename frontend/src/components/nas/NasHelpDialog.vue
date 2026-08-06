<script setup lang="ts">
defineProps<{ visible: boolean }>()
defineEmits<{ (e: 'update:visible', v: boolean): void }>()
</script>

<template>
  <el-dialog
    :model-value="visible"
    title="使用说明"
    width="620px"
    class="help"
    @update:model-value="$emit('update:visible', $event)"
  >
    <h4>选哪种监控方式？</h4>
    <p><b>服务器</b></p>
    <ul>
      <li><b>本机</b>：监控 PTPatronus 所在机器，无需填地址</li>
      <li><b>SSH 远程</b>：监控其它 Linux，填 IP + 账号 + 密码</li>
      <li><b>Agent 上报</b>：被监控机装 agent 主动上报 —— <b>推荐</b>，免 SSH 认证烦恼</li>
    </ul>
    <p><b>NAS</b>：国产 NAS 推荐 Agent 上报；群晖 / TrueNAS 也可用厂商 API，支持 SNMP 的设备可用通用 SNMP</p>
    <p><b>摄像头</b>：ONVIF / RTSP</p>

    <h4>SSH 连不上？</h4>
    <p>报 <code>no supported methods remain</code> = root 被禁密码登录（OpenSSH 默认安全策略）。</p>
    <ul>
      <li>点错误旁的「如何修复」→ 复制一条命令到服务器执行（放开 root 密码）</li>
      <li>或改用 <b>Agent 上报</b>，完全绕开 SSH</li>
    </ul>

    <h4>装 Agent（最省事）</h4>
    <ol>
      <li>点「+ 添加设备」→ 选择「服务器」或「NAS」→ 采集方式「Agent 上报」→ 填名 → 保存</li>
      <li>复制弹出的一条命令</li>
      <li>SSH / 宝塔进被监控机（Linux，root），粘贴执行（自动下载 + 装 systemd + 开机自启）</li>
      <li>回本页，设备自动上线并上报指标</li>
    </ol>

    <h4>其它</h4>
    <ul>
      <li>网站 / 端口 / 进程 / 脚本监控：点「+ 添加设备」后选择对应类型</li>
      <li>采集间隔 / 超时、告警阈值（CPU / 内存 / 磁盘 / 温度 / 离线）：编辑设备调整</li>
      <li>SSH 私钥认证：编辑 SSH 设备粘贴私钥（高级，适合禁密码的服务器）</li>
    </ul>
  </el-dialog>
</template>

<style scoped src="../../views/nas-shared/help.css"></style>
