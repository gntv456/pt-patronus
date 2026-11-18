# ptp-ffmpeg-thumb（FFmpeg 缩略图）

定时扫描媒体库，为缺少缩略图的视频用 FFmpeg 截取指定时刻的一帧，生成 `<文件名>-thumb.jpg`。

## 依赖

- 宿主机器需要已安装 `ffmpeg` 并加入 PATH。

## 动作

| 动作 | 说明 |
|---|---|
| `scan` | 扫描并生成缺失的缩略图 |
| `status` | 查询扫描状态与进度 |

## 主要配置

- `scan_paths`：待扫描目录，每行一个，递归处理。
- `exclude_paths`：排除目录关键字，每行一个。
- `timeline`：截帧时间点，`HH:MM:SS` 格式，默认 `00:03:01`。
- `overwrite`：是否覆盖已存在的缩略图。
- `media_ext`：视频扩展名列表。
- `cron`：定时任务，留空不自动扫描（如 `0 3 * * *`）。

Former PTPatronus built-in, now open-source market plugin.