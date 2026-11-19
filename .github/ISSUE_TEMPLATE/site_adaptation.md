name: 站点适配问题
description: 某个站点抓取失败/数据为 0/签到失败——站点定义选择器需要校准
labels: ["site-adaptation"]
body:
  - type: markdown
    attributes:
      value: |
        站点改版导致选择器失配是高频问题，站点定义是全实例共享热更新的——
        你的反馈会修复所有用户的同一站点。反馈前请先：
        1. **设置 → 诊断中心 → 开始体检**，确认不是自身 Cookie/网络问题；
        2. 站点页点该站的**连通检查**，把诊断结果一并贴上（Cookie 自动打码）。
  - type: input
    id: site-name
    attributes:
      label: 站点名称与地址
      description: 如「某某 PT / https://pt.example.org」（地址可脱敏域名后缀，但请保留站点可识别名）
      placeholder: "某某 PT / https://pt.xxx.org"
    validations:
      required: true
  - type: dropdown
    id: engine
    attributes:
      label: 站点引擎类型（站点添加时的定义里可见）
      options:
        - NexusPHP
        - UNIT3D
        - Gazelle
        - 馒头（M-Team API）
        - 自研/其它/不清楚
    validations:
      required: true
  - type: dropdown
    id: symptom
    attributes:
      label: 症状
      multiple: true
      options:
        - 同步后数据全是 0
        - 部分字段缺失/错误（上传量、魔力值等）
        - 签到失败
        - H&R 数据抓不到
        - 连通检查直接失败
        - 其它
    validations:
      required: true
  - type: textarea
    id: logs
    attributes:
      label: 运行日志相关段落
      description: 设置 → 运行日志 筛选该站报错（Cookie/密钥自动打码，可放心粘贴）
      render: text
    validations:
      required: true
  - type: checkboxes
    id: confirmed
    attributes:
      label: 自检确认
      options:
        - label: 已重新导入过 Cookie，问题仍在
        - label: 诊断中心体检无明显异常
        - label: 该站点最近能正常浏览器访问（非站点宕机）
    validations:
      required: true
