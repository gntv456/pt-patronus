package v1

import (
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/site"
)

// api/v1 handler 文件内未导出纯 helper 的表测。这些函数无 *gin.Context/DB/chain 依赖，
// 是 handler 层最低成本的覆盖面：限速解析防误解除、命令风险分级(安全关键)、URL 归一化、
// 秘钥脱敏、分页/ID 解析、消息邮箱枚举等。handler 早返回路径单列 handlers_*_test.go。

func TestClampDays(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		q            string
		def, min, mx int
		want         int
	}{
		{"empty uses default", "", 30, 1, 365, 30},
		{"non-numeric uses default", "abc", 30, 1, 365, 30},
		{"zero uses default", "0", 30, 1, 365, 30},
		{"below min clamps", "-5", 30, 1, 365, 1},
		{"above max clamps", "999999", 30, 1, 365, 365},
		{"valid passthrough", "90", 30, 1, 365, 90},
		{"exact min", "1", 30, 1, 365, 1},
		{"exact max", "365", 30, 1, 365, 365},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := clampDays(c.q, c.def, c.min, c.mx); got != c.want {
				t.Errorf("clampDays(%q,%d,%d,%d) = %d, want %d", c.q, c.def, c.min, c.mx, got, c.want)
			}
		})
	}
}

func TestClassifyCommandRisk(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		cmd        string
		risk       string
		wantReason bool // 仅校验是否给了 reason（具体文案易变，断言非空即可）
	}{
		{"rm -rf root", "rm -rf /", "high", true},
		{"mkfs", "mkfs.ext4 /dev/sda", "high", true},
		{"dd to device", "dd if=/x of=/dev/sda", "high", true},
		{"shutdown", "shutdown -h now", "high", true},
		{"chmod -R 777 root", "chmod -R 777 /", "high", true},
		{"chown -R root", "chown -R root /", "high", true},
		// 非根目录的递归删除 → 中危（high 要求根 / 收尾）
		{"rm -rf home is medium", "rm -rf /home", "medium", true},
		{"systemctl stop", "systemctl stop nginx", "medium", true},
		{"docker rm", "docker rm web", "medium", true},
		{"drop database", "DROP DATABASE prod", "medium", true},
		{"truncate table", "truncate table users", "medium", true},
		// 高危优先于中危
		{"low ls", "ls -la", "low", false},
		{"low echo", "echo hello", "low", false},
		{"empty command", "", "low", false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			risk, reason := classifyCommandRisk(c.cmd)
			if risk != c.risk {
				t.Errorf("classifyCommandRisk(%q) risk = %q, want %q", c.cmd, risk, c.risk)
			}
			if c.wantReason && reason == "" {
				t.Errorf("expected non-empty reason for %q", c.cmd)
			}
			if !c.wantReason && reason != "" {
				t.Errorf("expected empty reason for %q, got %q", c.cmd, reason)
			}
		})
	}
}

func TestParseLimit(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"", 0, false}, // 空串=不限速
		{"1048576", 1048576, false},
		{"0", 0, false},
		{"abc", 0, true}, // 非法不可静默当 0
		{"-1", 0, true},  // 负数禁止（防误解除限速）
		{"1.5", 0, true},
	}
	for _, c := range cases {
		c := c
		t.Run(c.in, func(t *testing.T) {
			t.Parallel()
			got, err := parseLimit(c.in)
			if (err != nil) != c.wantErr {
				t.Errorf("parseLimit(%q) err = %v, wantErr %v", c.in, err, c.wantErr)
			}
			if got != c.want {
				t.Errorf("parseLimit(%q) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}

func TestSplitCSV(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{"  a  , b ,, c ", []string{"a", "b", "c"}}, // trim + 空串丢弃
		{",,,", nil},
	}
	for _, c := range cases {
		c := c
		t.Run(c.in, func(t *testing.T) {
			t.Parallel()
			got := splitCSV(c.in)
			if len(got) != len(c.want) {
				t.Errorf("splitCSV(%q) = %v, want %v", c.in, got, c.want)
				return
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("splitCSV(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestMaskSecret(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"short", "****"},             // <=8 全掩
		{"12345678", "****"},          // 恰好 8 全掩
		{"123456789", "1234****6789"}, // 9 首尾各 4
		{"sk-abcdefghijkl", "sk-a****ijkl"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.in, func(t *testing.T) {
			t.Parallel()
			if got := maskSecret(c.in); got != c.want {
				t.Errorf("maskSecret(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestMediaImageRefererMapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		host string
		ref  string
		ok   bool
	}{
		{"img.doubanio.com", "https://movie.douban.com/", true}, // suffix
		{"doubanio.com", "https://movie.douban.com/", true},     // exact
		{"DOUBANIO.COM", "https://movie.douban.com/", true},     // lower
		{"image.tmdb.org", "", true},                            // 白名单无需 referer
		{"evil.com", "", false},                                 // 非白名单
		{"img9.doubanio.com.cn", "", false},                     // .cn 后缀不匹配 doubanio.com
	}
	for _, c := range cases {
		c := c
		t.Run(c.host, func(t *testing.T) {
			t.Parallel()
			ref, ok := mediaImageReferer(c.host)
			if ok != c.ok || ref != c.ref {
				t.Errorf("mediaImageReferer(%q) = (%q,%v), want (%q,%v)", c.host, ref, ok, c.ref, c.ok)
			}
		})
	}
}

func TestUniqueTargetIDs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   []uint
		want []uint
	}{
		{"empty", nil, []uint{}},
		{"dedup", []uint{1, 2, 1, 3, 2}, []uint{1, 2, 3}},
		{"filter zero", []uint{0, 1, 0, 2}, []uint{1, 2}},
		{"preserve order", []uint{3, 1, 2, 1}, []uint{3, 1, 2}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := uniqueTargetIDs(c.in)
			if len(got) != len(c.want) {
				t.Errorf("uniqueTargetIDs(%v) = %v, want %v", c.in, got, c.want)
				return
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("uniqueTargetIDs(%v)[%d] = %d, want %d", c.in, i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestCommandRunSource(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		{"deploy", "deploy"},
		{"manual", "manual"},
		{"terminal", "terminal"},
		{"", "batch"},
		{"  deploy  ", "deploy"}, // trim
		{"unknown", "batch"},     // 默认
		{"BATCH", "batch"},       // 大小写不匹配→默认（仅精确匹配白名单）
	}
	for _, c := range cases {
		if got := commandRunSource(c.in); got != c.want {
			t.Errorf("commandRunSource(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCommandRunStatus(t *testing.T) {
	t.Parallel()
	cases := []struct {
		success, failure int
		want             string
	}{
		{10, 0, "success"},
		{0, 5, "failed"},
		{3, 2, "partial"},
		{0, 0, "success"}, // 无失败即成功
	}
	for _, c := range cases {
		if got := commandRunStatus(c.success, c.failure); got != c.want {
			t.Errorf("commandRunStatus(%d,%d) = %q, want %q", c.success, c.failure, got, c.want)
		}
	}
}

func TestAtoiDefault(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		def  int
		want int
	}{
		{"", 42, 42},
		{"abc", 42, 42},
		{"7", 42, 7},
		{"-3", 42, -3},
	}
	for _, c := range cases {
		if got := atoiDefault(c.in, c.def); got != c.want {
			t.Errorf("atoiDefault(%q,%d) = %d, want %d", c.in, c.def, got, c.want)
		}
	}
}

func TestParseUintSlice(t *testing.T) {
	t.Parallel()
	t.Run("empty returns nil", func(t *testing.T) {
		t.Parallel()
		if got := parseUintSlice(""); got != nil {
			t.Errorf("parseUintSlice('') = %v, want nil", got)
		}
	})
	cases := []struct {
		in   string
		want []uint
	}{
		{"1,2,3", []uint{1, 2, 3}},
		{" 1 , 2 ,, 3 ", []uint{1, 2, 3}}, // trim + 空丢弃
		{"1,abc,3", []uint{1, 3}},         // 非法项跳过
		{"0,5", []uint{0, 5}},             // 0 保留（不滤零）
	}
	for _, c := range cases {
		c := c
		t.Run(c.in, func(t *testing.T) {
			t.Parallel()
			got := parseUintSlice(c.in)
			if len(got) != len(c.want) {
				t.Errorf("parseUintSlice(%q) = %v, want %v", c.in, got, c.want)
				return
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("parseUintSlice(%q)[%d] = %d, want %d", c.in, i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestNormalizedURLHost(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"example.com", "example.com"},
		{"https://www.example.com/path", "example.com"},
		{"HTTPS://Foo.COM", "foo.com"},
		{"http://sub.example.com/x?q=1", "sub.example.com"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.in, func(t *testing.T) {
			t.Parallel()
			if got := normalizedURLHost(c.in); got != c.want {
				t.Errorf("normalizedURLHost(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestNormalizedURLKey(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"https://www.example.com/", "https://example.com/"},
		{"https://example.com/a/b/", "https://example.com/a/b"},
		{"https://example.com/a/b", "https://example.com/a/b"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.in, func(t *testing.T) {
			t.Parallel()
			if got := normalizedURLKey(c.in); got != c.want {
				t.Errorf("normalizedURLKey(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestDefinitionScore(t *testing.T) {
	t.Parallel()
	t.Run("nil is zero", func(t *testing.T) {
		t.Parallel()
		if got := definitionScore(nil); got != 0 {
			t.Errorf("nil score = %d, want 0", got)
		}
	})
	t.Run("dead site loses bonus", func(t *testing.T) {
		t.Parallel()
		dead := definitionScore(&site.SiteMetadata{Dead: true})
		live := definitionScore(&site.SiteMetadata{}) // Dead=false → +100000
		if dead != 0 {
			t.Errorf("dead score = %d, want 0", dead)
		}
		if live != 100000 {
			t.Errorf("live empty score = %d, want 100000", live)
		}
	})
	t.Run("aka and tags add to score", func(t *testing.T) {
		t.Parallel()
		got := definitionScore(&site.SiteMetadata{AKA: []string{"a", "b", "c"}, Tags: []string{"x"}})
		// 100000(live) + 3*2(aka) + 1(tags) = 100007
		if got != 100007 {
			t.Errorf("aka/tags score = %d, want 100007", got)
		}
	})
}

func TestMergeStrings(t *testing.T) {
	t.Parallel()
	t.Run("dedup case-insensitive preserves first casing", func(t *testing.T) {
		t.Parallel()
		got := mergeStrings([]string{"Apple"}, "apple", "APPLE")
		if len(got) != 1 || got[0] != "Apple" {
			t.Errorf("mergeStrings = %v, want [Apple]", got)
		}
	})
	t.Run("trim + drop empty", func(t *testing.T) {
		t.Parallel()
		got := mergeStrings(nil, "  a  ", "", "b")
		if len(got) != 2 || got[0] != "a" || got[1] != "b" {
			t.Errorf("mergeStrings = %v, want [a b]", got)
		}
	})
	t.Run("append new values", func(t *testing.T) {
		t.Parallel()
		got := mergeStrings([]string{"a"}, "b", "c")
		if len(got) != 3 {
			t.Errorf("mergeStrings len = %d, want 3", len(got))
		}
	})
}

func TestParseMailbox(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want site.MailboxType
	}{
		{"inbox", site.MailboxInbox},
		{"sent", site.MailboxSent},
		{"system", site.MailboxSystem},
		{"cheater", site.MailboxCheater},
		{"reports", site.MailboxReports},
		{"staff", site.MailboxStaff},
		{"", site.MailboxInbox},        // 默认 inbox
		{"unknown", site.MailboxInbox}, // 未知回退 inbox
	}
	for _, c := range cases {
		c := c
		t.Run(c.in, func(t *testing.T) {
			t.Parallel()
			if got := parseMailbox(c.in); got != c.want {
				t.Errorf("parseMailbox(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestBoolStr(t *testing.T) {
	t.Parallel()
	if boolStr(true) != "ok" {
		t.Error(`boolStr(true) want "ok"`)
	}
	if boolStr(false) != "fail" {
		t.Error(`boolStr(false) want "fail"`)
	}
}

func TestPluginAssetScope(t *testing.T) {
	t.Parallel()
	if got := pluginAssetScope("my-plugin"); got != "plugin-assets:my-plugin" {
		t.Errorf("pluginAssetScope = %q", got)
	}
	// 防回退校验：空 id 仍带前缀（调用方应保证 id 非空）
	if !strings.HasPrefix(pluginAssetScope(""), "plugin-assets:") {
		t.Error("scope should always carry prefix")
	}
}
