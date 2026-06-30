import { h } from 'vue'
import { ElMessageBox } from 'element-plus'
import { copyText } from './clipboard'

// 检测 SSH 认证类错误（root 密码被服务器 PermitRootLogin prohibit-password 拒绝等典型场景）。
export function isSSHAuthError(msg?: string | null): boolean {
  if (!msg) return false
  return msg.includes('unable to authenticate') || msg.includes('no supported methods remain')
}

// 放开 root 密码登录（一次性）：执行后填 root + 密码即可连。
export const SSH_ROOT_LOGIN_FIX_CMD =
  "sed -i 's/^#*PermitRootLogin.*/PermitRootLogin yes/' /etc/ssh/sshd_config && systemctl restart sshd"

// 弹出修复引导对话框（含可一键复制的命令 + fail2ban/私钥备选说明）。
export function showSSHAuthFix() {
  ElMessageBox({
    title: 'root 密码登录被服务器拒绝',
    type: 'warning',
    confirmButtonText: '复制修复命令',
    cancelButtonText: '关闭',
    showCancelButton: true,
    message: h('div', { style: 'line-height:1.7; font-size:13px' }, [
      h(
        'p',
        null,
        '这台服务器禁止了 root 密码登录（OpenSSH 默认 PermitRootLogin prohibit-password），所以填 root + 密码连不上。这是服务器安全策略，任何客户端都绕不过。',
      ),
      h('p', { style: 'margin-top:10px' }, '在服务器上（如宝塔终端）执行下面一条命令放开，之后填 root + 密码即可：'),
      h(
        'pre',
        {
          style:
            'background: var(--glass-bg); padding:8px 10px; border-radius:6px; overflow-x:auto; font-size:12px; white-space:pre-wrap; word-break:break-all',
        },
        SSH_ROOT_LOGIN_FIX_CMD,
      ),
      h(
        'p',
        { style: 'margin-top:10px; color: var(--text-muted); font-size:12px' },
        '⚠️ 放开 root 密码有被暴力破解风险，建议同时装 fail2ban：apt install -y fail2ban',
      ),
      h(
        'p',
        { style: 'margin-top:6px; color: var(--text-muted); font-size:12px' },
        '若必须用 root 又不想放开密码，可改用 SSH 私钥认证（编辑设备 → 私钥框粘贴）。',
      ),
    ]),
  })
    .then(() => copyText(SSH_ROOT_LOGIN_FIX_CMD, '命令已复制，去服务器粘贴执行后重试', '复制失败，请手动选中上方命令复制'))
    .catch(() => {})
}
