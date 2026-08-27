package chain

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/cloud"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/notice"
)

// CloudChain 编排网盘（115/夸克/123）：CRUD 透传 + 按需构建 client（解密 cookie/token）
// 调网盘 API（列目录/配额/探活）+ STRM 生成（规则驱动）+ 订阅追更（STRM 记录匹配订阅→链接+通知）。
//
// 与 MediaServerChain 同构：client 按需构建（个位数网盘，构造廉价）。区别：网盘为公网站点，
// 复用一个 SSRF 守卫的 HTTP 客户端（AllowPrivate=false，Do 以 ssrf=true 走公网守卫，同站点抓取范式）。
type CloudChain struct {
	Base
	op     *oper.CloudDriveOper
	ruleOp *oper.StrmRuleOper
	sub    *SubscribeChain
	notice *notice.Service
	hc     *httpclient.Client

	tasks             *CloudTaskRunner    // 云端任务执行器（nil=未启动）
	taskOp            *oper.CloudTaskOper // 任务操作（入队用）
	transferCfgGetter KVGetter            // transfer.config 读取（任务尾部生成 STRM 用）
	appBaseURL        string              // 服务对外基址（strm 绝对 URL 渲染；空=相对）
	libRefresher      func(name string)   // 媒体库刷新钩子（upload 任务尾部；nil 跳过）
	mediaSearch       func(ctx context.Context, keyword string, year int, filter string) (*MediaSearchItem, error)
}

// KVGetter SystemConfigOper 最小接口（断 transfer 依赖环）。
type KVGetter interface {
	Get(key string) (string, error)
}

// NewCloudChain 构造；内部构建 SSRF 守卫的公网 HTTP 客户端（网盘为公网站点）。
func NewCloudChain(b Base, op *oper.CloudDriveOper, ruleOp *oper.StrmRuleOper, sub *SubscribeChain, noticeSvc *notice.Service) (*CloudChain, error) {
	hc, err := httpclient.New(httpclient.Options{Timeout: 30 * time.Second, MaxRetries: 2})
	if err != nil {
		return nil, err
	}
	return &CloudChain{Base: b, op: op, ruleOp: ruleOp, sub: sub, notice: noticeSvc, hc: hc}, nil
}

// ---- 网盘 CRUD 透传 ----

func (c *CloudChain) List() ([]models.CloudDrive, error) { return c.op.List() }

func (c *CloudChain) Create(in oper.CloudDriveInput) (*models.CloudDrive, error) {
	return c.op.Create(in)
}

func (c *CloudChain) Update(id uint, in oper.CloudDriveInput) (*models.CloudDrive, error) {
	return c.op.Update(id, in)
}

// Delete 删网盘并级联清理其 STRM 规则与记录。
func (c *CloudChain) Delete(id uint) error {
	if err := c.ruleOp.DeleteRulesByDrive(id); err != nil {
		slog.Warn("cloud: cascade delete rules failed", "drive", id, "err", err)
	}
	if err := c.ruleOp.DeleteRecordsByDrive(id); err != nil {
		slog.Warn("cloud: cascade delete records failed", "drive", id, "err", err)
	}
	return c.op.Delete(id)
}

// client 为指定网盘构建 client（解密 token）。
func (c *CloudChain) client(id uint) (cloud.Client, *models.CloudDrive, error) {
	m, err := c.op.Get(id)
	if err != nil {
		return nil, nil, err
	}
	token, err := c.op.Token(m)
	if err != nil {
		return nil, nil, fmt.Errorf("解密凭据失败: %w", err)
	}
	d := cloud.Drive{ID: m.ID, Type: m.Type, Token: token, RootPath: m.RootPath}
	cli, err := cloud.NewClient(d, c.hc)
	if err != nil {
		return nil, nil, err
	}
	// 开放平台类网盘：注入令牌刷新回写（401 自愈；刷新后凭据持久化，重启不丢）
	if rf, ok := cli.(cloud.TokenRefresher); ok {
		rf.SetTokenRefreshHook(func(newToken string) {
			if err := c.op.SetToken(m.ID, newToken); err != nil {
				slog.Warn("cloud: token refresh persist failed", "drive", m.ID, "err", err)
			}
		})
	}
	return cli, m, nil
}

// ---- 302 直链解析 + 上传 + STRM 落地 ----

// ResolveDirectLink 把「网盘 id + 云盘路径」解析为限时下载直链（公开 302 端点/媒体代理用）。
// 路径经 RootPath 钳制后实时列父目录按名解析（StrmRecord 命中时秒级；记录仅作索引，
// 直链端点每次现取——网盘直链限时且 UA 绑定，缓存无意义）。
func (c *CloudChain) ResolveDirectLink(ctx context.Context, driveID uint, cloudPath, ua string) (*cloud.DirectLink, error) {
	cli, drive, err := c.client(driveID)
	if err != nil {
		return nil, err
	}
	w, ok := cli.(cloud.Writer)
	if !ok {
		return nil, fmt.Errorf("网盘类型 %s 不支持直链解析", drive.Type)
	}
	return w.DownloadURL(ctx, clampCloudPath(drive.RootPath, cloudPath), ua)
}

// UploadFile 将本地文件上传到指定网盘路径（自动上传链路用）。已存在幂等成功。
func (c *CloudChain) UploadFile(ctx context.Context, driveID uint, cloudPath, localPath string, size int64) error {
	cli, _, err := c.client(driveID)
	if err != nil {
		return err
	}
	w, ok := cli.(cloud.Writer)
	if !ok {
		return fmt.Errorf("网盘类型不支持上传")
	}
	return w.Upload(ctx, cloud.CleanPath(cloudPath), cloud.NewLocalSource(localPath, size))
}

// transferDefaultStrmTemplate 自动上传链路的 .strm 默认模板。
const transferDefaultStrmTemplate = "{direct}{cloudpath}"

// UpsertStrm 登记云盘文件为 StrmRecord，并按需在本地 outputDir 生成 .strm 文件
// （内容指向 302 直链端点）。structRoot 非空时镜像云盘目录结构到 outputDir 下。
func (c *CloudChain) UpsertStrm(ctx context.Context, driveID uint, cloudPath, outputDir, template string, structRoot ...string) error {
	cloudPath = cloud.CleanPath(cloudPath)
	if _, err := c.ruleOp.UpsertRecord(driveID, cloudPath, "", 0, 0); err != nil {
		return err
	}
	if outputDir == "" {
		return nil // 仅登记记录（直链端点可用），不落 .strm 文件
	}
	if template == "" {
		template = transferDefaultStrmTemplate
	}
	w := cloud.WriteStrmOptions{
		OutputDir:  outputDir,
		CloudPath:  cloudPath,
		Template:   template,
		FileName:   path.Base(cloudPath),
		DriveID:    driveID,
		DirectBase: "/api/v1/cloud/direct",
		AppBaseURL: c.appBaseURL,
	}
	if len(structRoot) > 0 {
		w.StructRoot = structRoot[0]
	}
	return cloud.WriteStrmFile(w)
}

// Start115OpenAuth 发起 115 开放平台设备码授权。
func (c *CloudChain) Start115OpenAuth(ctx context.Context, appID string) (*cloud.DeviceCode115, string, error) {
	return cloud.StartDeviceCode115(ctx, c.hc, appID)
}

// Poll115OpenAuth 轮询授权状态；确认成功返回新令牌串（TokenPair JSON，调用方建盘/回写）。
func (c *CloudChain) Poll115OpenAuth(ctx context.Context, code *cloud.DeviceCode115, verifier string) (confirmed bool, status int, tokenJSON string, err error) {
	ok, st, tp, err := cloud.PollDeviceCode115(ctx, c.hc, code, verifier)
	if err != nil || !ok || tp == nil {
		return ok, st, "", err
	}
	return true, st, cloud.EncodeTokenPair(*tp), nil
}

// fileManager 为网盘构建文件管理器客户端（不支持的网盘返回明确错误）。
func (c *CloudChain) fileManager(id uint) (cloud.FileManager, *models.CloudDrive, error) {
	cli, m, err := c.client(id)
	if err != nil {
		return nil, nil, err
	}
	fm, ok := cli.(cloud.FileManager)
	if !ok {
		return nil, m, fmt.Errorf("网盘类型 %s 不支持文件管理操作", m.Type)
	}
	return fm, m, nil
}

// CloudMkdir 建目录（单层；幂等）。
func (c *CloudChain) CloudMkdir(ctx context.Context, id uint, parent, name string) error {
	fm, _, err := c.fileManager(id)
	if err != nil {
		return err
	}
	return fm.Mkdir(ctx, clampCloudPath(c.rootOf(id), parent), name)
}

// CloudMove 移动。
func (c *CloudChain) CloudMove(ctx context.Context, id uint, src, dstDir string) error {
	fm, _, err := c.fileManager(id)
	if err != nil {
		return err
	}
	return fm.Move(ctx, clampCloudPath(c.rootOf(id), src), clampCloudPath(c.rootOf(id), dstDir))
}

// CloudCopy 复制。
func (c *CloudChain) CloudCopy(ctx context.Context, id uint, src, dstDir string) error {
	fm, _, err := c.fileManager(id)
	if err != nil {
		return err
	}
	return fm.Copy(ctx, clampCloudPath(c.rootOf(id), src), clampCloudPath(c.rootOf(id), dstDir))
}

// CloudRename 重命名。
func (c *CloudChain) CloudRename(ctx context.Context, id uint, src, newName string) error {
	fm, _, err := c.fileManager(id)
	if err != nil {
		return err
	}
	return fm.Rename(ctx, clampCloudPath(c.rootOf(id), src), newName)
}

// CloudDelete 删除。
func (c *CloudChain) CloudDelete(ctx context.Context, id uint, src string) error {
	fm, _, err := c.fileManager(id)
	if err != nil {
		return err
	}
	return fm.Delete(ctx, clampCloudPath(c.rootOf(id), src))
}

// CloudOfflineAdd 离线下载（磁力/URL → 云盘目录）。
func (c *CloudChain) CloudOfflineAdd(ctx context.Context, id uint, dir, rawURL string) error {
	fm, _, err := c.fileManager(id)
	if err != nil {
		return err
	}
	return fm.OfflineAdd(ctx, clampCloudPath(c.rootOf(id), dir), rawURL)
}

// CloudSaveShare 分享链接转存。
func (c *CloudChain) CloudSaveShare(ctx context.Context, id uint, dir, shareURL, pwd string) error {
	fm, _, err := c.fileManager(id)
	if err != nil {
		return err
	}
	return fm.SaveShare(ctx, clampCloudPath(c.rootOf(id), dir), shareURL, pwd)
}

// rootOf 网盘 RootPath（钳制用；查询失败返 "/"）。
func (c *CloudChain) rootOf(id uint) string {
	if m, err := c.op.Get(id); err == nil {
		return m.RootPath
	}
	return "/"
}

// EnqueueCloudTask 入队云端任务（自动上传链路调用；执行器消费）。
func (c *CloudChain) EnqueueCloudTask(t *models.CloudTask) error {
	if c.taskOp == nil {
		return fmt.Errorf("云端任务队列未启用")
	}
	return c.taskOp.Enqueue(t)
}

// SetAppBaseURL 注入服务对外基址（strm 内容渲染绝对 URL；Emby 远端播放必需）。
func (c *CloudChain) SetAppBaseURL(u string) { c.appBaseURL = strings.TrimRight(u, "/") }

// SetTransferConfigGetter 注入 transfer.config 读取（任务尾部生成 STRM 用）。
func (c *CloudChain) SetTransferConfigGetter(g KVGetter) { c.transferCfgGetter = g }

// SetCloudTaskOper 注入任务操作并启动执行器（app 装配期一次）。
func (c *CloudChain) SetCloudTaskOper(taskOp *oper.CloudTaskOper) {
	c.taskOp = taskOp
	c.StartCloudTaskExecutor(taskOp)
}

// CloudTasks 任务列表（API 用）。
func (c *CloudChain) CloudTasks(status string, limit int) ([]models.CloudTask, error) {
	if c.taskOp == nil {
		return []models.CloudTask{}, nil
	}
	return c.taskOp.List(status, limit)
}

// RetryCloudTask 手动重试。
func (c *CloudChain) RetryCloudTask(id uint) error { return c.taskOp.Retry(id) }

// DeleteCloudTask 删除任务记录。
func (c *CloudChain) DeleteCloudTask(id uint) error { return c.taskOp.Delete(id) }

// Test 探活。
func (c *CloudChain) Test(ctx context.Context, id uint) (string, error) {
	cli, _, err := c.client(id)
	if err != nil {
		return "", err
	}
	return cli.Test(ctx)
}

// Quota 拉容量。
func (c *CloudChain) Quota(ctx context.Context, id uint) (*cloud.Quota, error) {
	cli, _, err := c.client(id)
	if err != nil {
		return nil, err
	}
	return cli.Quota(ctx)
}

// Browse 列目录（UI 文件夹选择器用）。path 钳制在 RootPath 子树内，防浏览越界
// （原先直传 cli.List 可浏览网盘 RootPath 之外目录，与只读 cloudBackend.List 不一致）。
func (c *CloudChain) Browse(ctx context.Context, id uint, p string) ([]cloud.File, error) {
	cli, m, err := c.client(id)
	if err != nil {
		return nil, err
	}
	p = clampCloudPath(m.RootPath, p)
	files, err := cli.List(ctx, p)
	if err != nil {
		return nil, err
	}
	if files == nil {
		files = []cloud.File{}
	}
	return files, nil
}

// clampCloudPath 把云盘路径 p 钳制在 root 子树内（与 filemanager.resolvePath 同语义）：
// path.Clean 抹 ../，再校验未逃出 root，越界则回 root。
func clampCloudPath(root, p string) string {
	root = strings.TrimRight(path.Clean("/"+strings.TrimLeft(root, "/")), "/")
	cleaned := path.Clean("/" + strings.TrimLeft(strings.TrimSpace(p), "/"))
	if root == "" || root == "/" {
		return cleaned
	}
	if cleaned == root || strings.HasPrefix(cleaned, root+"/") {
		return cleaned
	}
	return root
}

// ---- STRM 规则 CRUD ----

func (c *CloudChain) ListRules() ([]models.StrmRule, error) { return c.ruleOp.List() }
func (c *CloudChain) CreateRule(in oper.StrmRuleInput) (*models.StrmRule, error) {
	return c.ruleOp.Create(in)
}
func (c *CloudChain) UpdateRule(id uint, in oper.StrmRuleInput) (*models.StrmRule, error) {
	return c.ruleOp.Update(id, in)
}
func (c *CloudChain) DeleteRule(id uint) error { return c.ruleOp.Delete(id) }

// ---- STRM 记录 ----

func (c *CloudChain) ListRecords(driveID uint) ([]models.StrmRecord, error) {
	return c.ruleOp.ListRecords(driveID)
}
func (c *CloudChain) DeleteRecord(id uint) error { return c.ruleOp.DeleteRecord(id) }

// ---- STRM 生成 ----

// GenerateStrm 按单条规则扫描网盘生成本地 .strm。
// OutputTag 非空时优先解析目录标签（transfer.dir_tags，local 型）为输出目录——
// 标签不存在/类型不符时报错（显式失败优于静默落错目录）。
func (c *CloudChain) GenerateStrm(ctx context.Context, ruleID uint) (cloud.GenerateResult, error) {
	rule, err := c.ruleOp.Get(ruleID)
	if err != nil {
		return cloud.GenerateResult{}, err
	}
	cli, drive, err := c.client(rule.DriveID)
	if err != nil {
		return cloud.GenerateResult{}, err
	}
	outputDir := rule.OutputDir
	if rule.OutputTag != "" {
		tag, err := c.resolveLocalDirTag(rule.OutputTag)
		if err != nil {
			return cloud.GenerateResult{}, err
		}
		outputDir = tag
	}
	return cloud.Generate(ctx, cli, cloud.GenerateOptions{
		DriveID:         rule.DriveID,
		CloudPath:       clampCloudPath(drive.RootPath, rule.CloudPath),
		OutputDir:       outputDir,
		MountRoot:       rule.MountRoot,
		Template:        rule.Template,
		Recursive:       rule.Recursive,
		KeepExt:         rule.KeepExt,
		SkipExisting:    rule.Incremental,
		IncludeExts:     splitCSV(rule.IncludeExts),
		ExcludeKeywords: splitCSV(rule.ExcludeKeywords),
		LocalBaseURL:    rule.LocalBaseURL,
		PublicBaseURL:   rule.PublicBaseURL,
		FallbackBaseURL: c.appBaseURL,
		OnFile: func(written bool, cloudPath, strmPath string, size int64) {
			if _, err := c.ruleOp.UpsertRecord(rule.DriveID, cloudPath, strmPath, size, 0); err != nil {
				slog.Warn("cloud: upsert strm record failed", "err", err)
			}
		},
	})
}

// resolveLocalDirTag 解析 local 型目录标签为本机路径；cloud 型/不存在报错。
func (c *CloudChain) resolveLocalDirTag(name string) (string, error) {
	if c.transferCfgGetter == nil {
		return "", fmt.Errorf("目录标签解析不可用（未注入配置读取器）: %s", name)
	}
	tags := LoadDirTagsFromKV(c.transferCfgGetter)
	for _, t := range tags {
		if t.Name == name {
			if t.Kind != "local" {
				return "", fmt.Errorf("目录标签 %s 是 %s 型，STRM 输出目录需 local 型", name, t.Kind)
			}
			return t.Path, nil
		}
	}
	return "", fmt.Errorf("目录标签不存在: %s", name)
}

// splitCSV 逗号分隔字符串拆片（去空白；空串返回 nil）。
func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// dirTagItem 目录标签镜像（transfer.DirTag 的最小副本，避免 chain→transfer 依赖环；
// 字段与 transfer.dir_tags 存储格式严格一致）。
type dirTagItem struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	DriveID uint   `json:"drive_id,omitempty"`
	Path    string `json:"path"`
	Note    string `json:"note,omitempty"`
}

// LoadDirTagsFromKV 从 SystemConfig 读 transfer.dir_tags 标签表（读失败返回空）。
func LoadDirTagsFromKV(kv KVGetter) []dirTagItem {
	if kv == nil {
		return nil
	}
	raw, err := kv.Get("transfer.dir_tags")
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var ts []dirTagItem
	if json.Unmarshal([]byte(raw), &ts) != nil {
		return nil
	}
	return ts
}

// GenerateInput 即时 STRM 生成参数（文件页选中目录直接生成，不持久化为规则）。
type GenerateInput struct {
	DriveID         uint   `json:"drive_id"`
	CloudPath       string `json:"cloud_path"`
	OutputDir       string `json:"output_dir"`
	OutputTag       string `json:"output_tag"` // 输出目录标签（非空优先于 OutputDir）
	MountRoot       string `json:"mount_root"`
	Template        string `json:"template"`
	Recursive       bool   `json:"recursive"`
	KeepExt         bool   `json:"keep_ext"`
	IncludeExts     string `json:"include_exts"`     // 扩展名白名单（逗号分隔；空=全部）
	ExcludeKeywords string `json:"exclude_keywords"` // 文件名排除关键词（逗号分隔）
	LocalBaseURL    string `json:"local_base_url"`   // 内网基址（{direct} 用）
	PublicBaseURL   string `json:"public_base_url"`  // 外网基址（{directPublic} 用）
}

// GenerateInstant 即时生成 STRM（不持久化规则；仍 upsert 记录便于追溯删除）。
func (c *CloudChain) GenerateInstant(ctx context.Context, in GenerateInput) (cloud.GenerateResult, error) {
	cli, drive, err := c.client(in.DriveID)
	if err != nil {
		return cloud.GenerateResult{}, err
	}
	outputDir := in.OutputDir
	if in.OutputTag != "" {
		tag, err := c.resolveLocalDirTag(in.OutputTag)
		if err != nil {
			return cloud.GenerateResult{}, err
		}
		outputDir = tag
	}
	return cloud.Generate(ctx, cli, cloud.GenerateOptions{
		DriveID:         in.DriveID,
		CloudPath:       clampCloudPath(drive.RootPath, in.CloudPath),
		OutputDir:       outputDir,
		MountRoot:       in.MountRoot,
		Template:        in.Template,
		Recursive:       in.Recursive,
		KeepExt:         in.KeepExt,
		IncludeExts:     splitCSV(in.IncludeExts),
		ExcludeKeywords: splitCSV(in.ExcludeKeywords),
		LocalBaseURL:    in.LocalBaseURL,
		PublicBaseURL:   in.PublicBaseURL,
		FallbackBaseURL: c.appBaseURL,
		OnFile: func(written bool, cloudPath, strmPath string, size int64) {
			if _, err := c.ruleOp.UpsertRecord(in.DriveID, cloudPath, strmPath, size, 0); err != nil {
				slog.Debug("cloud: upsert strm record failed", "err", err)
			}
		},
	})
}

// GenerateAll 遍历启用规则生成 STRM（cron 入口）。单规则失败不阻断其余。
func (c *CloudChain) GenerateAll(ctx context.Context) {
	rules, err := c.ruleOp.ListEnabled()
	if err != nil {
		slog.Warn("cloud: list rules failed", "err", err)
		return
	}
	for _, r := range rules {
		res, err := c.GenerateStrm(ctx, r.ID)
		if err != nil {
			slog.Warn("cloud: generate rule failed", "rule", r.Name, "err", err)
			continue
		}
		slog.Info("cloud: strm generated", "rule", r.Name, "generated", res.Generated, "skipped", res.Skipped, "errors", len(res.Errors))
		// 本轮有新落地内容 → 触发一次媒体库扫库（生成与扫描解耦，别让 Emby 等 6h 下一轮）。
		if res.Generated > 0 && c.libRefresher != nil {
			c.libRefresher("strm:" + r.Name)
		}
		c.Emit(ctx, "CloudStrmGenerated", map[string]any{"rule": r.Name, "generated": res.Generated, "skipped": res.Skipped})
	}
}

// ---- 订阅追更 ----

// SyncSubscriptions 把已生成的 STRM 记录匹配到媒体订阅：命中即链接（记 SubscriptionID）+ 通知。
// 设计：追更复用「STRM 规则已扫描落库的 StrmRecord」（不重复扫盘），按文件名归一化匹配订阅标题。
// 即用户先用 STRM 规则覆盖媒体目录，追更自动把其中的资源链接到订阅并提醒。
func (c *CloudChain) SyncSubscriptions(ctx context.Context) {
	if c.sub == nil {
		return
	}
	subs, err := c.sub.List()
	if err != nil || len(subs) == 0 {
		return
	}
	active := make([]models.Subscription, 0, len(subs))
	for _, s := range subs {
		if s.Status == "subscribed" {
			active = append(active, s)
		}
	}
	if len(active) == 0 {
		return
	}
	recs, err := c.ruleOp.ListRecords(0) // driveID=0 = 全部
	if err != nil {
		slog.Warn("cloud: list strm records failed", "err", err)
		return
	}
	for _, rec := range recs {
		if rec.SubscriptionID != 0 {
			continue // 已链接
		}
		matched := matchAnySubscription(active, path.Base(rec.CloudPath))
		if matched == nil {
			continue
		}
		if err := c.ruleOp.LinkRecord(rec.ID, matched.ID); err != nil {
			slog.Warn("cloud: link record failed", "err", err)
			continue
		}
		if c.notice != nil {
			_ = c.notice.Notice(ctx, notice.Input{
				Source: "cloud",
				Key:    fmt.Sprintf("cloud:%d:%s", rec.DriveID, rec.CloudPath),
				Title:  fmt.Sprintf("订阅《%s》网盘资源已就绪", matched.Title),
				Body:   fmt.Sprintf("%s（已生成 STRM）", path.Base(rec.CloudPath)),
				Level:  "info",
			}, true)
		}
	}
}
