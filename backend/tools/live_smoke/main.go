// Command live_smoke 用真实 cookie 对站点跑端到端 Search + UserInfo，验证适配器+选择器对真实页面的解析。
//
// 两种模式：
//  1. 单站：SITE_COOKIE + SITE_ID(默认 hxpt) + SITE_BASE(默认按 id 取定义首个 url) + SITE_KW(默认 test)
//  2. 批量：SITES_FILE 指向一个 JSON 数组文件（本地放 cookie，不入库），一次跑多站并汇总。
//
// SITES_FILE 格式（每项 id 必填，base 默认按 id 取定义首个 url，kw 默认 test）：
//
//	[
//	  {"id":"hxpt","cookie":"...","kw":"test"},
//	  {"id":"ourbits","cookie":"..."},
//	  {"id":"mteam","base":"https://api.m-team.cc","cookie":"<apiKey>"}
//	]
//
// cookie 仅经环境变量/本地文件传入，不落命令行历史/日志/记忆。
// 运行：cd backend && SITE_COOKIE='...' go run ./tools/live_smoke
//
//	cd backend && SITES_FILE=./my-sites.json go run ./tools/live_smoke
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/site/schemas"
)

// siteSpec 是单站的校准输入。
type siteSpec struct {
	ID     string `json:"id"`
	Base   string `json:"base,omitempty"`
	Cookie string `json:"cookie,omitempty"`
	KW     string `json:"kw,omitempty"`
}

// siteResult 是单站校准结果（错误记入字段，不中断批量）。
type siteResult struct {
	Spec         siteSpec
	Schema       string
	SearchErr    string
	Torrents     int
	Sample       string
	UserInfoErr  string
	Name         string
	Level        string
	Uploaded     int64
	Downloaded   int64
	Ratio        float64
	Seeding      int
	Leeching     int
	SeedingSize  int64
	Uploads      int
	Bonus        float64
	BonusPerHour float64
	JoinDate     string
}

func main() {
	ctx := context.Background()

	if sitesFile := os.Getenv("SITES_FILE"); sitesFile != "" {
		runFile(ctx, sitesFile)
		return
	}
	runSingle(ctx)
}

// runSingle 单站模式（环境变量）。
// SITE_BASE 留空时按 SITE_ID 取定义首个 url（早年默认 hxpt 域名会让换站调试
// 直接打到错误站点，报 not logged in 造成误判）。
func runSingle(ctx context.Context) {
	sp := siteSpec{
		ID:     envOr("SITE_ID", "hxpt"),
		Base:   os.Getenv("SITE_BASE"),
		Cookie: os.Getenv("SITE_COOKIE"),
		KW:     envOr("SITE_KW", "test"),
	}
	if sp.Cookie == "" {
		fmt.Println("SITE_COOKIE env required (or set SITES_FILE for batch)")
		os.Exit(1)
	}
	hc, reg, err := newRuntime()
	if err != nil {
		fmt.Println("init:", err)
		os.Exit(1)
	}
	printSummary([]siteResult{runOne(ctx, hc, reg, sp)})
}

// runFile 批量模式（JSON 数组文件）。
func runFile(ctx context.Context, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("read SITES_FILE:", err)
		os.Exit(1)
	}
	var specs []siteSpec
	if err := json.Unmarshal(data, &specs); err != nil {
		fmt.Println("parse SITES_FILE (expect JSON array of {id,base,cookie,kw}):", err)
		os.Exit(1)
	}
	if len(specs) == 0 {
		fmt.Println("SITES_FILE has no sites")
		os.Exit(1)
	}
	hc, reg, err := newRuntime()
	if err != nil {
		fmt.Println("init:", err)
		os.Exit(1)
	}
	run := func(sp siteSpec) siteResult { return runOne(ctx, hc, reg, sp) }
	printSummary(runBatch(specs, run))
}

// newRuntime 构造共享 httpclient + 加载 definitions。
func newRuntime() (*httpclient.Client, *site.Registry, error) {
	hc, err := httpclient.New(httpclient.Options{Timeout: 30 * time.Second})
	if err != nil {
		return nil, nil, err
	}
	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("definitions"); err != nil {
		return nil, nil, err
	}
	return hc, reg, nil
}

// runOne 对单个站点跑 Search + UserInfo，返回结构化结果（错误记字段不中断）。
func runOne(ctx context.Context, hc *httpclient.Client, reg *site.Registry, sp siteSpec) siteResult {
	res := siteResult{Spec: sp}
	def, ok := reg.Definition(sp.ID)
	if !ok {
		res.SearchErr = "definition not found"
		return res
	}
	defd := site.EnsureDefaults(def)
	res.Schema = defd.Schema
	base := sp.Base
	if base == "" && len(defd.URLs) > 0 {
		base = defd.URLs[0]
	}
	adapter := pickAdapter(hc, defd.Schema)
	if adapter == nil {
		res.SearchErr = "no adapter for schema " + defd.Schema
		return res
	}
	kw := sp.KW
	if kw == "" {
		kw = "test"
	}
	ts, err := adapter.Search(ctx, defd, base, sp.Cookie, kw, "", 1)
	if err != nil {
		res.SearchErr = err.Error()
	} else {
		res.Torrents = len(ts)
		if len(ts) > 0 {
			res.Sample = ts[0].Title
		}
	}
	ui, err := adapter.FetchUserInfo(ctx, defd, base, sp.Cookie)
	if err != nil {
		res.UserInfoErr = err.Error()
	} else if ui != nil {
		res.Name, res.Level = ui.Name, ui.Level
		res.Uploaded, res.Downloaded, res.Ratio = ui.Uploaded, ui.Downloaded, ui.Ratio
		res.Seeding, res.Leeching, res.SeedingSize, res.Uploads = ui.Seeding, ui.Leeching, ui.SeedingSize, ui.Uploads
		res.Bonus, res.BonusPerHour, res.JoinDate = ui.Bonus, ui.BonusPerHour, ui.JoinDate
	}
	return res
}

// runFunc 是单站执行器（可注入，便于测试批量编排而不触网）。
type runFunc func(siteSpec) siteResult

// runBatch 顺序跑所有 spec（避免并发对站点造成突发请求），返回结果切片。
func runBatch(specs []siteSpec, run runFunc) []siteResult {
	results := make([]siteResult, len(specs))
	for i, sp := range specs {
		results[i] = run(sp)
	}
	return results
}

// summaryString 汇总结果为对齐文本表。
func summaryString(results []siteResult) string {
	var b strings.Builder
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSCHEMA\tSEARCH\tTORRENTS\tUSERINFO\tDETAILS")
	for _, r := range results {
		search := fmt.Sprintf("%d", r.Torrents)
		if r.SearchErr != "" {
			search = "ERR: " + truncate(r.SearchErr, 30)
		}
		userinfo := "ok"
		if r.UserInfoErr != "" {
			userinfo = "ERR: " + truncate(r.UserInfoErr, 20)
		}
		details := ""
		if r.UserInfoErr == "" {
			details = fmt.Sprintf("name=%s lvl=%s up=%s down=%s ratio=%.3f seed=%d leech=%d seedSize=%s uploads=%d bonus=%.0f bph=%.2f join=%s",
				truncate(r.Name, 12), truncate(r.Level, 12), humanBytes(r.Uploaded), humanBytes(r.Downloaded),
				r.Ratio, r.Seeding, r.Leeching, humanBytes(r.SeedingSize), r.Uploads, r.Bonus, r.BonusPerHour, r.JoinDate)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\n", r.Spec.ID, r.Schema, search, r.Torrents, userinfo, details)
		if r.Sample != "" {
			fmt.Fprintf(tw, "\t\t\tsample:\t%s\n", truncate(r.Sample, 50))
		}
	}
	tw.Flush()
	return b.String()
}

func printSummary(results []siteResult) {
	fmt.Print(summaryString(results))
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// truncate 按 rune 截断到约 n 宽（rune 安全，不劈开中文；末尾加 …）。
func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}

// humanBytes 字节 → "1.2TB"（紧凑，汇总列用）。
func humanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// pickAdapter 按 schema 返回对应适配器（覆盖已实现的 + generic 兜底）。
func pickAdapter(hc *httpclient.Client, schema string) site.Adapter {
	switch schema {
	case "", "nexusphp":
		return schemas.NewNexusPHP(hc)
	case "mteam":
		return schemas.NewMTeam(hc)
	case "unit3d":
		return schemas.NewUnit3D(hc)
	case "gazelle":
		return schemas.NewGazelle(hc)
	case "luminance":
		return schemas.NewLuminance(hc)
	case "gazelle_jsonapi":
		return schemas.NewGazelleJSONAPI(hc)
	case "avistaz":
		return schemas.NewAvistaz(hc)
	case "rousi":
		return schemas.NewRousi(hc)
	default:
		return schemas.NewGenericHTML(hc, schema)
	}
}
