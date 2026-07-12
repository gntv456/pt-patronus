import { http } from './http'

// 应用版本号 —— 离线兜底常量（构建期来自 package.json），非权威来源。
// 权威版本在后端 internal/version，经 /system/info 暴露；运行时优先读取，失败才用本兜底，故不必精确同步。
export const APP_VERSION: string = typeof __APP_VERSION__ !== 'undefined' ? __APP_VERSION__ : '0.00.02'

export interface SystemInfo {
  name: string
  // 后端未认证时不返 version（防指纹），故可选；调用方均以 ?. 守卫。
  version?: string
}

export interface UpdateCheckResult {
  hasUpdate: boolean
  currentVersion: string
  latestVersion: string
  releaseName?: string
  releaseNotes?: string
  downloadUrl?: string
  sha256?: string
  image?: string
  instructions?: string
  isForced?: boolean
  isBelowMin?: boolean
}

export const systemApi = {
  // 公开端点，免登录：登录页/关于页在认证前读取后端部署态版本。
  info: () => http.get<SystemInfo>('/system/info').then((r) => r.data),
  // 在线更新检查：platform 决定返回哪个平台的产物(docker=服务端自检，给升级命令；
  // android/windows=客户端，给下载地址)。需登录。客户端经本后端中转，不直连官方源。
  checkUpdate: (platform = 'docker') =>
    http.get<UpdateCheckResult>('/system/update/check', { params: { platform } }).then((r) => r.data),
}
