package chain

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path"
	"time"

	"github.com/hxpt/ptpatronus/internal/cloud"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/notice"
)

// ---- 云端任务执行器：串行消费 CloudTask 队列 ----
//
// 自动上传链路（afterTransferCloudUpload）不再直接调网盘 API，改为入队 upload 任务；
// executor 单工作协程按序执行（失败自动重试 ≤3、退避 30s），UI 经 /cloud/tasks 看进度。
// strm 任务：登记 StrmRecord + 落 .strm（自动上传完成后由 executor 追加，保证顺序）。

const cloudTaskMaxRetries = 3

// CloudTaskRunner 队列执行器（*CloudChain 内嵌）。
type CloudTaskRunner struct {
	op   *oper.CloudTaskOper
	hydr func() *CloudChain // 宿主 chain（避免构造环）
	stop chan struct{}
}

// StartCloudTaskExecutor 启动消费协程（app 装配期一次）。
func (c *CloudChain) StartCloudTaskExecutor(taskOp *oper.CloudTaskOper) {
	c.tasks = &CloudTaskRunner{op: taskOp, hydr: func() *CloudChain { return c }, stop: make(chan struct{})}
	// 恢复上次进程中断的 running 任务
	if n, err := taskOp.RequeueStaleRunning(10 * time.Minute); err == nil && n > 0 {
		slog.Info("cloud tasks: requeued stale running", "count", n)
	}
	go c.tasks.loop()
}

func (r *CloudTaskRunner) loop() {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.stop:
			return
		case <-tick.C:
			if r.drainOnce() {
				// 失败退避（防打爆网盘）：可中断等待 30s——停机信号到达立即退出。
				select {
				case <-r.stop:
					return
				case <-time.After(30 * time.Second):
				}
			}
		}
	}
}

// drainOnce 取空当前 pending（单条串行；executor 内串行避免同盘并发限流）。
// 失败退避用 backoffUntil 串行计数（非 sleep 阻塞）——execute 返回 true 表示
// 刚失败需退避，loop 侧在 stop 与定时器之间 select 等待，停机时立即退出，
// 不再被 30s 阻塞 sleep 拖住优雅关闭。
func (r *CloudTaskRunner) drainOnce() bool {
	for {
		t, err := r.op.ClaimPending()
		if err != nil {
			slog.Warn("cloud tasks: claim failed", "err", err)
			return false
		}
		if t == nil {
			return false
		}
		if r.execute(t) {
			return true // 失败：交回 loop 做可中断退避
		}
	}
}

// execute 执行单任务并落状态。返回 true 表示本条失败（调用方需退避后再取下一条）。
func (r *CloudTaskRunner) execute(t *models.CloudTask) bool {
	c := r.hydr()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	_ = r.op.UpdateProgress(t.ID, 10)
	var err error
	switch t.Kind {
	case "upload":
		err = c.UploadFile(ctx, t.DriveID, t.CloudPath, t.LocalPath, t.Size)
		if err == nil {
			_ = r.op.UpdateProgress(t.ID, 90)
			// 上传完成 → 登记 StrmRecord + 落 .strm（读 transfer.cloud_upload 的 STRM 配置）。
			// DeleteLocal 的前置条件（P1 修复）：STRM 失败不删本地——否则本地已删、
			// 云端有文件、无 strm 无记录，302 直链/订阅追更/媒体库全部看不到该文件。
			if strmErr := r.afterUploadStrm(ctx, t); strmErr != nil {
				r.noticeDeleteLocalBlocked(t, strmErr)
				break
			}
			// 上传后删本地整理文件（DeleteLocal 开启时；硬链模式仅删整理产物，不触源文件）
			r.afterUploadCleanup(t)
		}
	case "strm":
		err = c.UpsertStrm(ctx, t.DriveID, t.CloudPath, payloadStr(t.Payload, "output_dir"), payloadStr(t.Payload, "template"))
	case "organize":
		var in cloud.OrganizeOptions
		if jerr := json.Unmarshal([]byte(t.Payload), &in); jerr != nil {
			err = fmt.Errorf("organize payload parse: %w", jerr)
		} else {
			res, oerr := c.OrganizeCloud(ctx, in)
			if oerr != nil {
				err = oerr
			} else {
				slog.Info("cloud task: organize done", "task", t.ID, "moved", res.Moved, "skipped", res.Skipped)
				// 整理归位后内容变了 → 补刷媒体库，让 STRM 规则/直链尽早看到新结构。
				c.refreshLibraries()
			}
		}
	case "import_tree":
		err = c.ImportTreeStrm(ctx, t)
		if err == nil {
			c.refreshLibraries() // 批量落完 .strm 后立即扫库（不待 6h cron）
		}
	default:
		err = fmt.Errorf("未知任务类型 %s", t.Kind)
	}
	if err != nil {
		slog.Warn("cloud task failed", "id", t.ID, "kind", t.Kind, "attempts", t.Attempts+1, "err", err)
		if rerr := r.op.Fail(t.ID, err.Error(), cloudTaskMaxRetries); rerr != nil {
			slog.Warn("cloud tasks: mark fail errored", "err", rerr)
		}
		return true
	}
	if serr := r.op.Succeed(t.ID); serr != nil {
		slog.Warn("cloud tasks: mark success errored", "err", serr)
	}
	return false
}

// afterUploadStrm 上传成功后按 transfer.cloud_upload 配置生成 STRM（同事务链尾）。
// 落地 .strm 后触发一次媒体库刷新——Emby/JF 才能立刻扫到新入库的流媒体条目
// （此前只在直传兜底分支刷新，队列路径漏扫，需等下一次偶发事件）。
// 返回错误供 DeleteLocal 决策：STRM 未成功不得删本地（数据丢失防护）。
// STRM 功能未启用（nil 配置/无输出目录）返回 nil——用户没开 STRM，删除不受阻。
func (r *CloudTaskRunner) afterUploadStrm(ctx context.Context, t *models.CloudTask) error {
	c := r.hydr()
	cfg := loadTransferCloudUpload(c)
	if cfg == nil || !cfg.Enabled || cfg.StrmOutputDir == "" {
		return nil
	}
	if err := c.UpsertStrm(ctx, t.DriveID, t.CloudPath, cfg.StrmOutputDir, cfg.StrmTemplate); err != nil {
		slog.Warn("cloud task: post-upload strm failed, keep local file", "path", t.CloudPath, "err", err)
		return err
	}
	if ms := c.mediaServerRefresher(); ms != nil {
		ms(path.Base(t.CloudPath))
	}
	return nil
}

// noticeDeleteLocalBlocked STRM 失败阻断 DeleteLocal 时发告警（当天去重）：
// 本地文件保留是安全侧，但用户需知道「云端有、本地也在」的原因与修复入口。
func (r *CloudTaskRunner) noticeDeleteLocalBlocked(t *models.CloudTask, strmErr error) {
	c := r.hydr()
	if c.notice == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = c.notice.Notice(ctx, notice.Input{
		Source: "cloud",
		Key:    fmt.Sprintf("strm-blocked:%s:%s", t.Kind, path.Base(t.CloudPath)),
		Title:  "STRM 生成失败，已保留本地文件",
		Body: fmt.Sprintf("上传成功但 STRM 登记失败，为防数据丢失未删除本地文件（%s）。"+
			"请排查 STRM 输出目录/数据库后重新生成。错误：%v", t.LocalPath, strmErr),
		Level: "warning",
	}, true)
}

// afterUploadCleanup DeleteLocal 开启时上传成功后删除本地源文件（best-effort）。
func (r *CloudTaskRunner) afterUploadCleanup(t *models.CloudTask) {
	cfg := loadTransferCloudUpload(r.hydr())
	if cfg == nil || !cfg.DeleteLocal || t.LocalPath == "" {
		return
	}
	if err := os.Remove(t.LocalPath); err != nil && !os.IsNotExist(err) {
		slog.Warn("cloud task: delete local failed", "file", t.LocalPath, "err", err)
	}
}

// mediaServerRefresher 整理链的扫库钩子（TransferChain.OnDownloadCompleted → 所有
// RefreshOnDownload 服务器整库扫描）。app 装配期注入；nil 时跳过。
func (c *CloudChain) mediaServerRefresher() func(name string) {
	return c.libRefresher
}

// refreshLibraries 触发注入的媒体库刷新（organize/import_tree 任务收尾；best-effort）。
func (c *CloudChain) refreshLibraries() {
	if c.libRefresher == nil {
		return
	}
	c.libRefresher("cloud-task")
}

// SetLibraryRefresher 注入媒体库刷新钩子（断 chain 内 CloudChain→TransferChain 依赖）。
func (c *CloudChain) SetLibraryRefresher(f func(name string)) { c.libRefresher = f }

// loadTransferCloudUpload 从 SystemConfig 读 transfer.cloud_upload（懒注入避免依赖环）。
func loadTransferCloudUpload(c *CloudChain) *transferCloudUploadCfg {
	getter := c.transferCfgGetter
	if getter == nil {
		return nil
	}
	raw, err := getter.Get("transfer.config")
	if err != nil || raw == "" {
		return nil
	}
	var cfg struct {
		CloudUpload transferCloudUploadCfg `json:"cloud_upload"`
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil
	}
	return &cfg.CloudUpload
}

// transferCloudUploadCfg transfer.Config.CloudUpload 的镜像（避免 chain→transfer 依赖环）。
type transferCloudUploadCfg struct {
	Enabled       bool   `json:"enabled"`
	StrmOutputDir string `json:"strm_output_dir"`
	StrmTemplate  string `json:"strm_template"`
	DeleteLocal   bool   `json:"delete_local"`
}

// payloadStr 从 Payload JSON 取字段。
func payloadStr(payload, key string) string {
	if payload == "" {
		return ""
	}
	var m map[string]string
	if json.Unmarshal([]byte(payload), &m) != nil {
		return ""
	}
	return m[key]
}

// ImportTreeStrm 目录树批量导入（P1-3 入口）：Payload {tree, output_dir, template}。
// tree 为 115「导出目录树」文本（一行一个相对路径，目录/文件混排，缩进或全路径均可）。
func (c *CloudChain) ImportTreeStrm(ctx context.Context, t *models.CloudTask) error {
	var p struct {
		Tree      string `json:"tree"`
		OutputDir string `json:"output_dir"`
		Template  string `json:"template"`
		Base      string `json:"base"` // 树路径相对的云盘根（默认 "/"）
	}
	if err := json.Unmarshal([]byte(p2b(t.Payload)), &p); err != nil {
		return fmt.Errorf("payload parse: %w", err)
	}
	if p.OutputDir == "" {
		return fmt.Errorf("import_tree: 缺 output_dir")
	}
	base := p.Base
	if base == "" {
		base = "/"
	}
	n := 0
	for _, line := range splitLines(p.Tree) {
		line = trimSpace(line)
		if line == "" || stringsContains(line, "---") { // 分隔行跳过
			continue
		}
		if !cloud.IsMediaFile(line) {
			continue
		}
		cp := joinCloudPath(base, line)
		if err := c.UpsertStrm(ctx, t.DriveID, cp, p.OutputDir, p.Template, base); err != nil {
			slog.Debug("import_tree: strm failed", "path", cp, "err", err)
			continue
		}
		n++
	}
	slog.Info("cloud: import tree done", "task", t.ID, "generated", n)
	return nil
}

func p2b(s string) []byte { return []byte(s) }

func splitLines(s string) []string {
	out := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\r') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

func stringsContains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// joinCloudPath base + line（line 可能带缩进前缀或全路径斜杠）。
func joinCloudPath(base, line string) string {
	line = trimSlashes(line)
	base = trimSlashesRight(base)
	if base == "" || base == "/" {
		return "/" + line
	}
	return base + "/" + line
}

func trimSlashes(s string) string {
	for len(s) > 0 && s[0] == '/' {
		s = s[1:]
	}
	return s
}

func trimSlashesRight(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
