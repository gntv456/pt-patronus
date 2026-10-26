// Package backup 实现配置导出/导入：自身 JSON 格式（含站点/下载器）+ MoviePilot 站点导入。
package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
)

// SiteExport 站点导出项（密钥明文，由用户妥善保管备份文件）。
type SiteExport struct {
	Name       string `json:"name"`
	Platform   string `json:"platform"`
	Definition string `json:"definition"`
	BaseURL    string `json:"base_url"`
	Cookie     string `json:"cookie"`
	APIKey     string `json:"api_key"`
	Passkey    string `json:"passkey"`
	Username   string `json:"username"` // 站点账号（财神等 API 登录用）
	Password   string `json:"password"` // 站点密码（明文，由用户妥善保管）
	IsActive   bool   `json:"is_active"`
}

// DownloaderExport 下载器导出项。
type DownloaderExport struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	URL       string `json:"url"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	IsDefault bool   `json:"is_default"`
}

// Backup 是自身格式的备份结构。
type Backup struct {
	Version     int                `json:"version"`
	ExportedAt  time.Time          `json:"exported_at"`
	Sites       []SiteExport       `json:"sites"`
	Downloaders []DownloaderExport `json:"downloaders"`
}

const backupVersion = 1

// ErrNotBackupFile 文件是合法 JSON 但没有本程序的 version 标记：选错文件（别人的导出、
// 随手挑的 .json）不该被当成「空备份」报成功。
var ErrNotBackupFile = errors.New("不是 PTPatronus 导出的备份文件（缺少 version 标记）")

// ErrBackupVersionTooNew 备份来自更高版本的程序：本程序可能不认识里面的字段，
// 硬导入会静默丢配置，必须先升级。
var ErrBackupVersionTooNew = errors.New("备份版本高于当前程序，请先升级 PTPatronus 再导入")

// ErrNotMoviePilotFile 是「传进来的不是 MoviePilot 的站点 JSON」。
// 自有格式与 .ptd 在 #257 各有一条这种「该换个文件」的判据，MP 这一路当时一条都没加：
// 形状不对只剩 400「请求参数有误，请检查后重试」，用户既不知道要的是数组，也不知道去哪儿导出。
var ErrNotMoviePilotFile = errors.New("不是 MoviePilot 导出的站点 JSON")

// Service 备份服务。
type Service struct {
	siteOp *oper.SiteOper
	dlOp   *oper.DownloaderOper
	reg    *site.Registry // .ptd 恢复时按 URL 匹配站点定义（MatchByURL），避免全退化为默认 nexusphp
}

// New 构造。
func New(siteOp *oper.SiteOper, dlOp *oper.DownloaderOper, reg *site.Registry) *Service {
	return &Service{siteOp: siteOp, dlOp: dlOp, reg: reg}
}

// Export 导出为 JSON 字节。
func (s *Service) Export(ctx context.Context) ([]byte, error) {
	b := Backup{Version: backupVersion, ExportedAt: time.Now()}

	sites, err := s.siteOp.List()
	if err != nil {
		return nil, err
	}
	// 凭据解不开（主密钥换过 / 数据来自另一实例）绝不能写成空白继续：ExportToFile 之后只保留
	// 最新 backupKeepLatest 份，这份缺字段的备份会把唯一完好的旧备份挤掉轮换删除。
	var unreadable []string
	note := func(item string, err error) {
		if err == nil {
			return
		}
		slog.Warn("backup: credential undecryptable", "item", item, "err", err)
		unreadable = append(unreadable, item)
	}
	for _, st := range sites {
		cookie, cerr := s.siteOp.Cookie(&st)
		apiKey, aerr := s.siteOp.APIKey(&st)
		password, perr := s.siteOp.Password(&st)
		passkey, kerr := s.siteOp.Passkey(&st)
		note(st.Name+".Cookie", cerr)
		note(st.Name+".APIKey", aerr)
		note(st.Name+".Password", perr)
		note(st.Name+".Passkey", kerr)
		b.Sites = append(b.Sites, SiteExport{
			Name: st.Name, Platform: st.Platform, Definition: st.Definition,
			BaseURL: st.BaseURL, Cookie: cookie, APIKey: apiKey,
			Passkey: passkey, Username: st.Username,
			Password: password, IsActive: st.IsActive,
		})
	}

	if len(unreadable) > 0 {
		return nil, fmt.Errorf("备份中止：%d 项站点凭据无法解密（主密钥可能已更换，或数据来自其它实例）：%s"+
			"。强行导出会把这些字段写成空白，而备份轮换随后会删掉仍完好的旧备份——"+
			"请用原主密钥启动，或在「站点」里重新保存这些凭据后再备份",
			len(unreadable), strings.Join(firstOf(unreadable, 3), "、"))
	}

	dls, err := s.dlOp.List()
	if err != nil {
		return nil, err
	}
	for _, d := range dls {
		pw, pwErr := s.dlOp.Password(&d)
		note("下载器 "+d.Name+".Password", pwErr)
		b.Downloaders = append(b.Downloaders, DownloaderExport{
			Name: d.Name, Type: d.Type, URL: d.URL, Username: d.Username,
			Password: pw, IsDefault: d.IsDefault,
		})
	}
	if len(unreadable) > 0 {
		return nil, fmt.Errorf("备份中止：%d 项凭据无法解密（主密钥可能已更换，或数据来自其它实例）：%s"+
			"。强行导出会把这些字段写成空白，而备份轮换随后会删掉仍完好的旧备份——"+
			"请用原主密钥启动，或在「站点/下载器」里重新保存这些凭据后再备份",
			len(unreadable), strings.Join(firstOf(unreadable, 3), "、"))
	}
	return json.MarshalIndent(b, "", "  ")
}

// backupKeepLatest 定时备份保留的最新文件数。
const backupKeepLatest = 10

// ExportToFile 导出备份并写入 dir（ptpatronus-backup-<时间戳>.json），只保留最新
// backupKeepLatest 份（按文件名时间戳轮换，仅删除本工具命名的文件）。
// 备份含站点 Cookie/下载器密码明文：文件权限 0600、目录 0700，目录可信性由使用者保证。
// 供定时备份 cron 使用；返回写入路径。
func (s *Service) ExportToFile(ctx context.Context, dir string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("备份目录为空")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("创建备份目录: %w", err)
	}
	data, err := s.Export(ctx)
	if err != nil {
		return "", err
	}
	stamp := time.Now().Format("20060102-150405")
	// 四轮 P2 修复：后缀掺 UnixNano——此前 Stat-then-Write 防撞在并发导出（手动
	// 连点 + cron 同秒撞车）时双双 Stat 到不存在、选同一 -1 后缀互相覆盖丢一份。
	path := filepath.Join(dir, fmt.Sprintf("ptpatronus-backup-%s-%d.json", stamp, time.Now().UnixNano()%1e9))
	// 四轮 P2 修复：tmp+rename 原子写——直写崩溃会留截断 JSON 且被轮换策略当成
	// "最新"保留，灾难恢复时才发现坏。
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return "", fmt.Errorf("写备份文件: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("落盘备份文件: %w", err)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "ptpatronus-backup-*.json"))
	if len(matches) > backupKeepLatest {
		sort.Strings(matches) // 文件名时间戳字典序 = 时间序
		for _, old := range matches[:len(matches)-backupKeepLatest] {
			_ = os.Remove(old)
		}
	}
	return path, nil
}

// ImportResult 导入结果：成功计数 + 跳过/失败明细。
// 逐项失败过去被静默吞掉（只报成功数），用户无从察觉部分数据没恢复——明细随
// API 响应/通知回前端展示。
type ImportResult struct {
	Sites        int `json:"sites"`         // 成功导入的站点数
	Downloaders  int `json:"downloaders"`   // 成功导入的下载器数
	SitesSkipped int `json:"sites_skipped"` // 重复注册域/非法 URL 等被跳过的站点数
	// DownloadersSkipped：已存在（同类型+同 URL）而跳过的下载器数
	DownloadersSkipped int      `json:"downloaders_skipped"`
	DownloadersFailed  int      `json:"downloaders_failed"` // 创建失败的下载器数
	Failures           []string `json:"failures,omitempty"` // 失败原因（人类可读，最多 maxImportFailures 条）
}

// maxImportFailures 响应/通知携带的失败明细上限（计数不受限）。
const maxImportFailures = 10

// Import 从自身格式 JSON 导入（重复站点按注册域跳过，CreateMany 统一去重）。
func (s *Service) Import(ctx context.Context, data []byte) (*ImportResult, error) {
	var b Backup
	if err := json.Unmarshal(data, &b); err != nil {
		// 连本程序的结构都解析不了 = 不是我们的备份文件；原因留给日志（WithErr），
		// 用户只需要知道该换一个文件。
		return nil, fmt.Errorf("%w：%v", ErrNotBackupFile, err)
	}
	// 能解析成 JSON ≠ 是备份。修复前 `{}`、别人的配置导出、甚至一个空数组都会走
	// 「导入 0 个站点、0 个下载器」并回成功，界面与站内通知一起说「导入完成」，
	// 用户以为恢复做完了，实际什么都没进来。version 由 Export 恒置为 backupVersion。
	if b.Version == 0 {
		return nil, ErrNotBackupFile
	}
	if b.Version > backupVersion {
		return nil, fmt.Errorf("%w（备份 v%d，本程序最高 v%d）", ErrBackupVersionTooNew, b.Version, backupVersion)
	}
	siteIns := make([]oper.SiteInput, 0, len(b.Sites))
	for _, se := range b.Sites {
		defID := se.Definition
		if defID == "" {
			defID = s.matchDef(se.BaseURL) // 备份未带 definition → 按 URL 匹配 yaml
		}
		siteIns = append(siteIns, oper.SiteInput{
			Name: se.Name, Platform: se.Platform, Definition: defID,
			BaseURL: se.BaseURL, Cookie: se.Cookie, IsActive: se.IsActive,
			APIKey: se.APIKey, Passkey: se.Passkey,
			Username: se.Username, Password: se.Password,
		})
	}
	sites, err := s.siteOp.CreateMany(siteIns) // 单事务批量：N 次 fsync → 1 次
	if err != nil {
		return nil, err
	}
	res := &ImportResult{Sites: sites, SitesSkipped: len(siteIns) - sites}
	// 下载器按「类型+URL」去重：重复导入同一份备份（恢复演练/多机同步）从前会把
	// 同名同地址的下载器整份再建一遍（实测 2 条→4 条），下载器选择器里出现双胞胎。
	// 站点有注册域去重，下载器此前一条判据都没有。
	existing := map[string]bool{}
	if list, err := s.dlOp.List(); err != nil {
		slog.Warn("backup import: list downloaders failed, dedupe disabled",
			"err", err)
	} else {
		for i := range list {
			existing[downloaderIdentity(list[i].Type, list[i].URL)] = true
		}
	}
	for _, de := range b.Downloaders {
		key := downloaderIdentity(de.Type, de.URL)
		if existing[key] {
			res.DownloadersSkipped++
			continue
		}
		if _, e := s.dlOp.Create(oper.DownloaderInput{
			Name: de.Name, Type: de.Type, URL: de.URL, Username: de.Username,
			Password: de.Password, IsDefault: de.IsDefault,
		}); e != nil {
			// 失败必须计数上报：旧实现 continue 后成功数照报，坏条目静默消失。
			res.DownloadersFailed++
			if len(res.Failures) < maxImportFailures {
				res.Failures = append(res.Failures, fmt.Sprintf("下载器 %s: %v", de.Name, e))
			}
			continue
		}
		existing[key] = true // 同一份备份里重复条目也只建一次
		res.Downloaders++
	}
	return res, nil
}

// moviePilotSite 是 MoviePilot 站点配置的可识别子集。
type moviePilotSite struct {
	Name   string `json:"name"`
	URL    string `json:"url" json_url:"url"`
	Domain string `json:"domain"`
	Cookie string `json:"cookie"`
	Pri    int    `json:"pri"`
}

// ImportMoviePilot 从 MoviePilot 站点 JSON 数组导入站点。
func (s *Service) ImportMoviePilot(ctx context.Context, data []byte) (*ImportResult, error) {
	var list []moviePilotSite
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("%w：%v", ErrNotMoviePilotFile, err)
	}
	ins := make([]oper.SiteInput, 0, len(list))
	// 没有 name 的条目在下面被丢弃。从前这个 `continue` 不进任何账：
	// 用户拿一份 20 条、其中 8 条缺字段的 MP 导出，只会看到「已导入 12 个站点」，
	// 那 8 条去哪儿了没有任何交代（与自有格式已修的「跳过必须计数」同一条轴）。
	dropped := 0
	for _, m := range list {
		if m.Name == "" {
			dropped++
			continue
		}
		ins = append(ins, oper.SiteInput{
			Name: m.Name, Platform: "nexusphp", Definition: s.matchDef(m.URL),
			BaseURL: m.URL, Cookie: m.Cookie, IsActive: true,
		})
	}
	n, err := s.siteOp.CreateMany(ins)
	if err != nil {
		return nil, err
	}
	return &ImportResult{
		Sites: n,
		// 注册域/名称与现有站点重复的也不在成功数里，一起记进跳过账。
		SitesSkipped: dropped + (len(ins) - n),
	}, nil
}

// matchDef 按 baseURL 匹配站点定义（definitions/*.yaml 的 host）。用于各导入路径
// （.ptd / 自身 JSON / MoviePilot）统一挂「正确定义」，避免全退化为默认 nexusphp——
// tnode/特殊 schema 站走错适配器会彻底抓空，nexusphp 站用通用 def 也常抓不全。
// s.reg 为空（测试）或无匹配返回空串，调用方回退到 platform 推断。
func (s *Service) matchDef(baseURL string) string {
	if s.reg == nil {
		return ""
	}
	id, _ := s.reg.MatchByURL(baseURL)
	return id
}

// downloaderIdentity 下载器去重键：同一客户端类型 + 同一端点地址即视为同一个下载器。
// 名称是用户可随意改的展示字段，不参与判等。
func downloaderIdentity(typ, rawURL string) string {
	return strings.ToLower(strings.TrimSpace(typ)) + "|" +
		strings.TrimRight(strings.TrimSpace(rawURL), "/")
}

// firstOf 截断清单用于文案展示（完整条数已在错误里给出）。
func firstOf(xs []string, n int) []string {
	if len(xs) > n {
		return xs[:n]
	}
	return xs
}
