name: 功能建议
description: 提新功能/改进想法
labels: [enhancement]
body:
  - type: textarea
    id: problem
    attributes:
      label: 你想解决什么问题？
      description: 先说场景和痛点，而不是直接说想要的方案
      placeholder: 比如：H&R 快到期但我只在周末开电脑，经常错过处理窗口
    validations:
      required: true
  - type: textarea
    id: idea
    attributes:
      label: 你的想法
  - type: dropdown
    id: area
    attributes:
      label: 主要涉及
      options:
        - 站点监控/保号（签到/H&R/Cookie）
        - 下载/辅种/订阅
        - 媒体/推荐/云盘
        - 通知/告警
        - 移动端
        - 浏览器扩展
        - 其它
