package chain

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
// 四轮 P2 修复：三删改单事务——此前分三条 SQL，中间步失败仅 Warn 继续走，SQLite
// busy 下可留下孤儿规则/记录（残留规则每轮 cron 刷 "record not found" Warn）。
func (c *CloudChain) Delete(id uint) error {
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
// （内容指向 302 直链端点）。structRoot 非空时镜像云盘目录结构到 outputDir 下；
// outPath 非空时直写该路径（repair 沿用记录里的既有 .strm 文件名）。
func (c *CloudChain) UpsertStrm(ctx context.Context, driveID uint, cloudPath, outputDir, template string, structRoot ...string) error {
	return c.upsertStrm(ctx, driveID, cloudPath, outputDir, template, "", structRoot...)
}

func (c *CloudChain) upsertStrm(ctx context.Context, driveID uint, cloudPath, outputDir, template, outPath string, structRoot ...string) error {
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
		OutPath:    outPath,
		// 限时直链签名：URL 即凭证，防匿名枚举 driveId+路径换全盘直链。
		SignFunc: c.SignDirectLink,
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

// SetDirectLinkSignKey 注入直链签名密钥（与 JWT/AES 同源即可）。非空时：
//   - .strm 内容的 {direct} URL 追加 ?sign=<exp>.<hmac>（默认 12h 有效）；
//   - 公开 302 端点校验签名，签名缺失/过期/不匹配一律 404。
//
// 收敛「匿名枚举 driveId+路径换全盘直链」面：URL 本身即凭证。过期后播放器 404，
// 由 STRM 重新生成/扫库刷新。兼容：密钥未注入（安全模式/旧库）不签名也不校验。
func (c *CloudChain) SetDirectLinkSignKey(key []byte) { c.directSignKey = key }

// 二轮 P0-2 修复：TTL 从 12h 延长到 7 天——上传型 .strm 只在上传瞬间写一次，
// 12h 意味着每天两次全库 404（唯一恢复是管理员手动 repair）。7 天对齐
// cloud-strm cron（默认 6h 一跑 + RepairStrm 兜底），URL 即凭证的安全模型
// 不变：泄露的 .strm URL 本来就在媒体库可读范围内。
const directLinkSignTTL = 7 * 24 * time.Hour

// SignDirectLink 生成 driveId|path|exp 的限时签名「<exp十进制>.<hmac-hex>」。
// 密钥未注入返回空串（不签名，兼容模式）。
func (c *CloudChain) SignDirectLink(driveID uint, cloudPath string) string {
	if len(c.directSignKey) == 0 {
		return ""
	}
	exp := time.Now().Add(directLinkSignTTL).Unix()
	return fmt.Sprintf("%d.%s", exp, c.directSignMAC(driveID, cloudPath, exp))
}

func (c *CloudChain) directSignMAC(driveID uint, cloudPath string, exp int64) string {
	mac := hmac.New(sha256.New, c.directSignKey)
	fmt.Fprintf(mac, "%d|%s|%d", driveID, cloudPath, exp)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyDirectLinkSign 校验「<exp>.<hmac>」签名（exp 未过 + 恒定时间 HMAC 匹配）。
// 密钥未注入时放行（兼容模式——与旧版行为一致）。
func (c *CloudChain) VerifyDirectLinkSign(driveID uint, cloudPath, sign string) bool {
	if len(c.directSignKey) == 0 {
		return true
	}
	dot := strings.IndexByte(sign, '.')
	if dot <= 0 {
		return false
	}
	var exp int64
	if _, err := fmt.Sscanf(sign[:dot], "%d", &exp); err != nil || time.Now().Unix() > exp {
		return false
	}
	want := c.directSignMAC(driveID, cloudPath, exp)
	return hmac.Equal([]byte(want), []byte(sign[dot+1:]))
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
