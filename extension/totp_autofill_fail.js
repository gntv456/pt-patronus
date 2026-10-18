// TOTP 自动填充失败的原因码 → 人话（单一来源，#400）。
//
// 为什么单列一个模块而不是留在 background.js 里：
//
//	credSaveFailText / credReadFailText 都在 credentials.js，那是**内容脚本也能 import 的**
//	模块（manifest 里有 web_accessible_resources），所以能直接写行为测试
//	（见 test/cred_read_fail.test.mjs）。而 background.js 是 MV3 的 service worker、
//	不是可导入模块——把分类器留在那里就只能靠源码扫描测（#336 的教训：源码里有那句
//	中文曾被当成证据，而出口照样丢）。
//
//	所以与 credentials.js 同构：分类器放可导入模块，background 只调用。
//	background.js 不 import 本模块（它本来就不能被 import 反过来），而是把这份
//	纯函数搬过去——两处会漂，所以下面有一条测试钉住两边的码集合一致。
//
// 码表与凭据侧**故意不同**：验证器库没有凭据黑名单、没有服务器侧。
// 之前 totpAutofill 借用 credReadFail('no-match')，行为上碰巧对（那张表里 no-match
// 的 message 恰好是空串），但那是两个域撞在一起的巧合：哪天给凭据的 no-match 补上
// 「去验证器页添加」之类的提示，TOTP 就会跟着冒出一句不属于它的话。
export function totpAutofillFailText(code) {
  const c = String(code || '')
  const known = {
    // 这两档要出声：用户正盯着一个不会自己填的输入框，而扩展其实有码（库在，只是锁着）
    // 或压根还没建库。静默 = 用户以为助手坏了，反复刷新/换浏览器都找不出原因（#400）。
    'no-vault': '还没建验证器库：到扩展设置页「两步验证器」里添加本站后再回来登录',
    locked: '验证器库已锁定，所以没能自动填码：到扩展设置页解锁后再回本页（PIN 只在本轮会话内有效）',
    'need-unlock': '验证器库已锁定，所以没能自动填码：到扩展设置页解锁后再回本页（PIN 只在本轮会话内有效）',
    // 正常状态：库里没有这个站。首次登录本站就是这样，不该打扰。
    'no-match': '',
    'off-site-request': '填码请求的页面与目标站点不一致，已拒绝（防跨站读取）',
    'untrusted-sender': '填码请求来源不可信，已拒绝（只有本站页面能触发填码）',
  }
  if (c in known) return known[c]
  // 未知原因：原样带出，绝不替换成某一句猜测——那正是本仓反复出的问题（#335/#337/#400）。
  return c ? `未能自动填码：${c}` : '未能自动填码：未返回原因（请打开扩展设置页查看，或刷新重试）'
}
