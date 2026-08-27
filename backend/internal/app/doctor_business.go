package app

import (
	"context"
	"time"

	"github.com/hxpt/ptpatronus/internal/doctor"
)

// buildDoctorBusinessChecks 组装业务体检六类检查（对标 MSaber 系统体检）：
// 站点 / 下载器 / 媒体服务器 / 云盘 / 网络 / 云任务队列。探活复用各 chain 既有 Test 入口，
// doctor 包保持叶子（Target/ProbeFunc 闭包注入，不 import 业务包）。
func buildDoctorBusinessChecks(d runtimeDeps) []doctor.Check {
	siteTargets := func() []doctor.Target {
		sites, err := d.siteOp.ListActive()
		if err != nil {
			return nil
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
		return ts
	}
	dlTargets := func() []doctor.Target {
		dls, err := d.dlOp.List()
		if err != nil {
			return nil
		}
		ts := make([]doctor.Target, 0, len(dls))
		for i := range dls {
			id := dls[i].ID
			name := dls[i].Name
			ts = append(ts, doctor.Target{ID: id, Name: name, Type: "downloader",
				Prob: func(ctx context.Context) error { return d.dlCh.Test(ctx, id) }})
		}
		return ts
	}
	msTargets := func() []doctor.Target {
		servers, err := d.mediaServerCh.List()
		if err != nil {
			return nil
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
		return ts
	}
	cloudTargets := func() []doctor.Target {
		drives, err := d.cloudCh.List()
		if err != nil {
			return nil
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
		return ts
	}
	return []doctor.Check{
		lazyTargets("business.sites", "站点连通", false, siteTargets),
		lazyTargets("business.downloaders", "下载器连通", false, dlTargets),
		lazyTargets("business.mediaservers", "媒体服务器连通", false, msTargets),
		lazyTargets("business.clouddrives", "云盘连通", true, cloudTargets),
		doctor.CheckNetwork(),
		doctor.CheckQueue(func() (int64, time.Duration, error) {
			if d.cloudTaskOp == nil {
				return 0, 0, nil
			}
			return d.cloudTaskOp.PendingStats()
		}),
	}
}

// lazyTargets 每次体检执行时才解析目标列表（目标可能中途增删）。
func lazyTargets(id, title string, warnOnly bool, targets func() []doctor.Target) doctor.Check {
	return func(ctx context.Context) doctor.Finding {
		check := doctor.CheckTargets(id, title, warnOnly, targets())
		return check(ctx)
	}
}
