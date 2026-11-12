// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package app

import (
	"context"
	"time"

	"github.com/hxpt/ptpatronus/internal/doctor"
)

// buildDoctorBusinessChecks 组装业务体检六类检查：
// 站点 / 下载器 / 媒体服务器 / 云盘 / 网络 / 云任务队列。探活复用各 chain 既有 Test 入口，
// doctor 包保持叶子（Target/ProbeFunc 闭包注入，不 import 业务包）。
func buildDoctorBusinessChecks(d runtimeDeps) []doctor.Check {
	siteTargets := func() ([]doctor.Target, error) {
		sites, err := d.siteOp.ListActive()
		if err != nil {
			return nil, err
		}
		ts := make([]doctor.Target, 0, len(sites))
		for i := range sites {
			s := sites[i]
			ts = append(ts, doctor.Target{ID: s.ID, Name: s.Name, Type: "site",
				Prob: func(ctx context.Context) error {
					_, err := d.siteCh.TestConnection(ctx, &s)
					return err
				}})
		}
		return ts, nil
	}
	dlTargets := func() ([]doctor.Target, error) {
		dls, err := d.dlOp.List()
		if err != nil {
			return nil, err
		}
		ts := make([]doctor.Target, 0, len(dls))
		for i := range dls {
			id := dls[i].ID
			name := dls[i].Name
			ts = append(ts, doctor.Target{ID: id, Name: name, Type: "downloader",
				Prob: func(ctx context.Context) error { return d.dlCh.Test(ctx, id) }})
		}
		return ts, nil
	}
	msTargets := func() ([]doctor.Target, error) {
		servers, err := d.mediaServerCh.List()
		if err != nil {
			return nil, err
		}
		ts := make([]doctor.Target, 0, len(servers))
		for i := range servers {
			id := servers[i].ID
			name := servers[i].Name
			ts = append(ts, doctor.Target{ID: id, Name: name, Type: "mediaserver",
				Prob: func(ctx context.Context) error {
					_, err := d.mediaServerCh.Test(ctx, id)
					return err
				}})
		}
		return ts, nil
	}
	cloudTargets := func() ([]doctor.Target, error) {
		drives, err := d.cloudCh.List()
		if err != nil {
			return nil, err
		}
		ts := make([]doctor.Target, 0, len(drives))
		for i := range drives {
			id := drives[i].ID
			name := drives[i].Name
			ts = append(ts, doctor.Target{ID: id, Name: name, Type: "clouddrive",
				Prob: func(ctx context.Context) error {
					_, err := d.cloudCh.Test(ctx, id)
					return err
				}})
		}
		return ts, nil
	}
	return []doctor.Check{
		lazyTargets("business.sites", "站点连通", false, siteTargets),
		lazyTargets("business.downloaders", "下载器连通", false, dlTargets),
		lazyTargets("business.mediaservers", "媒体服务器连通", false, msTargets),
		lazyTargets("business.clouddrives", "云盘连通", true, cloudTargets),
		doctor.CheckNetwork(tmdbProbeURL(d)),
		doctor.CheckQueue(func() (int64, time.Duration, error) {
			if d.cloudTaskOp == nil {
				return 0, 0, nil
			}
			return d.cloudTaskOp.PendingStats()
		}),
	}
}

// tmdbProbeURL 体检探活用当前生效的 TMDB 地址：与搜索/识别链路同一份设置
// （SystemConfig 热更新优先、env 兜底），不再写死官方域名。
func tmdbProbeURL(d runtimeDeps) func() string {
	return func() string {
		domain := ""
		if d.cfg != nil {
			domain = d.cfg.TMDBAPIDomain
		}
		if d.mediaCh != nil {
			if p := d.mediaCh.Provider(); p != nil {
				if s := p.LoadSettings(); s.TMDBAPIDomain != "" {
					domain = s.TMDBAPIDomain
				}
			}
		}
		if domain == "" {
			domain = "api.themoviedb.org"
		}
		return "https://" + domain + "/3/configuration"
	}
}

// lazyTargets 每次体检执行时才解析目标列表（目标可能中途增删）。
// 目标列表读失败必须报 failed——以前吞成 0 目标，体检渲染成「未配置」(skipped/info)，
// 与「真的没配」同形，体检面板把读库故障画成没事。
func lazyTargets(id, title string, warnOnly bool, targets func() ([]doctor.Target, error)) doctor.Check {
	return func(ctx context.Context) doctor.Finding {
		ts, err := targets()
		if err != nil {
			return doctor.Finding{
				ID: id, Title: title, Severity: doctor.SevError, Status: doctor.StatusFailed,
				Detail:         "读取目标列表失败: " + err.Error(),
				Recommendation: "检查数据库状态后重新体检",
			}
		}
		check := doctor.CheckTargets(id, title, warnOnly, ts)
		return check(ctx)
	}
}
