# ptp-ai-subtitle（AI 字幕生成）

用 faster-whisper 为视频自动转录生成 `.srt` 字幕，可选通过 OpenAI 兼容接口翻译为目标语言（支持双语）。

## 依赖

- 宿主 Python 环境执行 `pip install -r requirements.txt`（含 faster-whisper），模型首次使用会自动下载。
- 有 CUDA 环境时可用 GPU；否则自动回退 CPU（较慢）。
- 翻译功能需要配置可用的 OpenAI 兼容 `base_url` 与 `api_key`。

## 动作

| 动作 | 说明 |
|---|---|
| `status` | 查询后台转录状态（运行中/进度/错误数） |
| `scan` | 递归扫描 `scan_paths` 中缺少字幕的视频并后台转录 |
| `transcribe` | 转录单个视频（input 传 `path`） |

## 主要配置

- `scan_paths`：待扫描目录，每行一个。
- `model` / `device` / `language`：模型大小、设备、识别语言。
- `media_ext`：视频扩展名。
- `translate_enabled` / `openai_base_url` / `openai_api_key` / `openai_model` / `target_lang` / `bilingual`：翻译相关。
- `cron`：定时任务，留空不自动扫描（如 `0 3 * * *`）。

Former PTPatronus built-in, now open-source market plugin.