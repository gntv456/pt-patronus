/// 应用版本号 —— **离线兜底常量**，非权威来源。
///
/// 权威版本号在后端 `backend/internal/version/version.go`，经 `GET /system/info`
/// 暴露；前端/移动端运行时读取该值显示。本常量仅在后端不可达（离线/首次加载）时兜底，
/// 因此**不必每次升版精确同步**——正常运行时显示的是后端的值。
///
/// 唯一需要与后端保持一致的是发布构建元数据 `pubspec.yaml` 的 `version:`（影响
/// Android versionCode / iOS build），那属于发布流程，与日常开发显示版本解耦。
const String appVersion = '0.80.06';
