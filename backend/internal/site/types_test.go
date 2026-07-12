package site

import "testing"

// types.go 的纯逻辑方法/函数穷尽表测：
//   - parseRatioFloat：分享率串解析（千分位逗号剥离、∞/inf、非法→-1）
//   - InviteStatus：邀请库存可购买数算术（price/bonus 守卫 + 向零截断）
//   - InviteeInfo：IsDead/IsPending/HasNoTrafficData/IsLowRatio 状态判定
// 已有 harem_test.go 只各覆盖 1 例，此处补全 i18n 关键词与边界。

func TestParseRatioFloat(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want float64
	}{
		{"empty", "", -1},
		{"whitespace only", "   ", -1},
		{"infinity symbol", "∞", 1e9},
		{"inf lowercase", "inf", 1e9},
		{"infinite mixed case", "InFiNiTe", 1e9},
		{"normal 0.5", "0.5", 0.5},
		{"normal 1.234", "1.234", 1.234},
		{"integer", "3", 3},
		{"zero", "0", 0},
		// 逗号按千分位剥离（非欧洲小数点）：1,000.5 → 1000.5；1,5 → 15
		{"thousands comma", "1,000.5", 1000.5},
		{"comma-not-decimal", "1,5", 15},
		{"leading/trailing whitespace", "  3.14  ", 3.14},
		{"non-numeric", "abc", -1},
		{"n/a slug", "N/A", -1},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := parseRatioFloat(c.in); got != c.want {
				t.Errorf("parseRatioFloat(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestInviteStatusBuyable(t *testing.T) {
	t.Parallel()
	t.Run("TotalRemain sums permanent+temporary", func(t *testing.T) {
		t.Parallel()
		if got := (&InviteStatus{PermanentCount: 3, TemporaryCount: 7}).TotalRemain(); got != 10 {
			t.Errorf("TotalRemain = %d, want 10", got)
		}
		if got := (&InviteStatus{}).TotalRemain(); got != 0 {
			t.Errorf("empty TotalRemain = %d, want 0", got)
		}
	})

	// BuyablePermanent：price>0 且 bonus>0 时 bonus/price 向零截断；否则 0。
	bpCases := []struct {
		name   string
		st     InviteStatus
		want   int
	}{
		{"exact division", InviteStatus{Bonus: 10000, PermanentInvitePrice: 2000}, 5},
		{"truncation toward zero", InviteStatus{Bonus: 9999, PermanentInvitePrice: 2000}, 4},
		{"MTeam temp 240000/80000", InviteStatus{Bonus: 240000, TemporaryInvitePrice: 80000}, 0}, // temp not permanent
		{"price le 0 (no shop)", InviteStatus{Bonus: 99999, PermanentInvitePrice: -1}, 0},
		{"price zero", InviteStatus{Bonus: 100, PermanentInvitePrice: 0}, 0},
		{"bonus le 0", InviteStatus{Bonus: 0, PermanentInvitePrice: 2000}, 0},
		{"bonus negative", InviteStatus{Bonus: -5, PermanentInvitePrice: 2000}, 0},
	}
	for _, c := range bpCases {
		c := c
		t.Run("BuyablePermanent/"+c.name, func(t *testing.T) {
			t.Parallel()
			if got := c.st.BuyablePermanent(); got != c.want {
				t.Errorf("BuyablePermanent = %d, want %d", got, c.want)
			}
		})
	}

	t.Run("BuyableTemporary MTeam", func(t *testing.T) {
		t.Parallel()
		st := InviteStatus{Bonus: 240000, TemporaryInvitePrice: 80000}
		if got := st.BuyableTemporary(); got != 3 {
			t.Errorf("BuyableTemporary = %d, want 3", got)
		}
		// NexusPHP 无临时商店（price=-1）→ 0
		if got := (&InviteStatus{Bonus: 500, TemporaryInvitePrice: -1}).BuyableTemporary(); got != 0 {
			t.Errorf("no-shop BuyableTemporary = %d, want 0", got)
		}
	})

	t.Run("TotalBuyable composes both", func(t *testing.T) {
		t.Parallel()
		st := InviteStatus{Bonus: 10000, PermanentInvitePrice: 2000, TemporaryInvitePrice: 5000} // 5 + 2
		if got := st.TotalBuyable(); got != 7 {
			t.Errorf("TotalBuyable = %d, want 7", got)
		}
	})
}

func TestInviteeInfoIsDead(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		inv  InviteeInfo
		want bool
	}{
		{"enabled normal", InviteeInfo{Enabled: true, Status: "正常"}, false},
		{"disabled flag overrides status", InviteeInfo{Enabled: false, Status: "正常"}, true},
		{"english disabled", InviteeInfo{Enabled: true, Status: "disabled"}, true},
		{"english banned", InviteeInfo{Enabled: true, Status: "banned"}, true},
		{"english suspend mixed case", InviteeInfo{Enabled: true, Status: "SUSPENDED"}, true},
		{"english forbidden", InviteeInfo{Enabled: true, Status: "forbidden"}, true},
		{"english no", InviteeInfo{Enabled: true, Status: "no"}, true},
		{"english disable short", InviteeInfo{Enabled: true, Status: "disable"}, true},
		{"chinese 禁用 substring", InviteeInfo{Enabled: true, Status: "已被禁用"}, true},
		{"chinese 封禁", InviteeInfo{Enabled: true, Status: "封禁"}, true},
		{"chinese 停用", InviteeInfo{Enabled: true, Status: "停用"}, true},
		{"chinese 禁止", InviteeInfo{Enabled: true, Status: "禁止"}, true},
		{"chinese 未启用", InviteeInfo{Enabled: true, Status: "未启用"}, true},
		{"empty status enabled", InviteeInfo{Enabled: true, Status: ""}, false},
		{"whitespace status", InviteeInfo{Enabled: true, Status: "   "}, false},
		{"unknown active status", InviteeInfo{Enabled: true, Status: "active"}, false},
		{"pending is not dead", InviteeInfo{Enabled: true, Status: "pending"}, false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := c.inv.IsDead(); got != c.want {
				t.Errorf("IsDead(status=%q enabled=%v) = %v, want %v", c.inv.Status, c.inv.Enabled, got, c.want)
			}
		})
	}
}

func TestInviteeInfoIsPending(t *testing.T) {
	t.Parallel()
	cases := []struct {
		status string
		want   bool
	}{
		{"pending", true},
		{" PENDING ", true},
		{"待确认", true},
		{"待確認", true},
		{"未确认", true},
		{"未確認", true},
		{"未激活", true},
		{"状态:待确认", true}, // substring
		{"confirmed", false},
		{"", false},
		{"active", false},
		{"disabled", false}, // disabled 不算 pending
	}
	for _, c := range cases {
		c := c
		t.Run(c.status, func(t *testing.T) {
			t.Parallel()
			inv := InviteeInfo{Status: c.status}
			if got := inv.IsPending(); got != c.want {
				t.Errorf("IsPending(%q) = %v, want %v", c.status, got, c.want)
			}
		})
	}
}

func TestInviteeInfoHasNoTrafficData(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		up   int64
		down int64
		want bool
	}{
		{"both zero", 0, 0, true},
		{"both negative", -1, -1, true},
		{"upload only", 100, 0, false},
		{"download only", 0, 100, false},
		{"one neg one zero", -1, 0, true},
		{"both positive", 1, 1, false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			inv := InviteeInfo{Uploaded: c.up, Downloaded: c.down}
			if got := inv.HasNoTrafficData(); got != c.want {
				t.Errorf("HasNoTrafficData(up=%d,down=%d) = %v, want %v", c.up, c.down, got, c.want)
			}
		})
	}
}

func TestInviteeInfoIsLowRatio(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		inv  InviteeInfo
		want bool
	}{
		// downloaded<=0 一律不判（纯做种者）
		{"pure seeder skipped", InviteeInfo{Downloaded: 0, Uploaded: 100, Ratio: "0"}, false},
		{"negative downloaded skipped", InviteeInfo{Downloaded: -5, Ratio: "0.1"}, false},
		// Ratio 串可解析
		{"ratio 0.5 low", InviteeInfo{Downloaded: 1000, Ratio: "0.5"}, true},
		{"ratio 0 boundary low", InviteeInfo{Downloaded: 1000, Ratio: "0"}, true},
		{"ratio 1.0 not low", InviteeInfo{Downloaded: 1000, Ratio: "1.0"}, false},
		{"ratio 2.0 not low", InviteeInfo{Downloaded: 1000, Ratio: "2.0"}, false},
		{"ratio infinity not low", InviteeInfo{Downloaded: 1000, Ratio: "∞"}, false},
		{"comma thousands ratio 1,000", InviteeInfo{Downloaded: 1, Ratio: "1,000"}, false}, // 1000.0
		// Ratio 解析失败（-1）→ 回退 Uploaded/Downloaded
		{"empty ratio fallback low", InviteeInfo{Downloaded: 100, Uploaded: 50, Ratio: ""}, true},  // 0.5
		{"n/a ratio fallback ok", InviteeInfo{Downloaded: 100, Uploaded: 200, Ratio: "N/A"}, false}, // 2.0
		{"empty ratio zero-up not low", InviteeInfo{Downloaded: 100, Uploaded: 0, Ratio: ""}, true},  // 0/100=0 → low
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := c.inv.IsLowRatio(); got != c.want {
				t.Errorf("IsLowRatio() = %v, want %v", got, c.want)
			}
		})
	}
}
