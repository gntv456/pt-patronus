# PTPatronus Mobile

Flutter 客户端共用 `mobile/lib`，覆盖 Android、iOS、Windows、macOS、Linux 和 Chrome 调试。移动端默认连接用户自建的 PTPatronus 后端，不内置公共服务。

## 首次启动

1. 先启动后端 Docker 或本地后端，确认 `http://<host>:8088/api/v1/system/health` 返回 `ok`。
2. 打开客户端，在首屏填写后端地址，例如 `http://192.168.1.10:8088`。
3. 点击“测试连接”，成功后保存进入登录页。
4. 使用本地用户名 + 密码登录（至少 8 位，含字母和数字）；全新实例首次登录自动建号并成为管理员。存量好学 VIP 实例升级后，需由运维先执行 `ptpatronus reset-admin --password <新密码>` 给已有账号补密码。

移动端会持久化后端地址、登录 token 和用户信息；如迁移后端，可在登录页底部重新进入后端地址配置。

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
