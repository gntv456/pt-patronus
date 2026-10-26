package chain

import (
	"context"
	"errors"
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

	"gorm.io/gorm"
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
	directSignKey     []byte              // 直链签名密钥（nil=兼容模式不签名不校验）
	directSignLegacy  []byte              // #406 升级前的原始 SECRET_KEY 字节，只用于**校验**存量 .strm
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

func (c *CloudChain) Get(id uint) (*models.CloudDrive, error) { return c.op.Get(id) }

func (c *CloudChain) Create(in oper.CloudDriveInput) (*models.CloudDrive, error) {
	return c.op.Create(in)
}

func (c *CloudChain) Update(id uint, in oper.CloudDriveInput) (*models.CloudDrive, error) {
	return c.op.Update(id, in)
}

// Delete 删网盘并级联清理其 STRM 规则与记录。
// 四轮 P2 修复：三删改单事务——此前分三条 SQL，中间步失败仅 Warn 继续走，SQLite
// busy 下可留下孤儿规则/记录（残留规则每轮 cron 刷 "record not found" Warn）。
func (c *CloudChain) Delete(id uint) error {
	// 存在性判定收在这一处：HTTP 面与插件工具 cloud_delete 都调它。
	// GORM 删未命中主键不报错，不先查就分不清「删掉了」与「本来就没有」。
	if _, err := c.op.Get(id); err != nil {
		return err
	}
	return c.op.DB().Transaction(func(tx *gorm.DB) error {
		if err := c.ruleOp.WithTx(tx).DeleteRulesByDrive(id); err != nil {
			return err
		}
		if err := c.ruleOp.WithTx(tx).DeleteRecordsByDrive(id); err != nil {
			return err
		}
		return c.op.WithTx(tx).Delete(id)
	})
}

// client 为指定网盘构建 client（解密 token）。
func (c *CloudChain) client(id uint) (cloud.Client, *models.CloudDrive, error) {
	m, err := c.op.Get(id)
	if err != nil {
		return nil, nil, err
	}
	token, err := c.op.Token(m)
	if err != nil {
		// 换过主密钥后库里令牌还在却解不开：从前只留一句 "解密凭据失败: %w"，
		// 接口层把它压成 400「参数有误」，于是拿着**空令牌**去打云盘这条路没人拦——
		// 整理/修 STRM 是会写云盘的操作，可能改坏一半而用户不知道要重新保存凭据。
		return nil, nil, fmt.Errorf("%w：网盘「%s」的令牌无法解密（主密钥可能已更换）——请到「云盘」里重新保存该盘凭据；本次没有向云盘发请求（%v）",
			ErrCloudTokenUnreadable, m.Name, err)
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

// strmWrite 单次 .strm 登记+落盘的完整参数（输出定位 + 规则级渲染上下文）。
type strmWrite struct {
	OutputDir     string // 空=仅登记记录不落文件
	Template      string // 空=上传默认模板（{direct}{cloudpath}）
	MountRoot     string // {mount}（规则/修复路径用）
	LocalBaseURL  string // {direct} 基址（规则/修复路径用）
	PublicBaseURL string // {directPublic} 基址（规则/修复路径用）
	OutPath       string // 非空=直写该路径（repair 沿用既有 .strm 文件名）
	StructRoot    string // 结构镜像根（{relpath} 相对路径基准）
	Size          int64  // 记录 size（repair 沿用旧值）
}

// UpsertStrm 登记云盘文件为 StrmRecord，并按需在本地 outputDir 生成 .strm 文件
// （内容指向 302 直链端点）。structRoot 非空时镜像云盘目录结构到 outputDir 下；
// outPath 非空时直写该路径（repair 沿用记录里的既有 .strm 文件名）。
func (c *CloudChain) UpsertStrm(ctx context.Context, driveID uint, cloudPath, outputDir, template string, structRoot ...string) error {
	w := strmWrite{OutputDir: outputDir, Template: template}
	if len(structRoot) > 0 {
		w.StructRoot = structRoot[0]
	}
	return c.upsertStrm(ctx, driveID, cloudPath, w)
}

// upsertStrm 先落盘后退一登记记录：.strm 写成功才 upsert，写失败不动记录
// （此前先 UpsertRecord 清空 strm_path/size 再试写，写失败后记录永久退化为
// 「登记型」，repair/resign 永不再修）；成功则回填实际 .strm 路径与大小，
// RepairStrm/ResignAllStrm 才能据此续签（否则上传型 .strm 到期 404 无人重签）。
func (c *CloudChain) upsertStrm(ctx context.Context, driveID uint, cloudPath string, w strmWrite) error {
	cloudPath = cloud.CleanPath(cloudPath)
	if w.OutputDir == "" {
		_, err := c.ruleOp.UpsertRecord(driveID, cloudPath, "", 0, 0)
		return err // 仅登记记录（直链端点可用），不落 .strm 文件
	}
	if w.Template == "" {
		w.Template = transferDefaultStrmTemplate
	}
	wo := cloud.WriteStrmOptions{
		OutputDir:  w.OutputDir,
		CloudPath:  cloudPath,
		Template:   w.Template,
		FileName:   path.Base(cloudPath),
		DriveID:    driveID,
		DirectBase: "/api/v1/cloud/direct",
		AppBaseURL: c.appBaseURL,
		OutPath:    w.OutPath,
		// 限时直链签名：URL 即凭证，防匿名枚举 driveId+路径换全盘直链。
		SignFunc: c.SignDirectLink,
		// 规则级渲染上下文（修复链路按记录所属规则原样重渲染）。
		MountRoot:     w.MountRoot,
		LocalBaseURL:  w.LocalBaseURL,
		PublicBaseURL: w.PublicBaseURL,
		StructRoot:    w.StructRoot,
	}
	strmPath, err := cloud.WriteStrmFile(wo)
	if err != nil {
		return err
	}
	_, err = c.ruleOp.UpsertRecord(driveID, cloudPath, strmPath, w.Size, 0)
	return err
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

// ErrCloudQueueDisabled 云端任务队列没装配（未注入 oper / 功能没开）。
// 发哨兵而不是裸中文：调用方要按「配置状态」和「库写失败」分两档回，
// 否则两者在界面上都是同一句「请求参数有误」。
var ErrCloudQueueDisabled = errors.New("云端任务队列未启用，请在设置里开启后重试")

// EnqueueCloudTask 入队云端任务（自动上传链路调用；执行器消费）。
func (c *CloudChain) EnqueueCloudTask(t *models.CloudTask) error {
	if c.taskOp == nil {
		return ErrCloudQueueDisabled
	}
	return c.taskOp.Enqueue(t)
}

// SetTransferConfigGetter 注入 transfer.config 读取（任务尾部生成 STRM 用）。
func (c *CloudChain) SetTransferConfigGetter(g KVGetter) { c.transferCfgGetter = g }

// SetCloudTaskOper 注入任务操作并启动执行器（app 装配期一次）。
func (c *CloudChain) SetCloudTaskOper(taskOp *oper.CloudTaskOper) {
	c.taskOp = taskOp
	c.StartCloudTaskExecutor(taskOp)
}

// StopCloudTaskExecutor 停止云端任务执行器（app Shutdown 调用）。
// 幂等：未启动/已停直接返回。在途任务的 ctx 不随停机取消（execute 自带 2h 上限），
// goroutine 在当前任务完成后随 stop 信号退出；下次启动由 RequeueStaleRunning 兜底恢复。
func (c *CloudChain) StopCloudTaskExecutor() {
	if c.tasks == nil || c.tasks.stop == nil {
		return
	}
	select {
	case <-c.tasks.stop:
		// 已关闭（幂等）
	default:
		close(c.tasks.stop)
	}
}

// CloudTasks 任务列表（API 用）。
func (c *CloudChain) CloudTasks(status string, limit int) ([]models.CloudTask, error) {
	if c.taskOp == nil {
		return []models.CloudTask{}, nil
	}
	return c.taskOp.List(status, limit)
}

// CountCloudTasks 任务总数（列表账面用）。未装配 taskOp 时如实交回错误，让上层保守报
// 「可能被截断」——返回空列表配上「就这些」是把装配缺口渲染成「队列已空」。
func (c *CloudChain) CountCloudTasks(status string) (int64, error) {
	if c.taskOp == nil {
		return 0, errors.New("云盘任务存储未启用：请先到「设置 · 云盘」配好云盘，再发起上传任务")
	}
	return c.taskOp.Count(status)
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
func (c *CloudChain) GetRule(id uint) (*models.StrmRule, error) { return c.ruleOp.Get(id) }

// DeleteRule 删 STRM 规则。存在性判定收在这一层：HTTP 面与插件工具 cloud_rule_delete 都调它，
// 写在 handler 里会让工具面继续把「本来就没有」报成已删除。
func (c *CloudChain) DeleteRule(id uint) error {
	if _, err := c.ruleOp.Get(id); err != nil {
		return err
	}
	return c.ruleOp.Delete(id)
}

// ---- STRM 记录 ----

func (c *CloudChain) ListRecords(driveID uint) ([]models.StrmRecord, error) {
	return c.ruleOp.ListRecords(driveID)
}

// DeleteRecord 删一条 STRM 记录（同上，判定在链层一处）。
func (c *CloudChain) DeleteRecord(id uint) error {
	if _, err := c.ruleOp.GetRecord(id); err != nil {
		return err
	}
	return c.ruleOp.DeleteRecord(id)
}
