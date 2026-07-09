package v1

import (
	"context"
	"fmt"
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

type commandRunRequest struct {
	TargetIDs   []uint `json:"target_ids"`
	Command     string `json:"command"`
	TimeoutSec  int    `json:"timeout_sec"`
	Source      string `json:"source"`
	Note        string `json:"note"`
	ConfirmRisk bool   `json:"confirm_risk"`
}

func (h *CommandRunHandler) List(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	runs, err := h.runs.List(c.Query("source"), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"runs": runs})
}

func (h *CommandRunHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	run, err := h.runs.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "command run not found"})
		return
	}
	c.JSON(http.StatusOK, run)
}

func (h *CommandRunHandler) Create(c *gin.Context) {
	var in commandRunRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	in.Command = strings.TrimRight(in.Command, "\r\n")
	if strings.TrimSpace(in.Command) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "command is required"})
		return
	}
	targetIDs := uniqueTargetIDs(in.TargetIDs)
	if len(targetIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_ids is required"})
		return
	}
	if len(targetIDs) > 50 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many targets, max 50"})
		return
	}
	riskLevel, riskReason := classifyCommandRisk(in.Command)
	if riskLevel == "high" && !in.ConfirmRisk {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":       "dangerous command requires confirmation",
			"code":        "risk_confirmation_required",
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out, err := h.runs.Get(run.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		errText = err.Error()
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
