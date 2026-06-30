# Jessibuca 播放器资源

本目录存放摄像头 **RTSP 实时预览**所需的 [jessibuca](https://github.com/langhuihui/jessibuca) wasm 播放器资源。
浏览器无法直接播放 RTSP，由后端 `RTSPProxy` 把 RTSP 转 WebSocket、前端 jessibuca 解码渲染。

## 资源获取

从 jessibuca 官方 release 下载 demo 包，解压后把播放器文件放入本目录：

1. 访问 https://github.com/langhuihui/jessibuca/releases （开源版，活跃维护，最新 v3.3.x）
2. 下载最新版 `demo.zip`（或 dist 包）
3. 解压，将以下文件复制到本目录（`frontend/public/jessibuca/`）：
   - `jessibuca.js`
   - `decoder.js`
   - `decoder.wasm`（H.264 解码器；新版本可能为 `decoder7.wasm` / `decoder8.wasm`，按实际文件放置并在 `RtspPlayer.vue` 的 `decoder` 字段同步名称）

放置后目录结构：
```
frontend/public/jessibuca/
├── README.md        ← 本说明
├── jessibuca.js
├── decoder.js
└── decoder.wasm
```

## 后端协议（自定义，轻量）

`RtspPlayer.vue` 经 `monitorApi.streamURL(id, rtspUrl)` 连接 `ws[s]://<host>/api/v1/monitor/cameras/:id/stream?token=<jwt>&url=<rtsp>`。后端握手与帧格式：

1. 升级后发 **文本帧** 信令：`{"type":"ready","codec":"h264"}`
2. 先发 **SPS/PPS**（annexB，`00 00 00 01` 起始码）
3. 之后每个 H.264 access unit 发一帧 **二进制消息**：
   ```
   [4 字节时间戳(小端 uint32, RTP Timestamp)] [annexB NAL 序列]
   ```
4. 观众断开后保留 30s 拉流窗口，期间无新观众则关闭拉流（多观众复用单路拉流）

## 缺失时的影响

- **仅摄像头「预览」按钮不可用**，前端显示「jessibuca 资源加载失败」提示
- 服务器 / NAS 监控、摄像头在线状态、ONVIF 设备信息、告警等**全部正常**（不依赖本资源）
- 资源仅在首次点开预览时按需加载（`<script>` 动态注入），不影响首屏体积
