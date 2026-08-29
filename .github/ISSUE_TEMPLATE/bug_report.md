name: Bug 报告
description: 报告功能异常/错误——请附版本号与诊断信息
labels: [bug]
body:
  - type: markdown
    attributes:
      value: |
        反馈前请先跑一次：**设置 → 诊断中心 → 开始体检**，并把结果一并贴上。
  - type: input
    id: version
    attributes:
      label: 版本号
      description: 设置 → 关于 页显示的版本（如 1.03.19）
      placeholder: "1.03.19"
    validations:
      required: true
  - type: dropdown
    id: client
    attributes:
      label: 出问题的端
      options:
        - Web（浏览器）
        - 移动端 APP（Android/iOS/鸿蒙/Windows/macOS）
        - 浏览器扩展
        - 服务端/部署本身
    validations:
      required: true
  - type: textarea
    id: what-happened
    attributes:
      label: 发生了什么？
      description: 具体操作步骤 → 预期结果 → 实际结果
      placeholder: |
        1. 打开 站点 → 同步
        2. 期望出现最新数据
        3. 实际：数据为 0，日志报 xxx
    validations:
      required: true
  - type: textarea
    id: logs
    attributes:
      label: 相关日志
      description: 设置 → 运行日志 里筛选报错段落（Cookie/密钥会自动打码，可放心粘贴）
      render: text
  - type: textarea
    id: doctor
    attributes:
      label: 诊断中心体检结果
      render: text
  - type: textarea
    id: env
    attributes:
      label: 部署环境
      placeholder: "如：飞牛 fnOS / 群晖 / Ubuntu + Docker，浏览器 Chrome 12x"
