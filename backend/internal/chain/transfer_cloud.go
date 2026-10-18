package chain

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/cloud"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/transfer"
)

// ---- 整理后自动上传网盘 + 生成 STRM ----
//
// 订阅下载 → 自动整理（既有链路）→ 本模块：上传媒体文件到网盘 CloudBase →
// （可选）在 StrmOutputDir 生成 .strm（内容指向 /api/v1/cloud/direct/:driveId/<云盘路径>
// 302 直链端点）→ 触发媒体库刷新。订阅追更（SyncSubscriptions）在云端出现新文件后按
// StrmRecord 记录自动链接订阅并通知。

// CloudUploader 云盘上传能力（*CloudChain 结构化实现；接口断耦避免 transfer→cloud 依赖环）。
type CloudUploader interface {
	UploadFile(ctx context.Context, driveID uint, cloudPath, localPath string, size int64) error
}

// afterTransferCloudUpload 在整理成功后执行云上传 + STRM 生成。异步调用（不阻塞 record），
// 单文件失败记日志继续；全部失败发通知。返回上传成功的文件数。
func (c *TransferChain) afterTransferCloudUpload(ctx context.Context, res *transfer.Result, cfg transfer.Config) {
	cu := cfg.CloudUpload
	if !cu.Enabled || c.cloudUp == nil || len(res.Files) == 0 {
		return
	}
	// 注入库根目录（云盘路径映射用，保留 标题/季 结构）；不入库序列化。
	cu.MovieDir, cu.TVDir = cfg.MovieDir, cfg.TVDir
	// 四轮 P2 修复：直传兜底是 fire-and-forget，此前 Shutdown 不感知、无法等待——
	// 退出时上传/删本地被硬杀（云端残留未 complete 的分片任务）。挂 WaitGroup 供
	// Shutdown 等待（队列路径由 cloudtask executor 的停机保护覆盖，无需跟踪）。
	c.cloudWg.Add(1)
	go func() {
		defer c.cloudWg.Done()
		actx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
		defer cancel()
		// 任务队列可用 → 入队（进度可视 + 自动重试）；否则直传（fire-and-forget 兜底）
		if q := c.cloudTaskQueue(); q != nil {
			n := 0
			for _, f := range res.Files {
				if !cu.UploadVideos && !isSubtitleOrImage(f) {
					continue
				}
				cloudPath, err := cloudPathFor(cu, res.DestDir, f)
				if err != nil {
					slog.Warn("cloud upload: map path failed", "file", f, "err", err)
					continue
				}
				fi, err := os.Stat(f)
				if err != nil {
					// 同一循环里 map path / enqueue 两支都留了痕，只有这里裸 continue：
					// 本地文件读不到（已被清理/权限/换盘）就静默少传一个文件，
					// 而通知正文只报「已排队 N 个」，用户无从知道少了哪一个。
					slog.Warn("cloud upload: stat failed, file skipped", "file", f, "err", err)
					continue
				}
				if err := q.EnqueueCloudTask(&models.CloudTask{
					DriveID: cu.DriveID, Kind: "upload", CloudPath: cloudPath,
					LocalPath: f, Size: fi.Size(),
				}); err != nil {
					slog.Warn("cloud upload: enqueue failed", "file", f, "err", err)
					continue
				}
				n++
			}
			if n > 0 && c.notice != nil {
				_ = c.notice.Notice(actx, notice.Input{
					Source: "cloud", Key: "cloud-enqueue:" + resTitle(res),
					Title: "已入队上传：" + resTitle(res),
					Body:  fmt.Sprintf("%d 个文件 → %s（队列自动执行）", n, cu.CloudBase),
					Level: "info",
				}, false)
			}
			return
		}
		uploaded, failed, strmBlocked := 0, 0, 0
		uploadedFiles := make(map[string]bool, len(res.Files))
		for _, f := range res.Files {
			if !cu.UploadVideos && !isSubtitleOrImage(f) {
				continue // 仅字幕/图片（视频本体不上传的极简模式）
			}
			cloudPath, err := cloudPathFor(cu, res.DestDir, f)
			if err != nil {
				slog.Warn("cloud upload: map path failed", "file", f, "err", err)
				failed++
				continue
			}
			fi, err := os.Stat(f)
			if err != nil {
				slog.Warn("cloud upload: stat failed", "file", f, "err", err)
				failed++
				continue
			}
			if err := c.cloudUp.UploadFile(actx, cu.DriveID, cloudPath, f, fi.Size()); err != nil {
				slog.Warn("cloud upload: upload failed", "file", f, "err", err)
				failed++
				continue
			}
			uploaded++
			uploadedFiles[f] = true
			slog.Info("cloud upload: uploaded", "file", filepath.Base(f), "to", cloudPath)
			// DeleteLocal 前置 STRM（P1 修复，直传兜底路径）：先登记 StrmRecord+落 .strm，
			// 成功才删本地——否则本地已删、无 strm 无记录，直链/追更/媒体库全部失明。
			// STRM 未启用（无输出目录）不受阻，直接删。
			if cu.DeleteLocal {
				if strmGen, ok := c.cloudUp.(CloudStrmSink); ok && cu.StrmOutputDir != "" {
					if err := strmGen.UpsertStrm(actx, cu.DriveID, cloudPath, cu.StrmOutputDir, cu.StrmTemplate); err != nil {
						slog.Warn("cloud upload: strm failed, keep local file", "file", f, "err", err)
						strmBlocked++
						continue
					}
				}
				if err := os.Remove(f); err != nil {
					slog.Warn("cloud upload: delete local failed", "file", f, "err", err)
				}
			}
		}
		if uploaded > 0 {
			c.emitCloudStrm(actx, cu, res, uploadedFiles)
			if c.notice != nil {
				title := "已上传网盘：" + resTitle(res)
				body := fmt.Sprintf("%d 个文件 → %s", uploaded, cu.CloudBase)
				if failed > 0 {
					body += fmt.Sprintf("（%d 个失败，见日志）", failed)
				}
				if strmBlocked > 0 {
					body += fmt.Sprintf("（%d 个因 STRM 失败保留本地）", strmBlocked)
				}
				_ = c.notice.Notice(actx, notice.Input{
					Source: "cloud", Key: "cloud-upload:" + title,
					Title: title, Body: body, Level: "success",
				}, false)
			}
		} else if failed > 0 && c.notice != nil {
			_ = c.notice.Notice(actx, notice.Input{
				Source: "cloud", Key: "cloud-upload-fail:" + resTitle(res),
				Title: "网盘上传失败：" + resTitle(res),
				Body:  fmt.Sprintf("%d 个文件上传失败，见日志", failed), Level: "warning",
			}, false)
		}
	}()
}

// emitCloudStrm 上传成功后把新文件登记为 StrmRecord 并按需写本地 .strm。
// StrmRecord 是 302 直链端点与订阅追更的索引；.strm 文件供 Emby 扫库识别。
// 只处理 uploaded 命中的文件——上传失败的文件若也登记，会为云端不存在的对象写
// .strm（媒体库 404 条目）并触发「订阅资源已就绪」误报。
func (c *TransferChain) emitCloudStrm(ctx context.Context, cu transfer.CloudUpload, res *transfer.Result, uploaded map[string]bool) {
	strmGen, ok := c.cloudUp.(CloudStrmSink)
	if !ok {
		return
	}
	for _, f := range res.Files {
		if !uploaded[f] {
			continue
		}
		cloudPath, err := cloudPathFor(cu, res.DestDir, f)
		if err != nil {
			// 同一个循环里 UpsertStrm 失败有 Warn，这一支却是裸 continue：
			// 该文件的 .strm 永不生成（媒体库里就此少一部），日志里也找不到线索。
			slog.Warn("cloud strm: map path failed, file skipped", "file", f, "err", err)
			continue
		}
		if err := strmGen.UpsertStrm(ctx, cu.DriveID, cloudPath, cu.StrmOutputDir, cu.StrmTemplate); err != nil {
			slog.Warn("cloud strm upsert failed", "path", cloudPath, "err", err)
		}
	}
	// 上传即刷新媒体库（.strm 已可扫）。
	if cfg := transfer.Load(c.cfg); cfg.RefreshLibrary && c.mediaMS != nil {
		c.mediaMS.OnDownloadCompleted(ctx, resTitle(res))
	}
}

// CloudStrmSink STRM 记录/文件落地能力（*CloudChain 实现）。
type CloudStrmSink interface {
	UpsertStrm(ctx context.Context, driveID uint, cloudPath, outputDir, template string) error
}

// cloudPathFor 计算本地整理文件对应的云盘路径：从 file 中剥掉本地库前缀（MovieDir/TVDir
// [+type/category 子目录]），再拼到 CloudBase 下——保留「标题 (年份)/Season xx/」完整结构。
// 此前用 Rel(DestDir, file) 只留季/文件名，不同影片在云盘根互相覆盖（目录塌平）。
// DestDir 形如 <库>/<movie|tv>/[分类/]<Title (Year)>；取其末段与 file 去重尾比对，
// 兼容 Windows 分隔符。匹配失败回退「DestDir 末段 + 文件相对 DestDir 路径」（不塌平）。
func cloudPathFor(cu transfer.CloudUpload, destDir, file string) (string, error) {
	rel, relErr := relToLibrary(cu.MovieDir, file)
	if relErr != nil {
		rel, relErr = relToLibrary(cu.TVDir, file)
	}
	if relErr != nil {
		// 库目录配了路径映射等场景拿不到公共前缀 → 回退 DestDir 相对路径，保住标题层。
		sub, err := filepath.Rel(destDir, file)
		if err != nil {
			return "", err
		}
		rel = path.Join(filepath.ToSlash(filepath.Base(destDir)), filepath.ToSlash(sub))
	}
	base := strings.TrimRight(cloud.CleanPath(cu.CloudBase), "/")
	return base + "/" + strings.TrimLeft(rel, "/"), nil
}

// relToLibrary 求 file 相对库根（可含 type/category 子目录）的斜杠路径；
// 剥离传入路径中的盘符前缀差异后按 / 归一化做尾部对齐，不要求严格前缀包含。
func relToLibrary(libDir, file string) (string, error) {
	lib := strings.TrimRight(strings.ReplaceAll(strings.TrimSpace(libDir), `\`, "/"), "/")
	if lib == "" {
		return "", fmt.Errorf("库目录为空")
	}
	f := strings.TrimRight(strings.ReplaceAll(file, `\`, "/"), "/")
	dropDrive := func(p string) string {
		if len(p) >= 2 && p[1] == ':' {
			p = p[2:]
		}
		return p
	}
	fSegs := strings.Split(strings.TrimLeft(dropDrive(f), "/"), "/")
	libSegs := strings.Split(strings.TrimLeft(dropDrive(lib), "/"), "/")
	if len(fSegs) <= len(libSegs) {
		return "", fmt.Errorf("文件 %s 不在库 %s 下", f, lib)
	}
	for i, seg := range libSegs {
		if !strings.EqualFold(seg, fSegs[i]) {
			return "", fmt.Errorf("文件 %s 与库前缀 %s 不匹配", f, lib)
		}
	}
	return path.Join(fSegs[len(libSegs):]...), nil
}

// isSubtitleOrImage 字幕/海报等附属文件（UploadVideos=false 时仅传这类）。
func isSubtitleOrImage(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".srt", ".ass", ".ssa", ".sub", ".idx", ".nfo", ".jpg", ".jpeg", ".png", ".webp":
		return true
	}
	return false
}

// cloudTaskQueue 任务队列注入点（app 装配后非 nil）。
func (c *TransferChain) cloudTaskQueue() CloudTaskEnqueuer {
	if e, ok := c.cloudUp.(CloudTaskEnqueuer); ok {
		return e
	}
	return nil
}

// CloudTaskEnqueuer 云端任务入队能力（*CloudChain 实现）。
type CloudTaskEnqueuer interface {
	EnqueueCloudTask(t *models.CloudTask) error
}

// resTitle 整理结果标题（通知用）。
func resTitle(res *transfer.Result) string {
	if res.Media != nil && res.Media.Title != "" {
		return res.Media.Title
	}
	return "未知媒体"
}
