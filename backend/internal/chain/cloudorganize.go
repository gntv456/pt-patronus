package chain

import (
	"context"
	"fmt"
	"log/slog"
	"path"
	"strings"

	"github.com/hxpt/ptpatronus/internal/cloud"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/transfer"
)

// ---- 云端整理（chain 层编排） ----
//
// 识别能力经 SetMediaSearcher 注入（app 装配期包 MediaChain.Search，断依赖环）；
// 文件操作走各网盘 FileManager。API 侧可同步执行或入队为 CloudTask(kind=organize)。

// MediaSearchItem MediaChain.Search 返回子集（app 装配层填充；云端整理识别用）。
type MediaSearchItem struct {
	Title string
	Year  int
	Type  string // movie | tv
}

// organizerAdapter 搜索函数 → cloud.Organizer 适配（取首个结果；ParseMeta 先洗词）。
type organizerAdapter struct {
	search func(ctx context.Context, keyword string, year int, filter string) (*MediaSearchItem, error)
}

func (a organizerAdapter) Recognize(ctx context.Context, name string) (*cloud.OrganizeMeta, error) {
	kw := transfer.ParseMeta(name).Title
	if kw == "" {
		return nil, fmt.Errorf("无法从文件名提取搜索词")
	}
	it, err := a.search(ctx, kw, 0, "")
	if err != nil || it == nil {
		return nil, err
	}
	return &cloud.OrganizeMeta{Title: it.Title, Year: it.Year, Type: it.Type}, nil
}

// SetMediaSearcher 注入媒体搜索（云端整理识别用）。
func (c *CloudChain) SetMediaSearcher(f func(ctx context.Context, keyword string, year int, filter string) (*MediaSearchItem, error)) {
	c.mediaSearch = f
}

// OrganizeCloud 云端整理执行入口。
func (c *CloudChain) OrganizeCloud(ctx context.Context, in cloud.OrganizeOptions) (*cloud.OrganizeResult, error) {
	if c.mediaSearch == nil {
		return nil, fmt.Errorf("%w：云端整理要先能识别影片/剧集——请到「设置 → 媒体源」配好检索（或启用相关插件）后重试；本次没有向云盘发请求，文件未移动", ErrCloudMediaSearchUnconfigured)
	}
	cli, _, err := c.client(in.DriveID)
	if err != nil {
		return nil, err
	}
	root := c.rootOf(in.DriveID)
	in.SrcDir = clampCloudPath(root, in.SrcDir)
	if in.DstMovieDir != "" {
		in.DstMovieDir = clampCloudPath(root, in.DstMovieDir)
	}
	if in.DstTVDir != "" {
		in.DstTVDir = clampCloudPath(root, in.DstTVDir)
	}
	return cloud.Organize(ctx, cli, organizerAdapter{search: c.mediaSearch}, in)
}

// ---- STRM 修复 + 目录标签解析 ----

// RepairStrm 按「记录所属规则」忠实重渲染全部（或指定网盘的）已登记 .strm 文件。
// 场景：直链签名续签、服务地址变更（相对→绝对）、模板调整、网盘迁移后批量修复失效 strm。
// 仅处理记录里带 StrmPath 的条目（规则生成/上传落盘的）；登记型（无文件）跳过。
// 此前用「该盘首条规则模板 + 空 mount」重渲染：规则生成的 {mount} 内容被改写成裸云盘
// 路径、{directPublic} 留字面占位符、一盘多规则时套错模板——6h 一轮 cron 静默改坏全库。
// 现按记录 CloudPath 匹配覆盖规则（recursive 子树/同层取最具体）原样重渲染；匹配不到
// （上传型/即时生成）用上传链路模板（transfer.config.CloudUpload.StrmTemplate）。
// StrmRepairSummary 一次 STRM 修复的账面。写入失败此前混进 skipped ——
// 「修复 12 / 跳过 3」里的 3 个既可能是「登记型没有文件」（正常无事可做），
// 也可能是「.strm 写不进去」（还是坏的），界面与日志都分不出来。
//
// RulesUnavailable/RulesSkipped 是第三种混法（#291）：规则表这次读不出来时，规则型记录
// 会被保守跳过（拿不到 mount/双地址就不改写），这些 .strm **仍然是坏的**，
// 却和「无事可做」共用同一个 skipped —— 用户以为修复完成了。
type StrmRepairSummary struct {
	Repaired         int      `json:"repaired"`
	Skipped          int      `json:"skipped"`
	Failed           int      `json:"failed"`
	Errors           []string `json:"errors,omitempty"`
	RulesUnavailable bool     `json:"rules_unavailable"`
	RulesSkipped     int      `json:"rules_skipped"` // 含在 Skipped 里，单列出来才看得见
}

func (c *CloudChain) RepairStrm(ctx context.Context, driveID uint) (StrmRepairSummary, error) {
	var sum StrmRepairSummary
	recs, err := c.ruleOp.ListRecords(driveID)
	if err != nil {
		return sum, err
	}
	rules, rerr := c.ruleOp.List()
	if rerr != nil {
		// 规则读取失败退化为上传模板兜底；规则型记录的 mount/双地址无法忠实重渲染时
		// 宁可跳过（避免低口径改写），见下 defaultTemplate 回退判定。
		rules = nil
		sum.RulesUnavailable = true // 这份「读不出来」必须进账面，不能只被丢进日志
	}
	uploadTmpl := transferDefaultStrmTemplate
	if c.transferCfgGetter != nil {
		if t := strings.TrimSpace(transfer.Load(kvAsTransfer(c.transferCfgGetter)).CloudUpload.StrmTemplate); t != "" {
			uploadTmpl = t
		}
	}
	for _, rec := range recs {
		if rec.StrmPath == "" {
			sum.Skipped++
			continue
		}
		w := strmWrite{OutputDir: dirOf(rec.StrmPath), OutPath: rec.StrmPath, Template: uploadTmpl, Size: rec.Size}
		if r := matchStrmRule(rules, rec.DriveID, rec.CloudPath); r != nil {
			w.Template = r.Template
			if w.Template == "" {
				w.Template = cloud.DefaultStrmTemplate // 规则空模板=生成侧默认 {mount}{cloudpath}
			}
			w.MountRoot, w.LocalBaseURL, w.PublicBaseURL = r.MountRoot, r.LocalBaseURL, r.PublicBaseURL
			w.StructRoot = cloud.CleanPath(r.CloudPath) // {relpath} 基准与生成侧一致
		} else if rerr != nil {
			// 规则表读取失败且记录可能来自规则：拿不到规则上下文就跳过，防低口径改写。
			sum.Skipped++
			sum.RulesSkipped++
			continue
		}
		werr := c.upsertStrm(ctx, rec.DriveID, rec.CloudPath, w)
		if werr != nil {
			// 写失败是失败，不是「跳过」：记进 Failed 并留原因，日志逐条点名。
			sum.Failed++
			slog.Warn("cloud: strm repair write failed", "path", rec.StrmPath, "drive", rec.DriveID, "err", werr)
			if len(sum.Errors) < 5 {
				sum.Errors = append(sum.Errors, rec.StrmPath+": "+werr.Error())
			}
			continue
		}
		sum.Repaired++
	}
	return sum, nil
}

// matchStrmRule 取覆盖该云盘路径的规则：recursive=子树任意深度；非 recursive=同层文件。
// 多条命中取 CloudPath 最长（最具体）。无匹配返回 nil（上传型 .strm，调用方按上传模板兜底）。
func matchStrmRule(rules []models.StrmRule, driveID uint, cloudPath string) *models.StrmRule {
	cp := cloud.CleanPath(cloudPath)
	var best *models.StrmRule
	bestLen := -1
	for i := range rules {
		r := &rules[i]
		if r.DriveID != driveID {
			continue
		}
		rc := cloud.CleanPath(r.CloudPath)
		if cp != rc && !strings.HasPrefix(cp, strings.TrimRight(rc, "/")+"/") {
			continue
		}
		if !r.Recursive && path.Dir(cp) != rc {
			continue // 非递归规则只覆盖同层文件
		}
		if len(rc) > bestLen {
			best, bestLen = r, len(rc)
		}
	}
	return best
}

// dirOf 本地路径父目录（跨平台分隔符）。
func dirOf(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i > 0 {
		return p[:i]
	}
	return "/"
}

// ResolveTagForCloud 目录标签 → (driveID, 云盘路径)。标签 kind=cloud 时生效。
func (c *CloudChain) ResolveTagForCloud(name string) (uint, string, error) {
	if c.transferCfgGetter == nil {
		return 0, "", fmt.Errorf("配置服务未注入")
	}
	t, err := transfer.ResolveDirTag(kvAsTransfer(c.transferCfgGetter), name)
	if err != nil {
		return 0, "", err
	}
	if t.Kind != "cloud" {
		return 0, "", fmt.Errorf("标签 %s 不是云盘目录", name)
	}
	return t.DriveID, t.Path, nil
}

// kvAsTransfer KVGetter → transfer.KVStore 适配（两接口同形）。
func kvAsTransfer(g KVGetter) transfer.KVStore {
	return kvAdapter{g: g}
}

type kvAdapter struct{ g KVGetter }

func (a kvAdapter) Get(key string) (string, error) { return a.g.Get(key) }
func (a kvAdapter) Set(key, val string) error      { return fmt.Errorf("只读适配") }
