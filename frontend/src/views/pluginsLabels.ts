// Plugins 宿主能力标签/描述（纯 Record 映射,无依赖,从 pluginsHelpers 拆出以控制单文件行数）。
export function hostCapabilityLabel(capability: string) {
  const labels: Record<string, string> = {
    'host.runtime.external-http.command': '托管进程',
    'host.runtime.external-http.base-url': '远程 HTTP',
    'host.event.subscription': '事件订阅',
    'host.schedule.cron': 'Cron 调度',
    'host.api.config.read': '配置读取',
    'host.api.config.write': '配置写入',
    'host.api.log.write': '日志写入',
    'host.api.notice.write': '站内通知',
    'host.api.event.publish': '事件发布',
    'host.api.kv.read': 'KV 读取',
    'host.api.kv.write': 'KV 写入',
    'host.api.kv.delete': 'KV 删除',
    'host.api.site.read': '站点读取',
    'host.api.site.cookie': '站点 Cookie',
    'host.ui.view': '插件视图',
    'host.ui.bridge': '视图桥接',
  }
  return labels[capability] || capability
}

export function hostCapabilityDescription(capability: string) {
  const descriptions: Record<string, string> = {
    'host.runtime.external-http.command': '宿主负责拉起和托管外部插件进程。',
    'host.runtime.external-http.base-url': '宿主可以接入已存在的外部 HTTP 插件服务。',
    'host.event.subscription': '宿主能够把声明的事件推送给插件。',
    'host.schedule.cron': '宿主能够按 Cron 计划触发插件动作。',
    'host.api.config.read': '插件可读取宿主保存的插件配置。',
    'host.api.config.write': '插件可写回运行期配置并触发调度刷新。',
    'host.api.log.write': '插件可把运行日志写回宿主审计。',
    'host.api.notice.write': '插件可发送宿主站内通知。',
    'host.api.event.publish': '插件可向宿主事件总线发布事件。',
    'host.api.kv.read': '插件可读取私有 KV 状态。',
    'host.api.kv.write': '插件可写入私有 KV 状态。',
    'host.api.kv.delete': '插件可删除私有 KV 状态。',
    'host.api.site.read': '插件可读取已配置站点的只读信息。',
    'host.api.site.cookie': '插件可读取站点 Cookie。',
    'host.ui.view': '宿主支持渲染插件 HTML 视图。',
    'host.ui.bridge': '插件视图可通过桥接对象调用宿主 API。',
  }
  return descriptions[capability] || capability
}
