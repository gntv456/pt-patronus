# Jessibuca 播放器资源（说明）

> **本目录不含二进制，正常情况下你也无需操作。**
>
> 摄像头预览的解码器由**后端在首次使用时自动获取**并落盘到 `DATA_DIR/jessibuca/`，
> 由 `/jessibuca/*` 路由直出（实现见 `backend/internal/web/assets.go`）。
> 打开摄像头预览页面时若尚未就位，界面会显示「正在准备播放器…」，下载完自动开始播放。

## 为什么不随仓库 / 镜像分发

jessibuca 是 **GPL-3.0**（见其 [LICENSE](https://github.com/langhuihui/jessibuca/blob/master/LICENSE)）。把 `decoder.wasm`（约 1 MB，内含 ffmpeg 解码器）提交进公开仓库或打进对外发布的 Docker 镜像，属于 GPL 意义上的「分发」，会要求 PTPatronus 整体同样以 GPL-3.0 发布。

摄像头实时预览是可选功能，为它给整个产品套上 GPL 不划算。故改为**运行时按需获取**：

- 仓库与镜像都不含该二进制
- 镜像体积不增加
- 未就绪时其余监控能力（在线状态、掉线告警、历史快照）**全部正常**，仅实时预览不可用

## 离线 / 内网部署

运行环境无法访问 GitHub 时，把下面三个文件预先拷入 `DATA_DIR/jessibuca/`（文件名保持一致）：

| 文件 | 大小 | 作用 |
|---|---|---|
| `jessibuca.js` | 219,743 B | 播放器主体（自动识别 H.264/H.265） |
| `decoder.js` | 110,237 B | wasm 胶水 + Worker |
| `decoder.wasm` | 1,050,314 B | ffmpeg 解码器，**含 H.264 与 H.265** |

后端每次读取都校验 sha256（值登记在 `backend/internal/web/assets.go`）。也可以跑 `bash scripts/fetch-jessibuca.sh` 拉取到 `frontend/public/jessibuca/` 再手工拷过去——该脚本供 CI / 离线构建使用，**普通用户无需执行**。

版本锁定 **v3.3.28**（上游 `dist/`）。校验和同时登记在三处，由测试 `frontend/src/utils/jessibucaAssets.test.ts` 交叉校验，漂移即失败：

- `backend/internal/web/assets.go`（运行时下载与校验，权威）
- `scripts/fetch-jessibuca.sh`（离线脚本）
- `frontend/src/utils/jessibucaAssets.ts`（前端探测用的字节数）

## H.265 支持

v3.3.28 的 wasm 内含 `HEVC decoder` / `HEVCSPS`（源自 ffmpeg），播放器内置编码表 `{7:"H264(AVC)", 12:"H265(HEVC)"}`，会从 annexB NAL 头**自动判别编码**，无需前端做任何分派配置。因此小米经 Miloco/Micam 转推的默认 H.265 流可直接播放。

后端侧对应实现在 `backend/internal/monitor/video_codec.go`（H.264 优先、H.265 兜底）。

## 安全说明

- 落盘目录为 `DATA_DIR/jessibuca/`，路径由代码内的固定清单决定，不接受用户输入的路径（防路径穿越）
- 下载后先比对 sha256，通过才原子改名落盘，避免半截文件被当成「已就位」
- 同名并发请求只下载一次
- 只接受 `GET` / `HEAD`；清单外的路径返回 404 而非 SPA 的 index.html（后者会让浏览器把 HTML 当 wasm，报难以定位的解码错误）
