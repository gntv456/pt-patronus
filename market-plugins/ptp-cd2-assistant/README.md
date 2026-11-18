# ptp-cd2-assistant（CloudDrive2 助手）

定时检查 CloudDrive2 实例：探测上传任务错误与账号失效并通知；支持重启实例、查看系统信息、提交离线下载，插件视图页提供CPU/内存/任务/速度/容量仪表盘。

## 依赖

- 需要 `clouddrive` Python 包（安装时自动安装）。
- 在配置中填写 CloudDrive2 实例的访问地址与账号密码。

## 动作

| 动作 | 说明 |
|---|---|
| `check` | 按配置检查所有实例并通知异常 |
| `status` | 查询最近一次检查结果 |
| `info` | 获取实例系统信息 |
| `restart` | 重启实例（谨慎使用） |
| `add_offline` | 提交离线下载任务（input 传 `urls` / `folder`） |

## 主要配置

- `cd2_confs`：每行一个实例，格式 `name#url#username#password`，URL 形如 `http://host:19798`。
- `keyword`：上传任务结果中含此关键字视为账号失效，默认 `账号异常`。
- `notify`：是否发送通知。
- `cron`：定时检查表达式，留空则仅手动触发（如 `*/10 * * * *`）。

Former PTPatronus built-in, now open-source market plugin.