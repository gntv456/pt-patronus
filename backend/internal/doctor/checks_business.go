// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package doctor

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ---- 业务体检（对标 MSaber 系统体检：站点 / 下载器 / 媒体服务器 / 云盘 / 网络 / 队列六类） ----
// 检查函数依赖经闭包注入（app 装配时传入各 chain），doctor 包保持叶子、不 import 业务包。

// ProbeFunc 单个目标的探活函数：返回 nil 表示健康。
type ProbeFunc func(ctx context.Context) error

// Target 一个待探测目标（站点/下载器/媒体服务器/云盘）。
type Target struct {
	ID   uint
	Name string
	Type string // 标识用途，如 site/downloader/mediaserver/clouddrive
	Prob ProbeFunc
}

// CheckTargets 并发探测一组同类目标，聚合为一条 Finding（每目标独立 8s 超时）。
// 无目标 → skipped（不算分）；全部通过 → ok；有失败 → degraded，Detail 列出失败项。
func CheckTargets(id, title string, warnOnly bool, targets []Target) Check {
	return func(ctx context.Context) Finding {
		f := Finding{ID: id, Title: title, Severity: SevWarn}
		if len(targets) == 0 {
			f.Status, f.Severity, f.Detail = StatusSkipped, SevInfo, "未配置"
			return f
		}
		type res struct {
			name string
			err  error
		}
		results := make([]res, len(targets))
		done := make(chan struct{})
		go func() {
			defer close(done)
			sem := make(chan struct{}, 5)
			fin := make(chan int, len(targets))
			for i := range targets {
				sem <- struct{}{}
				go func(i int) {
					defer func() { <-sem; fin <- i }()
					defer func() {
						if rec := recover(); rec != nil {
							results[i] = res{targets[i].Name, fmt.Errorf("panic: %v", rec)}
						}
					}()
					pctx, cancel := context.WithTimeout(ctx, 8*time.Second)
					defer cancel()
					results[i] = res{targets[i].Name, targets[i].Prob(pctx)}
				}(i)
			}
			for range targets {
				<-fin
			}
		}()
		select {
		case <-done:
		case <-ctx.Done():
			f.Status, f.Detail = StatusDegraded, "检查超时/取消"
			return f
		}
		var failed []string
		total, ok := len(targets), 0
		for _, r := range results {
			if r.err != nil {
				failed = append(failed, fmt.Sprintf("%s: %v", r.name, r.err))
			} else {
				ok++
			}
		}
		f.Context = map[string]any{"total": total, "ok": ok}
		if len(failed) == 0 {
			f.Status, f.Severity, f.Detail = StatusOK, SevInfo, fmt.Sprintf("全部 %d 项连接正常", total)
			return f
		}
		// 失败信息截断，避免 Detail 爆炸（每条限 80 字符、总 5 条）。
		for i := range failed {
			if len(failed[i]) > 80 {
				failed[i] = failed[i][:80] + "…"
			}
		}
		msg := fmt.Sprintf("%d/%d 项失败", len(failed), total)
		if len(failed) > 5 {
			f.Detail = msg + "：" + strings.Join(failed[:5], "；") + fmt.Sprintf("（等 %d 项）", len(failed)-5)
		} else {
			f.Detail = msg + "：" + strings.Join(failed, "；")
		}
		f.Severity = SevWarn
		if !warnOnly {
			f.Severity = SevError
		}
		f.Status = StatusDegraded
		f.Recommendation = "检查对应配置的地址 / 凭据是否有效"
		return f
	}
}

// CheckNetwork 出网连通性（TMDB / 公共 DNS 解析，best-effort）。
//
// tmdbProbe 返回当前生效的 TMDB 探活地址（应与搜索/识别链路同一份配置）。
// 从前这里写死官方 api.themoviedb.org：按 TMDB_API_DOMAIN 换成国内可达镜像的部署，
// 搜索明明正常，体检却每次都判「出网受限」并把用户支去查代理/DNS（假红）。
func CheckNetwork(tmdbProbe func() string) Check {
	return func(ctx context.Context) Finding {
		f := Finding{ID: "network", Severity: SevWarn, Title: "网络连通性"}
		client := &http.Client{Timeout: 8 * time.Second}
		var failed []string
		for _, u := range networkProbeURLs(tmdbProbe) {
			pctx, cancel := context.WithTimeout(ctx, 8*time.Second)
			req, _ := http.NewRequestWithContext(pctx, http.MethodGet, u, nil)
			resp, err := client.Do(req)
			cancel()
			if err != nil {
				failed = append(failed, fmt.Sprintf("%s: %v", u, err))
				continue
			}
			resp.Body.Close()
		}
		if len(failed) > 0 {
			f.Status = StatusDegraded
			f.Detail = "出网受限：" + strings.Join(failed, "；")
			f.Recommendation = "检查代理设置 / DNS / 防火墙（影响识别与搜索）"
			if len(failed) == 2 {
				f.Severity = SevError
			}
			return f
		}
		f.Status, f.Severity, f.Detail = StatusOK, SevInfo, "出网正常"
		return f
	}
}

// networkProbeURLs 体检探活目标：TMDB（跟搜索/识别同一份运行时配置）+ 通用出网。
// tmdbProbe 为空或其返回空串时回退官方域名。
func networkProbeURLs(tmdbProbe func() string) []string {
	tmdbURL := ""
	if tmdbProbe != nil {
		tmdbURL = strings.TrimSpace(tmdbProbe())
	}
	if tmdbURL == "" {
		tmdbURL = "https://api.themoviedb.org/3/configuration"
	}
	return []string{tmdbURL, "https://www.baidu.com"}
}

// CheckQueue 任务队列积压。pending 与 oldest 由调用方提供（当前只接 CloudTask 队列）。
func CheckQueue(pending func() (count int64, oldestAge time.Duration, err error)) Check {
	return func(ctx context.Context) Finding {
		f := Finding{ID: "queue", Severity: SevWarn, Title: "任务队列"}
		if pending == nil {
			f.Status, f.Severity, f.Detail = StatusSkipped, SevInfo, "未注入队列读取"
			return f
		}
		count, age, err := pending()
		if err != nil {
			f.Status, f.Detail, f.Recommendation = StatusDegraded, "读取队列失败: "+err.Error(), "检查数据库状态"
			return f
		}
		f.Context = map[string]any{"pending": count, "oldest_minutes": int(age.Minutes())}
		switch {
		case count == 0:
			f.Status, f.Severity, f.Detail = StatusOK, SevInfo, "队列空闲"
		case age > 6*time.Hour:
			f.Status, f.Detail, f.Recommendation = StatusDegraded, fmt.Sprintf("积压 %d 项，最久等待 %s（疑似执行器停滞）", count, age.Truncate(time.Minute)), "查看云盘任务页与日志，必要时重试/重启"
		case age > time.Hour:
			f.Status, f.Detail, f.Recommendation = StatusDegraded, fmt.Sprintf("积压 %d 项，最久等待 %s", count, age.Truncate(time.Minute)), "关注上传速率与网盘限流"
		default:
			f.Status, f.Severity, f.Detail = StatusOK, SevInfo, fmt.Sprintf("待处理 %d 项（正常消费中）", count)
		}
		return f
	}
}
