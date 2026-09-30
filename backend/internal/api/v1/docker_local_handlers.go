package v1

import (
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"net/http"

	"github.com/gin-gonic/gin"
)

// --- DockerHandler 本机模式 handler 层 ---
// 以下方法接收 HTTP 请求，委托给 h.local（DockerLocalClient）执行，
// 与远程 SSH handler（docker.go）一一对应，但走 Docker Engine API 而非 SSH。

// localDockerStatus 是本机 Docker 可用性的探测响应。
type localDockerStatus struct {
	Available bool   `json:"available"`
	Endpoint  string `json:"endpoint,omitempty"`
	Error     string `json:"error,omitempty"`
}

// LocalStatus 探测本机 Docker Engine 是否可连接（前端据此显示/隐藏"本机模式"）。
func (h *DockerHandler) LocalStatus(c *gin.Context) {
	if h.local == nil {
		c.JSON(http.StatusOK, localDockerStatus{Available: false, Error: "local docker client not initialized"})
		return
	}
	endpoint, probeErr := h.local.Probe()
	avail := probeErr == nil
	st := localDockerStatus{Available: avail, Endpoint: endpoint}
	if !avail {
		st.Error = probeErr.Error()
	}
	c.JSON(http.StatusOK, st)
}

// LocalList 列出本机所有容器/镜像/卷/网络。
func (h *DockerHandler) LocalList(c *gin.Context) {
	if h.local == nil {
		errcode.Respond(c, errcode.DockerLocalNotReady)
		return
	}
	resp, err := h.local.LocalList(c.Request.Context())
	if err != nil {
		errcode.Respond(c, errcode.BadGateway.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, resp)
}

// LocalStats 返回本机所有容器的实时资源占用。
func (h *DockerHandler) LocalStats(c *gin.Context) {
	if h.local == nil {
		errcode.Respond(c, errcode.DockerLocalNotReady)
		return
	}
	rows, err := h.local.LocalStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"stats": []dockerStatsRow{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"stats": rows})
}

// LocalInspect 查看本机某容器的详情。
func (h *DockerHandler) LocalInspect(c *gin.Context) {
	if h.local == nil {
		errcode.Respond(c, errcode.DockerLocalNotReady)
		return
	}
	name := c.Param("name")
	if name == "" {
		errcode.Respond(c, errcode.DockerContainerNameRequired)
		return
	}
	ct, err := h.local.LocalInspect(c.Request.Context(), name)
	if err != nil {
		errcode.Respond(c, errcode.BadGateway.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, ct)
}

// LocalLogs 获取本机某容器日志。
func (h *DockerHandler) LocalLogs(c *gin.Context) {
	if h.local == nil {
		errcode.Respond(c, errcode.DockerLocalNotReady)
		return
	}
	name := c.Param("name")
	if name == "" {
		errcode.Respond(c, errcode.DockerContainerNameRequired)
		return
	}
	tailStr := c.DefaultQuery("tail", "200")
	tail := 200
	if n, err := strconvAtoi(tailStr); err == nil && n > 0 {
		tail = n
	}
	// 四轮 P1 修复：tail 夹到 [1,10000]——超大值（如 999999999）会让 docker logs
	// 输出无界进内存（大日志容器单行 8KB 头部开销）。
	if tail > 10000 {
		tail = 10000
	}
	logs, err := h.local.LocalLogs(c.Request.Context(), name, tail)
	if err != nil {
		errcode.Respond(c, errcode.BadGateway.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"logs": logs})
}

// LocalContainerAction 执行本机容器生命周期操作（start/stop/restart/pause/unpause）。
func (h *DockerHandler) LocalContainerAction(c *gin.Context) {
	if h.local == nil {
		errcode.Respond(c, errcode.DockerLocalNotReady)
		return
	}
	name := c.Param("name")
	action := c.Param("action")
	if name == "" || action == "" {
		errcode.Respond(c, errcode.FieldRequired)
		return
	}
	// 白名单校验，防止任意 action 注入。
	switch action {
	case "start", "stop", "restart", "pause", "unpause":
	default:
		errcode.Respond(c, errcode.InvalidRequest)
		return
	}
	if err := h.local.LocalContainerAction(c.Request.Context(), name, action); err != nil {
		errcode.Respond(c, errcode.BadGateway.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// LocalRemove 删除本机容器（DELETE via action 路由未覆盖，复用 ContainerAction 风格）。
// 注意：路由表里 /local 下没有单独的 remove 容器路由；删除走 remote Remove。
// 这里保留方法以备前端 local 模式删除容器所需。
func (h *DockerHandler) LocalRemove(c *gin.Context) {
	if h.local == nil {
		errcode.Respond(c, errcode.DockerLocalNotReady)
		return
	}
	name := c.Param("name")
	if name == "" {
		errcode.Respond(c, errcode.DockerContainerNameRequired)
		return
	}
	var in dockerActionRequest
	_ = c.ShouldBindJSON(&in)
	if !in.ConfirmRisk {
		errcode.Respond(c, errcode.RiskConfirmationRequired)
		return
	}
	if err := h.local.LocalRemove(c.Request.Context(), name, true); err != nil {
		errcode.Respond(c, errcode.BadGateway.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// LocalRemoveImage 删除本机镜像。
func (h *DockerHandler) LocalRemoveImage(c *gin.Context) {
	if h.local == nil {
		errcode.Respond(c, errcode.DockerLocalNotReady)
		return
	}
	ref := c.Query("ref")
	if ref == "" {
		errcode.Respond(c, errcode.DockerImageRefRequired)
		return
	}
	var in dockerActionRequest
	_ = c.ShouldBindJSON(&in)
	if !in.ConfirmRisk {
		errcode.Respond(c, errcode.RiskConfirmationRequired)
		return
	}
	if err := h.local.LocalRemoveImage(c.Request.Context(), ref); err != nil {
		errcode.Respond(c, errcode.BadGateway.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// LocalPull 拉取镜像（本机模式）。
func (h *DockerHandler) LocalPull(c *gin.Context) {
	if h.local == nil {
		errcode.Respond(c, errcode.DockerLocalNotReady)
		return
	}
	ref := c.Query("ref")
	if ref == "" {
		errcode.Respond(c, errcode.DockerImageRefRequired)
		return
	}
	progress, err := h.local.LocalPull(c.Request.Context(), ref)
	if err != nil {
		errcode.Respond(c, errcode.BadGateway.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "progress": progress})
}

// LocalPrune 清理无用资源（本机模式）。?type=container|image|volume|network。
// 四轮 P1 修复：prune 是批量破坏性操作（volume prune 会删全部未使用卷，匿名卷常
// 挂数据库），与远程版对齐要求 ConfirmRisk 二次确认。
func (h *DockerHandler) LocalPrune(c *gin.Context) {
	if h.local == nil {
		errcode.Respond(c, errcode.DockerLocalNotReady)
		return
	}
	var in dockerActionRequest
	_ = c.ShouldBindJSON(&in)
	if !in.ConfirmRisk {
		errcode.Respond(c, errcode.RiskConfirmationRequired)
		return
	}
	typ := c.DefaultQuery("type", "image")
	result, err := h.local.LocalPruneResult(c.Request.Context(), typ)
	if err != nil {
		errcode.Respond(c, errcode.BadGateway.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "result": result})
}

// LocalVolumeCreate 创建卷（本机模式）。
func (h *DockerHandler) LocalVolumeCreate(c *gin.Context) {
	if h.local == nil {
		errcode.Respond(c, errcode.DockerLocalNotReady)
		return
	}
	var in struct {
		Name   string `json:"name" binding:"required"`
		Driver string `json:"driver"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if err := h.local.LocalVolumeCreate(c.Request.Context(), in.Name, in.Driver); err != nil {
		errcode.Respond(c, errcode.BadGateway.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// LocalVolumeRemove 删除卷（本机模式）。四轮 P1 修复：卷=持久数据（匿名卷常挂
// 数据库），删除不可恢复，与容器/镜像删除对齐要求 ConfirmRisk。
func (h *DockerHandler) LocalVolumeRemove(c *gin.Context) {
	if h.local == nil {
		errcode.Respond(c, errcode.DockerLocalNotReady)
		return
	}
	name := c.Param("name")
	if name == "" {
		errcode.Respond(c, errcode.FieldRequired)
		return
	}
	var in dockerActionRequest
	_ = c.ShouldBindJSON(&in)
	if !in.ConfirmRisk {
		errcode.Respond(c, errcode.RiskConfirmationRequired)
		return
	}
	if err := h.local.LocalVolumeRemove(c.Request.Context(), name, true); err != nil {
		errcode.Respond(c, errcode.BadGateway.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// LocalNetworkCreate 创建网络（本机模式）。
func (h *DockerHandler) LocalNetworkCreate(c *gin.Context) {
	if h.local == nil {
		errcode.Respond(c, errcode.DockerLocalNotReady)
		return
	}
	var in struct {
		Name   string `json:"name" binding:"required"`
		Driver string `json:"driver"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if err := h.local.LocalNetworkCreate(c.Request.Context(), in.Name, in.Driver); err != nil {
		errcode.Respond(c, errcode.BadGateway.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// LocalNetworkRemove 删除网络（本机模式）。
func (h *DockerHandler) LocalNetworkRemove(c *gin.Context) {
	if h.local == nil {
		errcode.Respond(c, errcode.DockerLocalNotReady)
		return
	}
	name := c.Param("name")
	if name == "" {
		errcode.Respond(c, errcode.FieldRequired)
		return
	}
	var inReq dockerActionRequest
	_ = c.ShouldBindJSON(&inReq)
	if !inReq.ConfirmRisk {
		errcode.Respond(c, errcode.RiskConfirmationRequired)
		return
	}
	if err := h.local.LocalNetworkRemove(c.Request.Context(), name); err != nil {
		errcode.Respond(c, errcode.BadGateway.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
