package chain

import (
	"context"
	"fmt"
	"log/slog"
	"path"
	"strconv"
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
	cli, err := cloud.NewClient(cloud.Drive{
		ID: m.ID, Type: m.Type, Token: token, RootPath: m.RootPath,
	}, c.hc)
	if err != nil {
		return nil, nil, err
	}
	return cli, m, nil
}

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

// Browse 列目录（UI 文件夹选择器用）。path 为云盘绝对路径（以 / 起）。
func (c *CloudChain) Browse(ctx context.Context, id uint, p string) ([]cloud.File, error) {
	cli, m, err := c.client(id)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(p) == "" {
		p = m.RootPath
	}
	files, err := cli.List(ctx, p)
	if err != nil {
		return nil, err
	}
	if files == nil {
		files = []cloud.File{}
	}
	return files, nil
}

// ---- STRM 规则 CRUD ----

func (c *CloudChain) ListRules() ([]models.StrmRule, error)          { return c.ruleOp.List() }
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
func (c *CloudChain) GenerateStrm(ctx context.Context, ruleID uint) (cloud.GenerateResult, error) {
	rule, err := c.ruleOp.Get(ruleID)
	if err != nil {
		return cloud.GenerateResult{}, err
	}
	cli, _, err := c.client(rule.DriveID)
	if err != nil {
		return cloud.GenerateResult{}, err
	}
	return cloud.Generate(ctx, cli, cloud.GenerateOptions{
		DriveID:   rule.DriveID,
		CloudPath: rule.CloudPath,
		OutputDir: rule.OutputDir,
		MountRoot: rule.MountRoot,
		Template:  rule.Template,
		Recursive: rule.Recursive,
		KeepExt:   rule.KeepExt,
		OnFile: func(written bool, cloudPath, strmPath string, size int64) {
			if _, err := c.ruleOp.UpsertRecord(rule.DriveID, cloudPath, strmPath, size, 0); err != nil {
				slog.Debug("cloud: upsert strm record failed", "err", err)
			}
		},
	})
}

// GenerateInput 即时 STRM 生成参数（文件页选中目录直接生成，不持久化为规则）。
type GenerateInput struct {
	DriveID   uint   `json:"drive_id"`
	CloudPath string `json:"cloud_path"`
	OutputDir string `json:"output_dir"`
	MountRoot string `json:"mount_root"`
	Template  string `json:"template"`
	Recursive bool   `json:"recursive"`
	KeepExt   bool   `json:"keep_ext"`
}

// GenerateInstant 即时生成 STRM（不持久化规则；仍 upsert 记录便于追溯删除）。
func (c *CloudChain) GenerateInstant(ctx context.Context, in GenerateInput) (cloud.GenerateResult, error) {
	cli, _, err := c.client(in.DriveID)
	if err != nil {
		return cloud.GenerateResult{}, err
	}
	return cloud.Generate(ctx, cli, cloud.GenerateOptions{
		DriveID:   in.DriveID,
		CloudPath: in.CloudPath,
		OutputDir: in.OutputDir,
		MountRoot: in.MountRoot,
		Template:  in.Template,
		Recursive: in.Recursive,
		KeepExt:   in.KeepExt,
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
			slog.Debug("cloud: link record failed", "err", err)
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

// matchAnySubscription 按文件名归一化匹配任一活跃订阅（标题为文件名子串即命中）。
func matchAnySubscription(subs []models.Subscription, filename string) *models.Subscription {
	nf := normalizeName(filename)
	if nf == "" {
		return nil
	}
	ftSet := make(map[string]bool)
	for _, t := range strings.Fields(nf) {
		ftSet[t] = true
	}
	for i := range subs {
		t := normalizeName(subs[i].Title)
		if t == "" {
			continue
		}
		titleTokens := strings.Fields(t)
		if len(titleTokens) == 0 {
			continue
		}
		// 订阅标题的每个 token 都须作为独立词出现在文件名（避免裸子串误匹配：
		// "It" 命中 "Comm**it**ments"、"Her" 命中 "Mot**her**"）。
		all := true
		for _, tt := range titleTokens {
			if !ftSet[tt] {
				all = false
				break
			}
		}
		if all {
			return &subs[i]
		}
	}
	return nil
}

// normalizeName 归一化文件名/标题用于匹配：去扩展名、小写、分隔符→空格、剥离分辨率/编码/年份噪声。
func normalizeName(s string) string {
	s = strings.ToLower(s)
	// 去扩展名：仅当尾段像扩展名（≤4 且纯字母数字，如 mkv/mp4/srt），避免误吃标题中的点分段。
	if i := strings.LastIndex(s, "."); i > 0 {
		tail := s[i+1:]
		if len(tail) <= 4 && isAlnum(tail) {
			s = s[:i]
		}
	}
	// 分隔符 → 空格
	s = strings.NewReplacer(".", " ", "_", " ", "-", " ", "[", " ", "]", " ", "(", " ", ")", " ").Replace(s)
	// 剥离常见噪声 token
	noise := []string{
		"1080p", "2160p", "4k", "720p", "480p",
		"x264", "h264", "x265", "h265", "hevc", "avc",
		"bluray", "blu-ray", "webrip", "web-dl", "webdl", "hdrip", "dvdrip",
		"10bit", "sdr", "hdr", "dts", "aac", "ac3", "flac", "mp3",
		"remux", "chs", "cht", "gb", "big5",
	}
	words := strings.Fields(s)
	out := make([]string, 0, len(words))
	for _, w := range words {
		clean := strings.Trim(w, ".,;:!?\"'")
		if clean == "" {
			continue
		}
		if isNoise(clean, noise) {
			continue
		}
		// 纯年份 token（1900-2099）剥离
		if isYear(clean) {
			continue
		}
		out = append(out, clean)
	}
	return strings.Join(out, " ")
}

func isNoise(w string, list []string) bool {
	for _, n := range list {
		if w == n {
			return true
		}
	}
	return false
}

func isYear(w string) bool {
	if len(w) != 4 {
		return false
	}
	y, err := strconv.Atoi(w)
	return err == nil && y >= 1900 && y <= 2099
}

// isAlnum 全小写字母数字（扩展名尾段判定）。
func isAlnum(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}
