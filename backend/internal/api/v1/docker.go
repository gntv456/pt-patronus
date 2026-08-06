package v1

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/monitor"
)

// DockerCommandRunner 是 DockerHandler 依赖的最小命令执行接口。
// *monitor.Service 天然实现它；测试中可注入 mock 解耦 SSH。
type DockerCommandRunner interface {
	RunCommandNow(ctx context.Context, id uint, command string, timeoutSec int) (monitor.CommandResult, error)
}

// DockerHandler 在远程 SSH 主机上管理 Docker 容器。
// 不直接调用本机 docker CLI——而是经 monitor.Service.RunCommandNow 在被控机上执行，
// 复用已有 SSH 凭据与权限链，与项目"特权后端 + SSH/Agent 远程通道"模型一致。
type DockerHandler struct {
	runner DockerCommandRunner
}

// nilRunner 在 svc 为 nil 时返回错误而非 panic（防御性：避免集成测试/部分启动场景崩溃）。
type nilRunner struct{}

func (nilRunner) RunCommandNow(_ context.Context, _ uint, _ string, _ int) (monitor.CommandResult, error) {
	return monitor.CommandResult{}, errors.New("docker management is not available: monitor service is not initialized")
}

func NewDockerHandler(svc DockerCommandRunner) *DockerHandler {
	// Go 接口陷阱：传入 nil *monitor.Service 时接口本身非 nil，
	// 需用反射检测底层指针是否为 nil。
	if svc == nil || reflect.ValueOf(svc).IsNil() {
		svc = nilRunner{}
	}
	return &DockerHandler{runner: svc}
}

// List 返回目标主机上的容器/镜像/卷/网络概览。
func (h *DockerHandler) List(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	resp := dockerListResp{
		Containers: []dockerListContainer{},
		Images:     []dockerImage{},
		Volumes:    []dockerVolume{},
		Networks:   []dockerNetwork{},
	}
	script := `docker ps -a --format '{{json .}}' 2>/dev/null; ` +
		`echo "---IMG---"; ` +
		`docker images --format '{{json .}}' 2>/dev/null; ` +
		`echo "---VOL---"; ` +
		`docker volume ls --format '{{json .}}' 2>/dev/null; ` +
		`echo "---NET---"; ` +
		`docker network ls --format '{{json .}}' 2>/dev/null; ` +
		`echo "---END---"`
	result, err := h.runner.RunCommandNow(c.Request.Context(), id, script, 30)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if !result.Success {
		c.JSON(http.StatusBadGateway, gin.H{"error": result.Stderr, "stdout": result.Stdout})
		return
	}
	sections := splitDockerSections(result.Stdout)
	if raw, ok := sections[""]; ok {
		resp.Containers = parseJSONLines[dockerListContainer](raw)
	}
	if raw, ok := sections["IMG"]; ok {
		resp.Images = parseJSONLines[dockerImage](raw)
	}
	if raw, ok := sections["VOL"]; ok {
		resp.Volumes = parseJSONLines[dockerVolume](raw)
	}
	if raw, ok := sections["NET"]; ok {
		resp.Networks = parseJSONLines[dockerNetwork](raw)
	}
	c.JSON(http.StatusOK, resp)
}

// Inspect 返回单个容器的详情（docker inspect）。
func (h *DockerHandler) Inspect(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "container name is required"})
		return
	}
	cmd := "docker inspect --type container " + shellQuote(name)
	result, err := h.runner.RunCommandNow(c.Request.Context(), id, cmd, 15)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if !result.Success {
		c.JSON(http.StatusBadGateway, gin.H{"error": result.Stderr})
		return
	}
	var arr []dockerInspectContainer
	if err := json.Unmarshal([]byte(result.Stdout), &arr); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "parse inspect output failed", "detail": err.Error()})
		return
	}
	if len(arr) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "container not found"})
		return
	}
	c.JSON(http.StatusOK, arr[0])
}

// Stats 返回所有容器的实时资源占用（docker stats --no-stream）。
func (h *DockerHandler) Stats(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	cmd := `docker stats --no-stream --format '{{json .}}' 2>/dev/null`
	result, err := h.runner.RunCommandNow(c.Request.Context(), id, cmd, 30)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if !result.Success {
		c.JSON(http.StatusOK, gin.H{"stats": []dockerStatsRow{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"stats": parseJSONLines[dockerStatsRow](result.Stdout)})
}

// Logs 返回容器日志（尾部 N 行）。
func (h *DockerHandler) Logs(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "container name is required"})
		return
	}
	tail := c.DefaultQuery("tail", "100")
	cmd := "docker logs --tail " + shellQuote(tail) + " " + shellQuote(name) + " 2>&1"
	result, err := h.runner.RunCommandNow(c.Request.Context(), id, cmd, 15)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"logs": result.Stdout})
}

// act 执行一个容器生命周期动作（start/stop/restart/pause/unpause/remove）。
func (h *DockerHandler) act(c *gin.Context, subcmd string, needConfirm bool) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "container name is required"})
		return
	}
	if needConfirm {
		var in dockerActionRequest
		_ = c.ShouldBindJSON(&in)
		if !in.ConfirmRisk {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "destructive action requires confirmation",
				"code":  "risk_confirmation_required",
			})
			return
		}
	}
	cmd := "docker " + subcmd + " " + shellQuote(name)
	result, err := h.runner.RunCommandNow(c.Request.Context(), id, cmd, 30)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if !result.Success {
		c.JSON(http.StatusBadGateway, gin.H{"error": result.Stderr})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *DockerHandler) Start(c *gin.Context)   { h.act(c, "start", false) }
func (h *DockerHandler) Stop(c *gin.Context)    { h.act(c, "stop", false) }
func (h *DockerHandler) Restart(c *gin.Context) { h.act(c, "restart", false) }
func (h *DockerHandler) Pause(c *gin.Context)   { h.act(c, "pause", false) }
func (h *DockerHandler) Unpause(c *gin.Context) { h.act(c, "unpause", false) }

// Remove 删除容器，需二次确认。
func (h *DockerHandler) Remove(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "container name is required"})
		return
	}
	var in dockerActionRequest
	_ = c.ShouldBindJSON(&in)
	if !in.ConfirmRisk {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "destructive action requires confirmation",
			"code":  "risk_confirmation_required",
		})
		return
	}
	force := c.Query("force") == "1"
	flag := ""
	if force {
		flag = "-f "
	}
	cmd := "docker rm " + flag + shellQuote(name)
	result, err := h.runner.RunCommandNow(c.Request.Context(), id, cmd, 30)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if !result.Success {
		c.JSON(http.StatusBadGateway, gin.H{"error": result.Stderr})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ImageRemove 删除镜像，需二次确认。
func (h *DockerHandler) ImageRemove(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	ref := c.Param("ref")
	if ref == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "image reference is required"})
		return
	}
	var in dockerActionRequest
	_ = c.ShouldBindJSON(&in)
	if !in.ConfirmRisk {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "destructive action requires confirmation",
			"code":  "risk_confirmation_required",
		})
		return
	}
	cmd := "docker rmi " + shellQuote(ref)
	result, err := h.runner.RunCommandNow(c.Request.Context(), id, cmd, 30)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if !result.Success {
		c.JSON(http.StatusBadGateway, gin.H{"error": result.Stderr})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ComposeUp 在指定目录执行 docker compose up -d。
func (h *DockerHandler) ComposeUp(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in composeRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cmd := "cd " + shellQuote(in.Path) + " && docker compose up -d 2>&1"
	result, err := h.runner.RunCommandNow(c.Request.Context(), id, cmd, 120)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": result.Success, "output": result.Stdout + result.Stderr})
}

// ComposeDown 在指定目录执行 docker compose down，需二次确认。
func (h *DockerHandler) ComposeDown(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in composeRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !in.ConfirmRisk {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "destructive action requires confirmation",
			"code":  "risk_confirmation_required",
		})
		return
	}
	cmd := "cd " + shellQuote(in.Path) + " && docker compose down 2>&1"
	result, err := h.runner.RunCommandNow(c.Request.Context(), id, cmd, 120)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": result.Success, "output": result.Stdout + result.Stderr})
}
