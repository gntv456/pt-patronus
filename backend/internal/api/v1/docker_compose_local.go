package v1

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
)

// LocalCompose 在本机通过容器内 docker compose CLI 执行编排。
// CLI 通过 DOCKER_HOST=unix:///var/run/docker.sock 连接 daemon（不依赖宿主机 docker）。
// 支持两种输入：
//   - content：直接粘贴 compose 内容（写到临时文件执行，最常用）
//   - path：容器内可访问的 compose 文件路径（需挂载宿主目录）
type LocalComposeHandler struct{}

func NewLocalComposeHandler() *LocalComposeHandler {
	return &LocalComposeHandler{}
}

// composeContentRequest 粘贴内容模式的请求体。
type composeContentRequest struct {
	Content     string `json:"content" binding:"required"` // docker-compose.yml 内容
	Name        string `json:"name"`                       // 项目名（可选）
	Action      string `json:"action"`                     // up | down | config | ps
	ConfirmRisk bool   `json:"confirm_risk"`               // down 等破坏性操作二次确认
}

// localComposeDir 存放临时 compose 文件的目录。
const localComposeDir = "/app/data/compose"

// runComposeCLI 执行 docker compose 命令，返回合并输出。
// workDir 是 compose 文件所在目录。
func (h *LocalComposeHandler) runComposeCLI(ctx context.Context, action, workDir, projectName string) (string, error) {
	args := []string{"compose"}
	if projectName != "" {
		args = append(args, "-p", projectName)
	}
	args = append(args, action)
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = workDir
	// 通过 socket 连 daemon（不用宿主机 docker daemon 的默认 socket 路径可能不同）
	cmd.Env = append(os.Environ(), "DOCKER_HOST=unix:///var/run/docker.sock")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// ComposeUp 执行 docker compose up -d（粘贴内容模式）。
func (h *LocalComposeHandler) ComposeUp(c *gin.Context) {
	var in composeContentRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	workDir, cleanup, err := writeComposeFile(in.Content, in.Name)
	if err != nil {
		errcode.Respond(c, errcode.ComposeWriteFailed.WithErr(err))
		return
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()
	// up 带 -d（detached）
	out, err := h.runComposeCLIWithFlags(ctx, workDir, in.Name, []string{"up", "-d"})
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "output": out + "\n" + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "output": out})
}

// ComposeDown 执行 docker compose down。
func (h *LocalComposeHandler) ComposeDown(c *gin.Context) {
	var in composeContentRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	// 四轮 P1 修复：down 会停删容器+网络（默认含匿名卷），与远程版对齐要确认。
	if !in.ConfirmRisk {
		errcode.Respond(c, errcode.RiskConfirmationRequired)
		return
	}
	workDir, cleanup, err := writeComposeFile(in.Content, in.Name)
	if err != nil {
		errcode.Respond(c, errcode.ComposeWriteFailed.WithErr(err))
		return
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Minute)
	defer cancel()
	out, err := h.runComposeCLIWithFlags(ctx, workDir, in.Name, []string{"down"})
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "output": out + "\n" + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "output": out})
}

// ComposeValidate 验证 compose 内容语法（docker compose config）。
func (h *LocalComposeHandler) ComposeValidate(c *gin.Context) {
	var in composeContentRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	workDir, cleanup, err := writeComposeFile(in.Content, in.Name)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	out, err := h.runComposeCLIWithFlags(ctx, workDir, in.Name, []string{"config"})
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "output": out})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "output": out})
}

// runComposeCLIWithFlags 执行带子命令参数的 compose。
func (h *LocalComposeHandler) runComposeCLIWithFlags(ctx context.Context, workDir, projectName string, subArgs []string) (string, error) {
	args := []string{"compose"}
	if projectName != "" {
		args = append(args, "-p", projectName)
	}
	args = append(args, subArgs...)
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "DOCKER_HOST=unix:///var/run/docker.sock")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// projectNameRe 项目名只允许字母数字下划线连字符。
var projectNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// writeComposeFile 把 compose 内容写到临时目录，返回目录路径和清理函数。
func writeComposeFile(content, name string) (string, func(), error) {
	if err := os.MkdirAll(localComposeDir, 0755); err != nil {
		return "", nil, err
	}
	// 项目名用于目录名，防冲突；空则用时间戳
	dirName := name
	if dirName == "" || !projectNameRe.MatchString(dirName) {
		dirName = fmt.Sprintf("tmp_%d", time.Now().UnixNano())
	}
	workDir := filepath.Join(localComposeDir, dirName)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return "", nil, err
	}
	composePath := filepath.Join(workDir, "docker-compose.yml")
	// 四轮 P2 修复：tmp+rename 原子写（直写崩溃留半份 compose 文件）。
	tmpPath := composePath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0644); err != nil {
		os.RemoveAll(workDir)
		return "", nil, err
	}
	if err := os.Rename(tmpPath, composePath); err != nil {
		os.RemoveAll(workDir)
		return "", nil, err
	}
	cleanup := func() { os.RemoveAll(workDir) }
	return workDir, cleanup, nil
}

// ComposeCheck 检查容器内 docker compose CLI 是否可用。
func (h *LocalComposeHandler) ComposeCheck(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "compose", "version")
	cmd.Env = append(os.Environ(), "DOCKER_HOST=unix:///var/run/docker.sock")
	out, err := cmd.CombinedOutput()
	c.JSON(http.StatusOK, gin.H{
		"available": err == nil,
		"version":   strings.TrimSpace(string(out)),
	})
}
