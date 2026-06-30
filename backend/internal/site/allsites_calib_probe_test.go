package site

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// allsites_calib_probe_test.go：全站明细扫描（D:\html 存在时跑，否则 skip）。
// 引擎修复（逐键合并/正则失配返空/默认 bonus+seeding）惠及全部 NexusPHP 站，
// 此探针逐站验证明细是否正常显示，标记疑似坏站（统计全空）供逐站校准。
// 与 five_sites_calib_probe_test.go 同包，复用 loadCalibDef/loadCalibDoc。

type sweepSite struct {
	def     string
	usercp  string
	detail  string
	skipDet bool
}

func sweepSites() []sweepSite {
	p := " - Powered by NexusPHP"
	return []sweepSite{
		{def: "agsvpt", usercp: "Arctic Global Seed Vault __ 控制面板 - 首页" + p + ".html", detail: "", skipDet: true},
		{def: "hdupt", usercp: "HDU __ 控制面板 - 首页(Z For A)-High Definition In Upxin_Privte Torrents.html", detail: "HDU __ 用户详情 - gntv(Z For A)-High Definition In Upxin_Privte Torrents.html"},
		{def: "longpt", usercp: "LongPT __ 控制面板 - 首页" + p + ".html", detail: "LongPT __ 用户详情 - gntv" + p + ".html"},
		{def: "march", usercp: "March __ 控制面板 - 首页" + p + ".html", detail: "March __ 用户详情 - gntv" + p + ".html"},
		{def: "ourbits", usercp: "OurBits __ 控制面板 - 首页" + p + ".html", detail: "OurBits __ 用户详情 - gntv" + p + ".html"},
		{def: "pandapt", usercp: "PANDA __ 控制面板 - 首页 熊猫高清.html", detail: "PANDA __ 用户详情 - gntv 熊猫高清.html"},
		{def: "hxpt", usercp: "好学 __ 控制面板 - 首页" + p + ".html", detail: "好学 __ 用户详情 - gntv" + p + ".html"},
		{def: "siqi", usercp: "思齐 __ 控制面板 - 首页 高清图书站" + p + ".html", detail: "思齐 __ 用户详情 - gntv 高清图书站" + p + ".html"},
		{def: "crabpt", usercp: "蟹黄堡 __ 控制面板 - 首页" + p + ".html", detail: "蟹黄堡 __ 用户详情 - gntv" + p + ".html"},
		{def: "cspt", usercp: "财神 __ 控制面板 - 首页" + p + ".html", detail: "财神 __ 用户详情 - gntv" + p + ".html"},
		{def: "qingwa", usercp: "青蛙 __ 控制面板 - 首页" + p + ".html", detail: "青蛙 __ 用户详情 - gntv" + p + ".html"},
	}
}

func TestSweepAllSites(t *testing.T) {
	if _, err := os.Stat(calibHTMLDir); err != nil {
		t.Skipf("calib dir not present: %v", err)
	}
	for _, s := range sweepSites() {
		t.Run(s.def, func(t *testing.T) {
			def := loadCalibDef(t, s.def)
			var sb strings.Builder
			fmt.Fprintf(&sb, "%-9s schema=%s", s.def, def.Schema)
			if doc, _ := loadCalibDoc(t, s.usercp); doc != nil {
				u := ParseUserInfo(doc, def)
				empty := u.Uploaded == 0 && u.Downloaded == 0 && u.Ratio == 0 && u.Bonus == 0
				mark := "✓"
				if empty {
					mark = "✗全空"
				}
				fmt.Fprintf(&sb, " | usercp[%s] up=%d dl=%d ratio=%.2f bonus=%.0f seed=%d", mark, u.Uploaded, u.Downloaded, u.Ratio, u.Bonus, u.Seeding)
			} else {
				fmt.Fprintf(&sb, " | usercp MISSING")
			}
			if !s.skipDet {
				if doc, _ := loadCalibDoc(t, s.detail); doc != nil {
					u := &UserInfo{}
					ParseUserDetail(doc, def, u)
					fmt.Fprintf(&sb, " | detail level=%q join=%q email=%q", u.Level, u.JoinDate, u.Email)
				}
			}
			t.Logf("%s", sb.String())
		})
	}
}
