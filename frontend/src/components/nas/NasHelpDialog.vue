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
    <p>报 <code>ssh: handshake failed: EOF</code> = 端口能连通，但对方不是正常工作的 SSH 服务（连接后未返回 SSH 标识即断开）。按顺序排查：</p>
    <ol>
      <li><b>确认端口是 SSH 端口</b>（最常见）：在服务器执行 <code>ss -tlnp | grep sshd</code> 看实际监听端口；填成 Web 面板 / 宝塔等其它服务端口就是这个报错。Docker / 端口映射环境要填映射出来的外部端口</li>
      <li><b>确认 sshd 在运行</b>：<code>systemctl status sshd</code>（Debian/Ubuntu 部分系统服务名是 <code>ssh</code>）；挂了就 <code>systemctl restart sshd</code></li>
      <li><b>检查 fail2ban 封禁</b>：反复点添加 / 探活可能触发。<code>fail2ban-client status sshd</code> 看列表，<code>fail2ban-client set sshd unbanip PTPatronus机器IP</code> 解封</li>
      <li><b>端口转发 / 反代场景</b>：前端端口活着但转发到的 SSH 后端已死也会 EOF，确认后端在线</li>
      <li><b>换个角度验证</b>：从 PTPatronus 所在机器（Docker 部署先进容器：<code>docker exec -it 容器名 sh</code>）执行 <code>ssh -v -p 端口 用户名@服务器IP</code>，能交互登录再来添加；<code>-v</code> 输出能直接看出卡在哪一步</li>
    </ol>

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
