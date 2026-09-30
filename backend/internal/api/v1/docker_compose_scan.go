package v1

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
)

// composeScanDir 从环境变量读取 compose 项目扫描根。
func composeScanDir() string {
	d := os.Getenv("COMPOSE_SCAN_DIR")
	if d == "" {
		d = "/host/compose"
	}
	return d
}

// composeProject 是扫描到的一个 compose 项目。
type composeProject struct {
	Name    string `json:"name"`     // 项目名（目录名）
	Path    string `json:"path"`     // compose 文件完整路径
	Dir     string `json:"dir"`      // 项目目录
	File    string `json:"file"`     // 文件名（docker-compose.yml / compose.yml）
	Content string `json:"content"`  // 文件内容（列表时截断，详情时完整）
	HostDir string `json:"host_dir"` // 宿主机对应路径（供用户参考）
}

// composeProjectListResp 项目列表响应。
type composeProjectListResp struct {
	Projects  []composeProject `json:"projects"`
	ScanDir   string           `json:"scan_dir"`
	Available bool             `json:"available"` // 扫描目录是否可访问
}

// composeFileNames 按优先级查找 compose 文件名。
var composeFileNames = []string{
	"docker-compose.yml", "docker-compose.yaml",
	"compose.yml", "compose.yaml",
}

// findComposeFile 在目录中查找 compose 文件，返回文件名（空=未找到）。
func findComposeFile(dir string) string {
	for _, name := range composeFileNames {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return name
		}
	}
	return ""
}

// ScanProjects 扫描 compose 项目目录，列出所有含 compose 文件的项目。
// 扫描深度 2 层（根目录的直接子目录 + 一层嵌套），避免全盘扫描。
func (h *LocalComposeHandler) ScanProjects(c *gin.Context) {
	root := composeScanDir()
	resp := composeProjectListResp{Projects: []composeProject{}, ScanDir: root}

	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		resp.Available = false
		c.JSON(http.StatusOK, resp)
		return
	}
	resp.Available = true

	// 扫描直接子目录 + 根目录
	var found []composeProject
	scanOneLevel(root, &found)
	entries, err := os.ReadDir(root)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			sub := filepath.Join(root, e.Name())
			scanOneLevel(sub, &found)
		}
	}

	// 按名称排序
	sort.Slice(found, func(i, j int) bool { return found[i].Name < found[j].Name })
	resp.Projects = found
	c.JSON(http.StatusOK, resp)
}

// scanOneLevel 扫描单个目录，若含 compose 文件则加入列表。
func scanOneLevel(dir string, out *[]composeProject) {
	name := findComposeFile(dir)
	if name == "" {
		return
	}
	fullPath := filepath.Join(dir, name)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return
	}
	// 列表时内容截断到 200 字符
	preview := string(content)
	if len(preview) > 200 {
		preview = preview[:200] + "..."
	}
	// 推算宿主机路径：/host/compose/xxx → /opt/xxx
	hostDir := dir
	if strings.HasPrefix(dir, "/host/compose") {
		rest := strings.TrimPrefix(dir, "/host/compose")
		hostDir = os.Getenv("COMPOSE_ROOT")
		if hostDir == "" {
			hostDir = "/opt"
		}
		hostDir = hostDir + rest
	}
	*out = append(*out, composeProject{
		Name:    filepath.Base(dir),
		Path:    fullPath,
		Dir:     dir,
		File:    name,
		Content: preview,
		HostDir: hostDir,
	})
}

// composeDirWithin 校验 dir（clean 后）位于扫描根内。不能用裸 HasPrefix：
// "/host/compose-evil" 会绕过 "/host/compose" 前缀（同级目录绕过 + "../" 穿越）。
func composeDirWithin(dir string) bool {
	root := strings.TrimSuffix(filepath.ToSlash(filepath.Clean(composeScanDir())), "/")
	clean := filepath.ToSlash(filepath.Clean(dir))
	return clean == root || strings.HasPrefix(clean, root+"/")
}

// ProjectDetail 返回单个项目的完整 compose 内容。
func (h *LocalComposeHandler) ProjectDetail(c *gin.Context) {
	dir := c.Query("dir")
	if dir == "" {
		errcode.Respond(c, errcode.FieldRequired)
		return
	}
	if !composeDirWithin(dir) {
		errcode.Respond(c, errcode.ComposeDirInvalid)
		return
	}
	name := findComposeFile(dir)
	if name == "" {
		errcode.Respond(c, errcode.ComposeFileNotFound)
		return
	}
	content, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		errcode.Respond(c, errcode.BadGateway.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"content": string(content), "file": name, "dir": dir})
}

// ProjectAction 对扫描到的项目执行 compose up/down（用容器内 CLI）。
// body: {"dir": "/host/compose/myapp", "action": "up|down"}
func (h *LocalComposeHandler) ProjectAction(c *gin.Context) {
	var in struct {
		Dir         string `json:"dir" binding:"required"`
		Action      string `json:"action" binding:"required"` // up | down | restart | pull
		ConfirmRisk bool   `json:"confirm_risk"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	// 安全校验：dir 必须在扫描目录下（防目录穿越）
	if !composeDirWithin(in.Dir) {
		errcode.Respond(c, errcode.ComposeDirInvalid)
		return
	}
	name := findComposeFile(in.Dir)
	if name == "" {
		errcode.Respond(c, errcode.ComposeFileNotFound)
		return
	}
	// 白名单 action
	subArgs := []string{}
	switch in.Action {
	case "up":
		subArgs = []string{"up", "-d"}
	case "down":
		// 四轮 P1 修复：down 停删容器/网络，要求二次确认（与远程版对齐）。
		if !in.ConfirmRisk {
			errcode.Respond(c, errcode.RiskConfirmationRequired)
			return
		}
		subArgs = []string{"down"}
	case "restart":
		subArgs = []string{"restart"}
	case "pull":
		subArgs = []string{"pull"}
	default:
		errcode.Respond(c, errcode.InvalidRequest)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()
	out, err := h.runComposeCLIWithFlags(ctx, in.Dir, "", subArgs)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "output": out + "\n" + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "output": out})
}

// ensure exec is used（runComposeCLIWithFlags 在 docker_compose_local.go 定义）
var _ = exec.Command

// projectSaveRequest 保存 compose 文件内容。
type projectSaveRequest struct {
	Dir     string `json:"dir" binding:"required"`
	Content string `json:"content" binding:"required"`
}

// ProjectSave 保存（覆盖）compose 文件内容，支持在线编辑。
func (h *LocalComposeHandler) ProjectSave(c *gin.Context) {
	var in projectSaveRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	// 安全校验：dir 必须在扫描目录下（防目录穿越写任意文件）
	if !composeDirWithin(in.Dir) {
		errcode.Respond(c, errcode.ComposeDirInvalid)
		return
	}
	name := findComposeFile(in.Dir)
	if name == "" {
		errcode.Respond(c, errcode.ComposeFileNotFound)
		return
	}
	fullPath := filepath.Join(in.Dir, name)
	// 写前先验证语法：写到临时文件，用 compose config 验证，通过才覆盖。
	// 四轮 P2 修复：tmp 掺随机后缀——固定 .tmp 在并发保存/双击时两个请求写同一
	// tmp 交错（内容混合的文件可能通过校验被 rename 上线，另一请求 rename 失败）。
	tmp, err := os.CreateTemp(in.Dir, name+".*.tmp")
	if err != nil {
		errcode.Respond(c, errcode.UpstreamFailed.WithErr(err))
		return
	}
	tmpPath := tmp.Name()
	if _, err := tmp.WriteString(in.Content); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		errcode.Respond(c, errcode.UpstreamFailed.WithErr(err))
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		errcode.Respond(c, errcode.UpstreamFailed.WithErr(err))
		return
	}
	// 验证语法
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", tmpPath, "config")
	cmd.Env = append(os.Environ(), "DOCKER_HOST=unix:///var/run/docker.sock")
	validateOut, validateErr := cmd.CombinedOutput()
	if validateErr != nil {
		// 语法错误：删临时文件，返回验证错误（不覆盖原文件）
		os.Remove(tmpPath)
		errcode.Respond(c, errcode.InvalidRequest.WithErr(errors.New(string(validateOut))))
		return
	}
	// 验证通过：rename 原子替换（避免半写状态被 compose 读到）
	if err := os.Rename(tmpPath, fullPath); err != nil {
		os.Remove(tmpPath)
		errcode.Respond(c, errcode.BadGateway.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "file": name})
}
