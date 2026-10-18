package v1

import (
	"errors"
	"fmt"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"io/fs"
	"net/http"
	"strings"
	"syscall"

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
		h.respondLocalErr(c, err)
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
		// 从前这里回 200 + {"stats":[]}：引擎不可达被渲染成「没有容器在跑」，与真的零读数同形
		//（2026-10-14 隔离实例实测两次都是 200 空数组，而 /local/list 同一次请求里是 503）。
		h.respondLocalErr(c, err)
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
		h.respondLocalErr(c, err)
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
		h.respondLocalErr(c, err)
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
	if err := h.local.LocalContainerAction(detachedAction(c.Request.Context()), name, action); err != nil {
		h.respondLocalErr(c, err)
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
	if err := h.local.LocalRemove(detachedAction(c.Request.Context()), name, true); err != nil {
		h.respondLocalErr(c, err)
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
	if err := h.local.LocalRemoveImage(detachedAction(c.Request.Context()), ref); err != nil {
		h.respondLocalErr(c, err)
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
	progress, err := h.local.LocalPull(detachedAction(c.Request.Context()), ref)
	if err != nil {
		h.respondLocalErr(c, err)
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
	result, err := h.local.LocalPruneResult(detachedAction(c.Request.Context()), typ)
	if err != nil {
		h.respondLocalErr(c, err)
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
	if err := h.local.LocalVolumeCreate(detachedAction(c.Request.Context()), in.Name, in.Driver); err != nil {
		h.respondLocalErr(c, err)
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
	if err := h.local.LocalVolumeRemove(detachedAction(c.Request.Context()), name, true); err != nil {
		h.respondLocalErr(c, err)
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
	if err := h.local.LocalNetworkCreate(detachedAction(c.Request.Context()), in.Name, in.Driver); err != nil {
		h.respondLocalErr(c, err)
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
	if err := h.local.LocalNetworkRemove(detachedAction(c.Request.Context()), name); err != nil {
		h.respondLocalErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// respondLocalErr 是本机 Docker 各出口的统一判据：引擎根本不在（拨不通/套接字不存在）
// 回 503 + 实际地址 + 「先启动 Docker / 设 DOCKER_HOST」，其余错误才回 502「请稍后重试」。
// 以前 12 个出口各自抄了一遍 BadGateway——面板开着时 Docker 退出，用户点「停止/删除/看日志」
// 只会拿到一句永远不会实现的「等一等」，而唯一的出路（启动引擎）一个字都没提（#240 同轴）。
func (h *DockerHandler) respondLocalErr(c *gin.Context, err error) {
	if dockerEngineUnavailable(err) {
		e := errcode.DockerEngineUnavailable.WithErr(err)
		e.Message = fmt.Sprintf("%s（实际连接地址：%s）", e.Message, h.local.endpoint())
		errcode.Respond(c, e)
		return
	}
	errcode.Respond(c, errcode.BadGateway.WithErr(err))
}

// dockerEngineUnavailable 判断本机 Docker 客户端的错误是不是「引擎根本不在」：
// 命名管道/套接字不存在、连接被拒、明确说 daemon 没跑。这些都是**重试不会自愈**的状态，
// 必须回 503 + 指名「先启动 Docker」，而不是 502「本次服务不可用，请稍后重试」。
// 「引擎答了但这次失败」（4xx/5xx、载荷解析不了）仍走 BadGateway，那条文案才谈得上重试。
func dockerEngineUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	msg := err.Error()
	low := strings.ToLower(msg)
	for _, s := range []string{
		"cannot find the file", "cannot find the path", "no such file or directory",
		"connection refused", "econnrefused", "docker daemon", "docker desktop",
		// Windows 原生运行时拨 unix 套接字的真实失败形态（2026-10-13 冒烟实测）：
		// dial unix /var/run/docker.sock: connect: A socket operation encountered a dead network.
		// 只收整句：单独的 "not supported" 会把引擎真答回来的 400「operation is not supported」
		// 也读成「引擎不在」，给已经在跑的引擎贴上「请先启动 Docker」。
		"dead network", "target machine actively refused",
		"address family not supported", "protocol not supported",
		"系统找不到指定的文件", "系统找不到指定的路径", "\\\\.\\pipe\\docker_engine",
	} {
		if strings.Contains(msg, s) || strings.Contains(low, strings.ToLower(s)) {
			return true
		}
	}
	return false
}
