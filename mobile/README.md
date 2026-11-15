# PTPatronus Mobile

Flutter 客户端共用 `mobile/lib`，覆盖 Android、iOS、Windows、macOS、Linux 和 Chrome 调试。移动端默认连接用户自建的 PTPatronus 后端，不内置公共服务。

## 首次启动

1. 先启动后端 Docker 或本地后端，确认 `http://<host>:8088/api/v1/system/health` 返回 `ok`。
2. 打开客户端，在首屏填写后端地址，例如 `http://192.168.1.10:8088`。
3. 点击“测试连接”，成功后保存进入登录页。
4. 使用本地用户名 + 密码登录（至少 8 位，含字母和数字）；全新实例首次登录自动建号并成为管理员。存量好学 VIP 实例升级后，需由运维先执行 `ptpatronus reset-admin --password <新密码>` 给已有账号补密码。

移动端会持久化后端地址、登录 token 和用户信息；如迁移后端，可在登录页底部重新进入后端地址配置。

## 平台能力取舍（有意为之，不是遗漏）

以下能力**全端统一缺失**，属设计决策而非待办——PT 助手的核心交互都在前台完成，
后台常驻/本地推送/深链带来的权限成本（电池白名单、自启动引导）大于收益：

| 能力 | 现状 | 通知/替代路径 |
|---|---|---|
| 本地推送 | 无（不引 flutter_local_notifications） | 服务端通知渠道：Telegram/Bark/QQ/微信/Webhook + WS 站内横幅 |
| 后台任务/前台服务 | 无（前后台切换时断/连 WS） | 服务器侧接管保活（cookie-keepalive 小时级） |
| 深链 scheme | 无（Android 无 intent-filter、iOS 无 CFBundleURLTypes） | 手动打开应用 |

## 更新机制（2026-11-15 改造后）

客户端启动/设置页经后端中转检查 `GET /system/update/check`（platform 见
`lib/core/platform/platform_caps.dart`）。产物分发走**服务器托管**：

1. CI（flutter-release → GitHub Release → `upload-release-to-server`）把产物按规范名
   `<platform>-<版本>.<后缀>`（如 `android-1.03.31.apk`、`ohos-1.03.31.hap`）镜像到服务器
   `<DataDir>/downloads/`；
2. 服务器启动 + 每小时扫描该目录（也可管理员调 `POST /admin/update/artifacts/scan` 或
   Web 上传 `POST /admin/update/artifacts`），自动算 sha256 挂进自托管清单；
3. 各端自动拿到直链：Android 应用内 OTA（sha256 校验+安装器）；其余端外链下载
   （iOS 未签名 ipa 走 AltStore/侧载）。

管理员可在 Web 界面拖拽上传产物即时生效；历史版本文件保留可回滚。
iOS 侧载源：`mobile/altsource/`（`gen_altsource.py` 读 `config.json` 生成
`AltSource.json`，把生成文件放到任一 HTTPS 静态地址即可被 AltStore 订阅）。
注意：AltStore 请求源是**无鉴权**的普通 GET，拿不到服务器登录态——托管目录
`downloads/` 是 guarded 下发，ipa 直链走网盘公开地址（`--url` 参数），二者口径不同。

## 开发运行

```bash
cd mobile
flutter pub get
flutter run -d windows
```

常用目标：

```bash
flutter run -d android
flutter run -d ios
flutter run -d macos
flutter run -d chrome
```

也可以在构建时覆盖默认地址：

```bash
flutter run -d windows --dart-define API_BASE_URL=http://127.0.0.1:8088
```

## 常见连接问题

- 手机访问本机后端时不要填 `localhost`，应填电脑或 NAS 的局域网 IP。
- Docker 需要映射 `8088:8088`，并确保防火墙允许局域网访问。
- HTTPS 反代如果使用自签证书，移动端系统可能拒绝连接，建议使用可信证书或先用 HTTP 内网地址验证。
- Web 能登录但移动端不能登录时，先检查移动端后端地址是否缺少端口或路径中误带 `/api/v1`。

## 质量检查

```bash
flutter analyze
flutter test
```

当前 UI 与 Web 端对齐：导航分组、彩色图标、本地账号登录、修改密码、通知设置、运行日志、下载器/站点编辑等核心能力都应保持同名同序。
