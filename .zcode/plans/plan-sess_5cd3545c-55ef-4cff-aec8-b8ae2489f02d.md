# PTPatronus 内置 CookieCloud 服务端

目标：PTPatronus 自身充当 CookieCloud 服务端——浏览器扩展直接把 cookie 推到 PTPatronus，PTPatronus 拉下来写入站点，**不再需要单独部署 CookieCloud 容器**。沿用官方协议，标准扩展零改装可用；已确认：UUID+密码即全部鉴权；**推送即同步**。

## 一、后端（D:\PTPatronus\backend）

### 1. 协议层 `internal/cookiecloud/`（纯函数，先建）
- `ServerStore` 接口 + 内存实现（mutex + map[uuid]envelope JSON 字符串），envelope 原样存密文（保留端到端加密，不落明文）。
- `HandleGet(uuid) ([]byte, bool)`：返回 `{"encrypted": "...", "crypto_type": "..."}`；未注册 uuid → 404 语义。
- `HandleUpdate(rawBody []byte) (string, error)`：解析 `{"uuid","encrypted","crypto_type"?}`（宽松 json.Unmarshal，gzip body 由 handler 层先解压）；校验 encrypted 非空、base64 合法、长度合理（≤ 16MB）；存入 store，返回 uuid。**不在线解密**（服务端只存密文，与官方一致；拉取侧已有双格式自动识别解密）。
- 对照官方 api/app.js 语义：GET 未存在 → 404 纯文本；POST 成功 → `{"action":"done"}`。

### 2. chain 层 `internal/chain/cookiecloud.go` + 新 `cookiecloud_server.go`
- `CookieCloudSettings` 增 `Mode string`（`external` 默认 | `builtin`）；`CookieCloudSettingsInput` 同步增 `Mode`。
- builtin 模式：
  - `SaveSettings` 时若 mode=builtin 且 uuid/password 空 → 自动生成（uuid 22 位、password 22 位字母数字，与官方格式一致）；Server 字段忽略。
  - `fetchPayload`：mode=builtin 时直读内置 store（不走 HTTP）；`pushLocal`：builtin 时写入 store 并返回。
  - `HandleUpdate` 成功后**立即触发一次 `doSync`（pull 分支）**（推送即同步），复用现有 goroutine 安全结构；失败只记日志不阻塞响应（扩展仍收到 done）。
  - 新导出方法 `ServerEnvelope(uuid)` / `ServerUpdate(raw []byte) (string, error)` 供 API handler 调用（包一层 store 读写）。
- cron 不变（*/5 tick 兜底）。

### 3. API 层 `internal/api/routes_public.go` + 新 `v1/cookiecloud_server.go`
- 公开路由（浏览器扩展无登录会话，仿 webhook/LLM gateway 模式）：
  - `GET  /api/v1/cookiecloud/get/:uuid`
  - `POST /api/v1/cookiecloud/update`
- handler 逻辑：
  - CORS：直接 `c.Header("Access-Control-Allow-Origin", "*")`（扩展 origin 是 chrome-extension://<id>，白名单配不了；无 cookie 凭证，`*` 安全）。官方扩展用 text/plain 避免 preflight，gin OPTIONS 已由全局中间件兜住。
  - gzip：检查 `Content-Encoding: gzip` 则 `compress/gzip` 解压再解析（官方扩展 POST 是 gzip 过的 JSON，全栈现无人处理）。
  - POST 成功 → `{"action":"done"}`；参数错 → 400 JSON `{"action":"error","message":...}`。
  - 只响应 builtin 模式启用且 uuid 匹配的请求；external 模式下 404（不泄露端点存在）。
- 路由注册处注明与 guarded 组 `/cookiecloud`（config/sync/push）并存：guarded 组挂 `/cookiecloud` 下的 config/sync/push 子路径，公开组挂 `/cookiecloud/get/:uuid`、`/cookiecloud/update`——gin 同前缀不同方法/子路径不冲突，注册前跑一次路由打印验证。

### 4. 配置 handler `v1/cookiecloud.go`
- `cookieCloudConfigResp` 增 `mode`、`builtin_endpoint`（= `/api/v1/cookiecloud`，前端自行拼 origin）；`cookieCloudConfigReq` 增 `mode`。

### 5. 测试
- `internal/cookiecloud`：store 往返、非法 body、超长拒绝。
- `internal/chain/cookiecloud_test.go`：builtin 模式 doSync 直读 store（注入 envelope 后 pull 成功）；ServerUpdate 触发即同步。
- `internal/api/v1`：httptest 走公开路由——GET 404/200、POST gzip body done、external 模式 404、CORS 头存在。

## 二、前端（D:\PTPatronus\frontend）

### 6. `src/api/cookiecloud.ts`
- `CookieCloudConfig` 增 `mode: 'external' | 'builtin'`、`builtin_endpoint?: string`；Input 增 `mode?`。

### 7. `src/composables/useCookieCloud.ts`
- `ccForm` 增 `mode`；load/save 映射。

### 8. `src/components/settings/CookieCloudPanel.vue`
- 顶部（enabled 下方）加模式单选（`el-radio-group`）：`外部服务器` | `内置服务端`。
- builtin 模式下：
  - 隐藏 server 输入；uuid/password 变只读展示（自动生成，带「重新生成」按钮 → 确认弹窗 → 保存后新值生效、旧数据作废）。
  - 展示扩展配置指引卡片（仿 agent-secret.ts 的 ElMessageBox/复制模式）：扩展地址 `${location.origin}/api/v1/cookiecloud` + UUID + 密码，三个一键复制按钮（用 utils/clipboard.ts 的 copyText，兼容纯 HTTP 局域网）。
- external 模式 UI 不变。

## 三、验收
- `go build ./...` + `go test ./internal/cookiecloud/ ./internal/chain/ ./internal/api/...` 全绿；gofmt；行数门禁通过。
- `vue-tsc --noEmit` 通过。
- 手动链路（实现后由我在本机起 backend + 用真实扩展验证一次推送→同步）。

## 关键文件
backend/internal/cookiecloud/（协议层）、internal/chain/cookiecloud*.go、internal/api/routes_public.go、internal/api/v1/cookiecloud*.go；frontend/src/api/cookiecloud.ts、composables/useCookieCloud.ts、components/settings/CookieCloudPanel.vue

注意：内置服务端与已有外部服务器模式并存，用户可随时切换，互不破坏。