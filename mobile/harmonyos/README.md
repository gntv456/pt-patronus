# 鸿蒙（HarmonyOS）端说明

本目录用于 **HarmonyOS Next（鸿蒙 5+）** 平台的工程配置。

## 现状

- 本项目主体用 **Flutter** 开发，一个代码库覆盖 Android / iOS / macOS / Windows / Linux。
- 标准 Google Flutter SDK（含 `flutter create` 生成的 android/ios/macos/windows/linux）**不包含 ohos（鸿蒙）平台目标**。
- HarmonyOS Next 已脱离 AOSP，**不支持运行 Android 应用**，必须用 ArkTS/ArkUI 或 Flutter 的鸿蒙化分支。

## 如何启用鸿蒙端

鸿蒙端需要 **华为官方维护的 Flutter 鸿蒙化分支（`flutter_flutter` / ohos 变体）**：

1. 克隆华为 flutter 鸿蒙分支：`git clone https://gitcode.com/openharmony-sig/flutter_flutter` 并切到对应版本，设置为本地 Flutter SDK。
2. 克隆 `flutter_engine`（ohos）与 `ArkUI` 适配层。
3. 在本工程执行 `flutter create --platforms ohos .`（鸿蒙分支提供该 target），生成 `ohos/` 工程目录。
4. 用 **DevEco Studio** 打开 `ohos/` 工程运行 / 打包。

> 详见华为官方文档：https://gitcode.com/openharmony-sig/flutter_flutter 及 https://developer.huawei.com/...

## 与后端的关系

鸿蒙端与其它端一样是 **薄客户端**：调用 Go 后端 REST API（默认 `http://localhost:8088`，真机改局域网 IP），核心抓取/自动化/VIP 校验全部在后端完成。VIP 登录 cookie 可在鸿蒙端用 `flutter_inappwebview` 的 WebView 完成后上传后端（与其它端一致）。

## 落地节奏

按总体方案，首阶段客户端先验证 Android / iOS / 桌面；鸿蒙 target **配置就绪、随华为 Flutter 生态成熟再验收**。本目录将随鸿蒙适配进展补充 `ohos/` 工程与签名配置。
