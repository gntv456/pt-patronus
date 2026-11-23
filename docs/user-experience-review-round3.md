# 用户视角复审（第三轮）· 登录后回原页

日期：2026-11-23
视角：资深用户（会话过期后的路径连续性）——只改有明确契约断裂证据的缺口。

---

## 一、已具备、不重复建设

- 登录页本身体验完整：form submit（Enter 提交）、show-password、autocomplete、aria-invalid、错误焦点管理、OIDC 回调 token 走 fragment 防日志泄漏、服务可达性自检。
- 重复提交：Element Plus loading 内部禁用按钮，15+ 保存按钮虽有 loading 无显式 disabled，但不是真缺口。
- 前两轮已补：未保存守护、放弃修改出口、按表单隔离脏状态。

## 二、核心缺口：登录后回原页契约断裂 —— 已整改

**用户场景**：在站点页操作到一半，会话过期（JWT 到期/被踢），前端跳到登录页；用户输入密码登录后——被扔回首页，得重新找回到刚才的站点页。资深用户觉得像失忆了一样。

**证据（契约只对了一半）**：
- http.ts 的 expireSession() 明确带 redirect：location.href = /login?redirect=原路径，注释写着"便于登录后回原页"。
- 但 Login.vue 与 useLoginAuth.ts 对 redirect 的引用均为 0 处——登录成功一律 router.push dashboard。发出方做了，接收方没做。

**整改**：
- 新增 safeRedirect.ts：redirect 来自 URL query（用户可控），必须校验为站内相对路径，防开放重定向（拒绝 //host、https://、反斜杠、/login 自身）。4 项单测覆盖合法路径/缺省回退/开放重定向载体/登录循环。
- Login.vue 三处登录成功路径（本地、OIDC 回调、已登录直跳）统一走 safeRedirect(route.query.redirect) + router.replace（避免返回键回到登录页）。

## 三、验收

- safeRedirect 4 项单测全过；typecheck 通过；全量测试/构建见提交记录。
- 边界：无 redirect / 非字符串 / 外部 URL / 反斜杠绕过 / 登录页循环 全部回退首页。

## 四、结论

会话过期的路径连续性现已闭环：前端发出 redirect（早已有）+ 登录页消费 redirect（本轮补）+ 开放重定向防护（本轮）。无遗留未落地项。
