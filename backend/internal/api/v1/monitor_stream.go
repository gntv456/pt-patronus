package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/monitor"
)

// MonitorStreamHandler 把 RTSP→WebSocket 升级转发到 monitor.RTSPProxy。
// 路由放 guarded 外（WS 无法走 header 鉴权，由 proxy 内部 ?token= 校验）。
type MonitorStreamHandler struct {
	proxy *monitor.RTSPProxy
}

// NewMonitorStreamHandler 构造。
func NewMonitorStreamHandler(proxy *monitor.RTSPProxy) *MonitorStreamHandler {
	return &MonitorStreamHandler{proxy: proxy}
}

// Stream 升级 WebSocket 并拉流转发（GET /api/v1/monitor/cameras/:id/stream?token=&url=）。
func (h *MonitorStreamHandler) Stream(c *gin.Context) {
	h.proxy.HandleWS(c)
}

// Playlist 返回摄像头 HLS 播放列表（GET /monitor/cameras/:id/hls/playlist.m3u8?token=）。
// 供移动端 media_kit 等标准播放器消费（WS/jessibuca 链路不动，Web 仍用 Stream）。
func (h *MonitorStreamHandler) Playlist(c *gin.Context) {
	h.proxy.ServePlaylist(c)
}

// Segment 返回一个 HLS ts 段（GET /monitor/cameras/:id/hls/segment/:seq?token=）。
func (h *MonitorStreamHandler) Segment(c *gin.Context) {
	h.proxy.ServeSegment(c)
}
