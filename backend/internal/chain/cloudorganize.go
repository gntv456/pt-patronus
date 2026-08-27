package chain

import (
	"context"
	"fmt"
	"strings"

	"github.com/hxpt/ptpatronus/internal/cloud"
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
		return nil, fmt.Errorf("媒体识别服务未配置")
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

// RepairStrm 按当前 AppBaseURL/模板重写全部（或指定网盘的）已登记 .strm 文件。
// 场景：服务地址变更（相对→绝对）、模板调整、网盘迁移后批量修复失效 strm。
// 仅处理记录里带 StrmPath 的条目（规则生成落盘的）；登记型（无文件）跳过。
func (c *CloudChain) RepairStrm(ctx context.Context, driveID uint) (repaired, skipped int, err error) {
	recs, err := c.ruleOp.ListRecords(driveID)
	if err != nil {
		return 0, 0, err
	}
	// 规则模板：取该网盘的首条规则（无规则用默认 direct 模板）
	tmpl := transferDefaultStrmTemplate
	if rules, rerr := c.ruleOp.List(); rerr == nil {
		for _, r := range rules {
			if driveID == 0 || r.DriveID == driveID {
				if r.Template != "" {
					tmpl = r.Template
				}
				break
			}
		}
	}
	for _, rec := range recs {
		if rec.StrmPath == "" {
			skipped++
			continue
		}
		werr := c.UpsertStrm(ctx, rec.DriveID, rec.CloudPath, dirOf(rec.StrmPath), tmpl)
		if werr != nil {
			skipped++
			continue
		}
		repaired++
	}
	return repaired, skipped, nil
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
