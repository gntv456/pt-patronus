package v1

import (
	"context"
	"fmt"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/monitor"
)

type CommandRunHandler struct {
	targets *oper.MonitorOper
	runs    *oper.CommandRunOper
	svc     *monitor.Service
}

func NewCommandRunHandler(targets *oper.MonitorOper, runs *oper.CommandRunOper, svc *monitor.Service) *CommandRunHandler {
	return &CommandRunHandler{targets: targets, runs: runs, svc: svc}
}

// opsRunMaxDuration 一次批量命令执行的总时长上限（脱离请求上下文后得有人兜底）。
const opsRunMaxDuration = 30 * time.Minute

// opsExecContext 派生批量执行用的上下文：不随客户端断开而取消（审计型批量任务不能被
// 一次关页面腰斩），值仍从请求上下文继承（trace 等），外加总时长上限。
func opsExecContext(req context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(req), opsRunMaxDuration)
}

type commandRunRequest struct {
	TargetIDs   []uint `json:"target_ids" binding:"max=200"` // DoS 硬上限；业务上限 50 由 Create 内 len 检查 + 友好错误处理
	Command     string `json:"command" binding:"max=2000"`
	TimeoutSec  int    `json:"timeout_sec" binding:"min=0,max=3600"`
	Source      string `json:"source" binding:"max=32"`
	Note        string `json:"note" binding:"max=512"`
	ConfirmRisk bool   `json:"confirm_risk"`
}

func (h *CommandRunHandler) List(c *gin.Context) {
	source := c.Query("source")
	reqLimit, _ := strconv.Atoi(c.Query("limit"))
	// 回显生效 limit：这条是运维审计台账，调用方以为自己要到了 500 条、
	// 实际只拿到默认 30 条时，「没有失败记录」会被读成整段历史的结论。
	limit := oper.NormalizeCommandRunLimit(reqLimit)
	runs, err := h.runs.List(source, limit)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	total, cntErr := h.runs.Count(source)
	truncated := cntErr != nil || total > int64(len(runs))
	if cntErr != nil {
		slog.Warn("command run count failed", "source", source, "err", cntErr)
	}
	resp := gin.H{"runs": runs, "limit": limit, "truncated": truncated}
	if cntErr == nil {
		resp["total"] = total
	}
	c.JSON(http.StatusOK, resp)
}

func (h *CommandRunHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	run, err := h.runs.Get(id)
	if err != nil {
		errcode.Respond(c, errcode.ResourceNotFound)
		return
	}
	c.JSON(http.StatusOK, run)
}

func (h *CommandRunHandler) Create(c *gin.Context) {
	var in commandRunRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	in.Command = strings.TrimRight(in.Command, "\r\n")
	if strings.TrimSpace(in.Command) == "" {
		errcode.Respond(c, errcode.FieldRequired)
		return
	}
	targetIDs := uniqueTargetIDs(in.TargetIDs)
	if len(targetIDs) == 0 {
		errcode.Respond(c, errcode.FieldRequired)
		return
	}
	if len(targetIDs) > 50 {
		errcode.Respond(c, errcode.InvalidRequest)
		return
	}
	riskLevel, riskReason := classifyCommandRisk(in.Command)
	if riskLevel == "high" && !in.ConfirmRisk {
		c.JSON(http.StatusBadRequest, gin.H{
			"message":     errcode.RiskConfirmationRequired.Message,
			"error":       errcode.RiskConfirmationRequired.Message,
			"code":        errcode.RiskConfirmationRequired.Code,
			"risk_level":  riskLevel,
			"risk_reason": riskReason,
		})
		return
	}

	started := time.Now()
	run := &models.OpsCommandRun{
		Command:    in.Command,
		TimeoutSec: in.TimeoutSec,
		Source:     commandRunSource(in.Source),
		Note:       strings.TrimSpace(in.Note),
		Status:     "running",
		RiskLevel:  riskLevel,
		RiskReason: riskReason,
		CreatedBy:  currentUsername(c),
		TotalCount: len(targetIDs),
		StartedAt:  started,
	}
	if err := h.runs.Create(run); err != nil {
		// #391 审计写不进去 ⇒ 命令一条都没执行。这句必须说明白：
		// 旧文案「服务内部错误，请稍后重试」会诱导盲目重试（甚至重复执行）。
		respondOpsAuditWriteErr(c, err)
		return
	}

	// 执行上下文不能挂在 HTTP 请求上：一次批量要跑几十台、每台可到 timeout_sec。
	// 用 c.Request.Context() 时，浏览器关标签页/前端超时会把在途命令一起取消，
	// 而远端的命令多半已经执行甚至跑完——审计记录却把它们写成 context canceled 的「失败」。
	ctx, cancelExec := opsExecContext(c.Request.Context())
	defer cancelExec()
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	var mu sync.Mutex
	successCount := 0
	failureCount := 0
	for _, targetID := range targetIDs {
		targetID := targetID
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			// recover 防止单个 target 执行 panic 致 wg 永不 Done（wg.Wait 阻塞、批量请求挂死）。
			// SSH/命令执行链路涉及多层协议解析，panic 不可扩散到整个批量。
			defer func() {
				if r := recover(); r != nil {
					result := &models.OpsCommandRunTarget{
						RunID: run.ID, TargetID: targetID, Status: "error",
						Error: fmt.Sprintf("执行异常(panic): %v", r),
					}
					_ = h.runs.AddTargetResult(result)
					mu.Lock()
					failureCount++
					mu.Unlock()
					slog.Error("ops command run panic", "target", targetID, "panic", r)
				}
			}()
			result := h.runOneTarget(ctx, run.ID, targetID, in.Command, in.TimeoutSec)
			if err := h.runs.AddTargetResult(result); err != nil {
				result.Status = "error"
				result.Error = strings.TrimSpace(result.Error + "\n保存执行结果失败: " + err.Error())
			}
			mu.Lock()
			if result.Status == "success" {
				successCount++
			} else {
				failureCount++
			}
			mu.Unlock()
		}()
	}
	wg.Wait()

	finished := time.Now()
	status := commandRunStatus(successCount, failureCount)
	if err := h.runs.Finish(run.ID, status, successCount, failureCount, finished, finished.Sub(started).Milliseconds()); err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	out, err := h.runs.Get(run.ID)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *CommandRunHandler) runOneTarget(ctx context.Context, runID uint, targetID uint, command string, timeoutSec int) *models.OpsCommandRunTarget {
	started := time.Now()
	finished := started
	targetName := fmt.Sprintf("#%d", targetID)
	if target, err := h.targets.Get(targetID); err == nil && target.Name != "" {
		targetName = target.Name
	}

	result, err := h.svc.RunCommandNow(ctx, targetID, command, timeoutSec)
	status := "error"
	stdout := ""
	stderr := ""
	errText := ""
	durationMS := int64(0)
	if err != nil {
		finished = time.Now()
		errText = userErrText(err) // 与其它面一致：认识的哨兵翻成中文可操作提示，其余原样透传
		durationMS = finished.Sub(started).Milliseconds()
	} else {
		if result.TargetName != "" {
			targetName = result.TargetName
		}
		if !result.StartedAt.IsZero() {
			started = result.StartedAt
		}
		if !result.FinishedAt.IsZero() {
			finished = result.FinishedAt
		} else {
			finished = time.Now()
		}
		stdout = result.Stdout
		stderr = result.Stderr
		errText = result.Error
		durationMS = result.DurationMS
		if durationMS <= 0 {
			durationMS = finished.Sub(started).Milliseconds()
		}
		if result.Success {
			status = "success"
		} else if errText == "" {
			errText = "执行失败"
		}
	}
	return &models.OpsCommandRunTarget{
		RunID:      runID,
		TargetID:   targetID,
		TargetName: targetName,
		Status:     status,
		Stdout:     stdout,
		Stderr:     stderr,
		Error:      errText,
		DurationMS: durationMS,
		StartedAt:  started,
		FinishedAt: &finished,
	}
}

func uniqueTargetIDs(ids []uint) []uint {
	seen := map[uint]bool{}
	out := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func commandRunSource(source string) string {
	source = strings.TrimSpace(source)
	switch source {
	case "deploy", "manual", "terminal":
		return source
	default:
		return "batch"
	}
}

func commandRunStatus(success, failure int) string {
	switch {
	case failure == 0:
		return "success"
	case success == 0:
		return "failed"
	default:
		return "partial"
	}
}

var highRiskCommandPatterns = []struct {
	reason string
	re     *regexp.Regexp
}{
	{"删除根目录或系统目录", regexp.MustCompile(`(?i)\brm\s+-[^\n;|&]*r[^\n;|&]*f[^\n;|&]*\s+/(?:\s|$|[;&|])`)},
	{"格式化或擦除磁盘", regexp.MustCompile(`(?i)\b(mkfs|wipefs)\b`)},
	{"向磁盘设备写入原始数据", regexp.MustCompile(`(?i)\bdd\s+.*\bof=/dev/`)},
	{"电源操作", regexp.MustCompile(`(?i)\b(shutdown|reboot|poweroff|halt)\b`)},
	{"递归放开根目录权限", regexp.MustCompile(`(?i)\bchmod\s+-R\s+777\s+/`)},
	{"递归修改根目录属主", regexp.MustCompile(`(?i)\bchown\s+-R\s+\S+\s+/`)},
}

var mediumRiskCommandPatterns = []struct {
	reason string
	re     *regexp.Regexp
}{
	{"递归强制删除", regexp.MustCompile(`(?i)\brm\s+-[^\n;|&]*r[^\n;|&]*f`)},
	{"停止或重启服务", regexp.MustCompile(`(?i)\bsystemctl\s+(stop|restart)\b`)},
	{"清理容器或镜像", regexp.MustCompile(`(?i)\bdocker\s+(rm|rmi|system\s+prune)\b`)},
	{"数据库破坏性操作", regexp.MustCompile(`(?i)\b(drop\s+database|truncate\s+table)\b`)},
}

func classifyCommandRisk(command string) (string, string) {
	for _, item := range highRiskCommandPatterns {
		if item.re.MatchString(command) {
			return "high", item.reason
		}
	}
	for _, item := range mediumRiskCommandPatterns {
		if item.re.MatchString(command) {
			return "medium", item.reason
		}
	}
	return "low", ""
}
